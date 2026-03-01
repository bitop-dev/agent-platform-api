// Package runner executes agent runs using agent-core/pkg/agent.
// Runs are dispatched asynchronously and results are persisted to the database.
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

	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

// RunRequest is what gets queued for execution.
type RunRequest struct {
	RunID    string
	AgentID  string
	Mission  string
	Provider string
	Model    string
	Config   string // YAML config from agent record
	APIKey   string // Decrypted LLM API key
	BaseURL  string // Optional base URL override
}

// Runner manages concurrent agent run execution.
type Runner struct {
	store    *db.Store
	hub      *ws.Hub
	queue    chan RunRequest
	wg       sync.WaitGroup
	workers  int
	mu       sync.Mutex
	cancels  map[string]context.CancelFunc // runID → cancel
}

// New creates a runner with the given number of concurrent workers.
func New(store *db.Store, hub *ws.Hub, workers int) *Runner {
	if workers <= 0 {
		workers = 4
	}
	return &Runner{
		store:   store,
		hub:     hub,
		queue:   make(chan RunRequest, 100),
		workers: workers,
		cancels: make(map[string]context.CancelFunc),
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

// Stop waits for all workers to finish.
func (r *Runner) Stop() {
	close(r.queue)
	r.wg.Wait()
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

	// Auto-install missing skills from default registry, then register their tools
	if len(skillNames) > 0 {
		skillDir := agentpkg.DefaultSkillDir()
		for _, name := range skillNames {
			// Try to install if not already present
			_ = agentpkg.InstallSkill(agentpkg.DefaultSkillSource, name, skillDir)
		}
		// Register subprocess tools (web_search.py, web_fetch.py, etc.) into the engine
		loaded := agentpkg.RegisterSkillTools(engine, skillNames, skillDir)
		// Merge any richer skill objects from local load (have Dir, Tools, etc.)
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
				inputTokens = d.TotalTokens // approximate
			}
		}
	}

	duration := time.Since(startTime).Milliseconds()

	// Check if cancelled
	finalStatus := "succeeded"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	}

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
