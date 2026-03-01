// Package runner executes agent runs using agent-core/pkg/agent.
// Runs are dispatched asynchronously and results are persisted to the database.
// All skill tools execute inside a WASM sandbox via Wazero.
package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agentpkg "github.com/bitop-dev/agent-core/pkg/agent"
	"github.com/bitop-dev/agent-platform-api/internal/metrics"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

// RunRequest is what gets queued for execution.
type RunRequest struct {
	RunID    string
	AgentID  string
	UserID   string // owner — used to resolve skill credentials
	Mission  string
	Provider string
	Model    string
	Config   string // YAML config from agent record
	APIKey   string // Decrypted LLM API key
	BaseURL  string // Optional base URL override
}

// Runner manages concurrent agent run execution.
type Runner struct {
	store      *db.Store
	hub        *ws.Hub
	queue      chan RunRequest
	wg         sync.WaitGroup
	workers    int
	mu         sync.Mutex
	cancels    map[string]context.CancelFunc // runID → cancel
	sandboxReg *agentpkg.SandboxRegistry     // WASM/container sandbox registry
	enc        *auth.Encryptor               // for decrypting user credentials
}

// New creates a runner with the given number of concurrent workers.
// Initializes the WASM sandbox runtime for skill tool execution.
func New(store *db.Store, hub *ws.Hub, enc *auth.Encryptor, workers int) *Runner {
	if workers <= 0 {
		workers = 4
	}

	// Initialize sandbox registry with WASM runtime
	reg := agentpkg.NewSandboxRegistry()

	wasmRT, err := agentpkg.NewWASMRuntime(context.Background())
	if err != nil {
		slog.Error("failed to initialize WASM runtime", "error", err)
	} else {
		reg.Register(wasmRT)
		slog.Info("WASM sandbox runtime ready")
	}

	// Try container runtime (optional — requires Docker/Podman)
	containerRT, err := agentpkg.NewContainerRuntime()
	if err == nil {
		reg.Register(containerRT)
		slog.Info("container sandbox runtime ready")
	}

	return &Runner{
		store:      store,
		hub:        hub,
		enc:        enc,
		queue:      make(chan RunRequest, 100),
		workers:    workers,
		cancels:    make(map[string]context.CancelFunc),
		sandboxReg: reg,
	}
}

// Start launches worker goroutines.
func (r *Runner) Start() {
	for i := range r.workers {
		r.wg.Add(1)
		go r.worker(i)
	}
	slog.Info("runner started", "workers", r.workers)
}

// Stop waits for all workers to finish and cleans up sandbox runtimes.
func (r *Runner) Stop() {
	close(r.queue)
	r.wg.Wait()
	if r.sandboxReg != nil {
		r.sandboxReg.Close()
	}
}

// Enqueue adds a run to the execution queue.
func (r *Runner) Enqueue(req RunRequest) {
	r.queue <- req
}

func (r *Runner) worker(_ int) {
	defer r.wg.Done()
	for req := range r.queue {
		r.execute(req)
	}
}

// makeProvider creates the right LLM provider based on name.
func makeProvider(name, apiKey, baseURL string) agentpkg.Provider {
	switch {
	case strings.Contains(strings.ToLower(name), "anthropic"):
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		return agentpkg.NewAnthropicProvider(apiKey, baseURL)
	default:
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return agentpkg.NewOpenAIProvider(apiKey, baseURL)
	}
}

// Cancel cancels an in-flight run. Returns true if the run was found and cancelled.
func (r *Runner) Cancel(runID string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[runID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *Runner) execute(req RunRequest) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.mu.Lock()
	r.cancels[req.RunID] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.cancels, req.RunID)
		r.mu.Unlock()
	}()

	startTime := time.Now()

	metrics.Global.RunsStarted.Add(1)

	// Mark as running
	_ = r.store.UpdateRunStatus(ctx, sqlc.UpdateRunStatusParams{
		Status:    "running",
		StartedAt: sql.NullTime{Time: startTime, Valid: true},
		ID:        req.RunID,
	})
	r.broadcast(req.RunID, "status", map[string]string{"status": "running"})

	// Build provider
	p := makeProvider(req.Provider, req.APIKey, req.BaseURL)
	p = agentpkg.NewReliableProvider(p)

	// Build agent config
	cfg := &agentpkg.Config{
		Name:           fmt.Sprintf("run-%s", req.RunID[:8]),
		Model:          req.Model,
		MaxTurns:       20,
		TimeoutSeconds: 300,
	}

	// If we have stored YAML config, parse it
	if req.Config != "" {
		if parsed, err := agentpkg.ParseConfig([]byte(req.Config)); err == nil {
			cfg = parsed
			cfg.Model = req.Model // ensure model matches
		}
	}

	engine := agentpkg.NewToolEngine()

	// Load skills attached to this agent
	agentSkills, err := r.store.ListAgentSkills(ctx, req.AgentID)
	if err != nil {
		slog.Error("failed to load agent skills", "agent_id", req.AgentID, "error", err)
	}

	var skills []*agentpkg.Skill
	var skillNames []string
	for _, as := range agentSkills {
		skillNames = append(skillNames, as.Name)
		// If we have full SKILL.md content, parse it to get rich metadata
		if as.SkillMd != "" {
			if parsed, err := agentpkg.ParseSkillMD([]byte(as.SkillMd)); err == nil {
				skills = append(skills, parsed)
				continue
			}
		}
		// Fallback: create a minimal skill from DB fields
		skills = append(skills, agentpkg.NewSkill(as.Name, as.Description, as.SkillMd))
	}

	// Auto-install missing skills from default registry, then register their WASM tools
	if len(skillNames) > 0 {
		skillDir := agentpkg.DefaultSkillDir()
		for _, name := range skillNames {
			// Try to install if not already present
			_ = agentpkg.InstallSkill(agentpkg.DefaultSkillSource, name, skillDir)
		}

		// Build sandbox capabilities — allow all network hosts by default for platform runs
		caps := agentpkg.SandboxCapabilities{
			AllowedHosts:   []string{"*"}, // platform runs get full network access
			MaxTimeoutSec:  60,
			MaxOutputBytes: 1 << 20, // 1MB
			MaxMemoryMB:    256,
			EnvVars:        make(map[string]string),
		}

		// Load user's skill credentials and pass as env vars to WASM/container tools
		if req.UserID != "" {
			creds, err := r.store.ListAllCredentialValues(ctx, req.UserID)
			if err == nil {
				for _, cred := range creds {
					plaintext, err := r.enc.Decrypt(cred.ValueEnc)
					if err != nil {
						slog.Warn("failed to decrypt credential", "name", cred.Name, "error", err)
						continue
					}
					caps.EnvVars[cred.Name] = plaintext
				}
				if len(creds) > 0 {
					slog.Info("loaded user credentials for sandbox", "agent_id", req.AgentID, "count", len(creds))
				}
			}
		}

		// Register WASM skill tools through the sandbox system
		loaded := agentpkg.RegisterSkillTools(engine, r.sandboxReg, skillNames, "wasm", caps, skillDir)
		// Merge richer skill objects from local load (have Dir, Tools, etc.)
		if len(loaded) > 0 {
			skills = loaded
		}
		slog.Info("loaded agent skills", "agent_id", req.AgentID, "count", len(skills), "tool_skills", len(loaded))
	}

	agent, err := agentpkg.NewBuilder().
		WithConfig(cfg).
		WithProvider(p).
		WithTools(engine).
		WithSkills(skills).
		WithObserver(agentpkg.NoopObserver{}).
		Build()
	if err != nil {
		r.failRun(ctx, req.RunID, startTime, fmt.Sprintf("build agent: %v", err))
		return
	}

	events, err := agent.Run(ctx, req.Mission)
	if err != nil {
		r.failRun(ctx, req.RunID, startTime, fmt.Sprintf("start run: %v", err))
		return
	}

	// Stream events
	var (
		outputText   string
		totalTurns   int
		inputTokens  int
		outputTokens int
		seq          int
	)

	for event := range events {
		seq++

		// Persist event
		data, _ := json.Marshal(event.Data)
		_ = r.store.InsertRunEvent(ctx, sqlc.InsertRunEventParams{
			RunID:     req.RunID,
			Seq:       int64(seq),
			EventType: string(event.Type),
			DataJson:  sql.NullString{String: string(data), Valid: true},
		})

		// Broadcast to WebSocket subscribers
		r.broadcast(req.RunID, string(event.Type), event.Data)

		// Accumulate results
		switch event.Type {
		case agentpkg.EventTextDelta:
			if d, ok := event.Data.(agentpkg.TextDeltaData); ok {
				outputText += d.Text
			}
		case agentpkg.EventAgentEnd:
			if d, ok := event.Data.(agentpkg.AgentEndData); ok {
				totalTurns = d.TotalTurns
				inputTokens = d.InputTokens
				outputTokens = d.OutputTokens
			}
		}
	}

	duration := time.Since(startTime).Milliseconds()

	// Check if cancelled
	finalStatus := "succeeded"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	}

	metrics.Global.RunsFinished.Add(1)

	_ = r.store.UpdateRunResult(ctx, sqlc.UpdateRunResultParams{
		Status:       finalStatus,
		OutputText:   sql.NullString{String: outputText, Valid: true},
		TotalTurns:   sql.NullInt64{Int64: int64(totalTurns), Valid: true},
		InputTokens:  sql.NullInt64{Int64: int64(inputTokens), Valid: true},
		OutputTokens: sql.NullInt64{Int64: int64(outputTokens), Valid: true},
		DurationMs:   sql.NullInt64{Int64: duration, Valid: true},
		CompletedAt:  sql.NullTime{Time: time.Now(), Valid: true},
		ID:           req.RunID,
	})

	r.broadcast(req.RunID, "status", map[string]string{"status": finalStatus})
}

func (r *Runner) failRun(ctx context.Context, runID string, startTime time.Time, errMsg string) {
	metrics.Global.RunsFailed.Add(1)
	duration := time.Since(startTime).Milliseconds()

	_ = r.store.UpdateRunResult(ctx, sqlc.UpdateRunResultParams{
		Status:       "failed",
		ErrorMessage: sql.NullString{String: errMsg, Valid: true},
		DurationMs:   sql.NullInt64{Int64: duration, Valid: true},
		CompletedAt:  sql.NullTime{Time: time.Now(), Valid: true},
		ID:           runID,
	})

	r.broadcast(runID, "status", map[string]string{"status": "failed", "error": errMsg})
}

func (r *Runner) broadcast(runID, eventType string, data any) {
	if r.hub == nil {
		return
	}
	r.hub.Broadcast(runID, ws.Event{
		Type: eventType,
		Data: data,
	})
}
