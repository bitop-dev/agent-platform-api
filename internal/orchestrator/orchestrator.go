// Package orchestrator executes multi-agent workflows.
// A workflow is a DAG of steps — each step is an agent run.
// Steps with no dependencies start immediately. Steps with dependencies
// wait until all prerequisites complete, then substitute outputs into
// the mission template before dispatching.
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/google/uuid"
)

// Orchestrator manages workflow execution.
type Orchestrator struct {
	store  *db.Store
	runner *runner.Runner
	enc    *auth.Encryptor
	mu     sync.Mutex
	active map[string]context.CancelFunc // workflowRunID → cancel
}

// New creates an orchestrator.
func New(store *db.Store, runner *runner.Runner, enc *auth.Encryptor) *Orchestrator {
	return &Orchestrator{
		store:  store,
		runner: runner,
		enc:    enc,
		active: make(map[string]context.CancelFunc),
	}
}

// StartWorkflow creates a workflow run and begins execution.
func (o *Orchestrator) StartWorkflow(ctx context.Context, workflowID, userID, input string) (sqlc.WorkflowRun, error) {
	// Load workflow + steps
	wf, err := o.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return sqlc.WorkflowRun{}, fmt.Errorf("workflow not found: %w", err)
	}
	if !wf.Enabled {
		return sqlc.WorkflowRun{}, fmt.Errorf("workflow is disabled")
	}

	steps, err := o.store.ListWorkflowSteps(ctx, workflowID)
	if err != nil {
		return sqlc.WorkflowRun{}, fmt.Errorf("failed to load steps: %w", err)
	}
	if len(steps) == 0 {
		return sqlc.WorkflowRun{}, fmt.Errorf("workflow has no steps")
	}

	// Create workflow run
	wfRun, err := o.store.CreateWorkflowRun(ctx, sqlc.CreateWorkflowRunParams{
		ID:         uuid.New().String(),
		WorkflowID: workflowID,
		UserID:     userID,
		InputText:  input,
	})
	if err != nil {
		return sqlc.WorkflowRun{}, fmt.Errorf("failed to create workflow run: %w", err)
	}

	// Create step runs
	for _, step := range steps {
		_, err := o.store.CreateStepRun(ctx, sqlc.CreateStepRunParams{
			ID:            uuid.New().String(),
			WorkflowRunID: wfRun.ID,
			StepID:        step.ID,
		})
		if err != nil {
			return sqlc.WorkflowRun{}, fmt.Errorf("failed to create step run: %w", err)
		}
	}

	// Mark as running
	now := sql.NullTime{Time: time.Now(), Valid: true}
	_ = o.store.UpdateWorkflowRunStatus(ctx, sqlc.UpdateWorkflowRunStatusParams{
		Status: "running", StartedAt: now, ID: wfRun.ID,
	})

	// Launch async execution
	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	o.mu.Lock()
	o.active[wfRun.ID] = cancel
	o.mu.Unlock()

	go o.execute(execCtx, wfRun.ID, userID, input)

	wfRun.Status = "running"
	return wfRun, nil
}

// execute runs the workflow DAG. Steps with no deps start immediately.
// Completed steps trigger dependent steps.
func (o *Orchestrator) execute(ctx context.Context, wfRunID, userID, input string) {
	defer func() {
		o.mu.Lock()
		delete(o.active, wfRunID)
		o.mu.Unlock()
	}()

	log := slog.With("workflow_run_id", wfRunID)
	log.Info("workflow execution started")

	// Track step outputs by name
	stepOutputs := make(map[string]string) // step name → output text

	for {
		select {
		case <-ctx.Done():
			o.failWorkflow(wfRunID, "workflow execution timed out")
			return
		default:
		}

		// Load current state of all step runs
		stepRuns, err := o.store.ListStepRuns(ctx, wfRunID)
		if err != nil {
			o.failWorkflow(wfRunID, "failed to load step runs: "+err.Error())
			return
		}

		// Check if all done
		allDone := true
		anyFailed := false
		for _, sr := range stepRuns {
			if sr.Status == "pending" || sr.Status == "running" {
				allDone = false
			}
			if sr.Status == "failed" {
				anyFailed = true
			}
		}

		if allDone {
			if anyFailed {
				o.failWorkflow(wfRunID, "one or more steps failed")
			} else {
				o.completeWorkflow(wfRunID, stepOutputs)
			}
			return
		}

		// If any step failed, don't start new steps — let running ones finish
		if anyFailed {
			// Wait for running steps to complete
			hasRunning := false
			for _, sr := range stepRuns {
				if sr.Status == "running" {
					hasRunning = true
					break
				}
			}
			if !hasRunning {
				o.failWorkflow(wfRunID, "one or more steps failed")
				return
			}
			time.Sleep(2 * time.Second)
			continue
		}

		// Find pending steps whose dependencies are met
		pendingSteps, _ := o.store.ListPendingStepRuns(ctx, wfRunID)
		for _, ps := range pendingSteps {
			// Parse depends_on
			var deps []string
			json.Unmarshal([]byte(ps.DependsOn), &deps)

			// Check all deps are succeeded
			depsReady := true
			for _, dep := range deps {
				if _, ok := stepOutputs[dep]; !ok {
					depsReady = false
					break
				}
			}

			if !depsReady {
				continue
			}

			// Dispatch this step
			if err := o.dispatchStep(ctx, wfRunID, ps, userID, input, stepOutputs); err != nil {
				log.Error("failed to dispatch step", "step", ps.StepName, "error", err)
				o.updateStepStatus(ps.ID, "failed", "")
			}
		}

		// Wait before polling again
		time.Sleep(2 * time.Second)

		// Collect completed step outputs
		for _, sr := range stepRuns {
			if sr.Status == "succeeded" && sr.RunID.Valid {
				if _, already := stepOutputs[sr.StepName]; !already {
					// Load the run output
					run, err := o.store.GetRun(ctx, sr.RunID.String)
					if err == nil && run.OutputText.Valid {
						stepOutputs[sr.StepName] = run.OutputText.String
					}
				}
			}
		}
	}
}

// dispatchStep starts an agent run for a workflow step.
func (o *Orchestrator) dispatchStep(ctx context.Context, wfRunID string, sr sqlc.ListPendingStepRunsRow, userID, input string, stepOutputs map[string]string) error {
	log := slog.With("workflow_run_id", wfRunID, "step", sr.StepName)

	// Resolve the agent
	agent, err := o.store.GetAgent(ctx, sr.AgentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	// Resolve API key
	apiKey, baseURL, err := resolveAPIKey(ctx, o.store, o.enc, userID, agent.ModelProvider)
	if err != nil {
		return fmt.Errorf("no API key for provider %s: %w", agent.ModelProvider, err)
	}

	// Build mission from template
	mission := sr.MissionTemplate
	mission = strings.ReplaceAll(mission, "{{input}}", input)
	for name, output := range stepOutputs {
		mission = strings.ReplaceAll(mission, "{{steps."+name+".output}}", output)
	}

	// Create the actual agent run
	runID := uuid.New().String()
	_, err = o.store.CreateRun(ctx, sqlc.CreateRunParams{
		ID:            runID,
		AgentID:       agent.ID,
		Mission:       mission,
		ModelProvider: agent.ModelProvider,
		ModelName:     agent.ModelName,
	})
	if err != nil {
		return fmt.Errorf("failed to create run: %w", err)
	}

	// Link step run to agent run
	now := sql.NullTime{Time: time.Now(), Valid: true}
	_ = o.store.UpdateStepRun(ctx, sqlc.UpdateStepRunParams{
		RunID:     sql.NullString{String: runID, Valid: true},
		Status:    "running",
		StartedAt: now,
		ID:        sr.ID,
	})

	log.Info("dispatching step", "agent", agent.Name, "run_id", runID)

	// Enqueue in the runner
	o.runner.Enqueue(runner.RunRequest{
		RunID:    runID,
		AgentID:  agent.ID,
		UserID:   userID,
		Mission:  mission,
		Provider: agent.ModelProvider,
		Model:    agent.ModelName,
		Config:   agent.ConfigYaml,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})

	// Start a goroutine to watch for completion
	go o.watchStep(ctx, wfRunID, sr.ID, runID)

	return nil
}

// watchStep polls until the agent run completes, then updates the step status.
func (o *Orchestrator) watchStep(ctx context.Context, wfRunID, stepRunID, runID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		run, err := o.store.GetRun(ctx, runID)
		if err != nil {
			continue
		}

		switch run.Status {
		case "succeeded":
			now := sql.NullTime{Time: time.Now(), Valid: true}
			_ = o.store.UpdateStepRun(ctx, sqlc.UpdateStepRunParams{
				RunID: sql.NullString{String: runID, Valid: true},
				Status: "succeeded", CompletedAt: now, ID: stepRunID,
			})
			return
		case "failed", "cancelled":
			now := sql.NullTime{Time: time.Now(), Valid: true}
			_ = o.store.UpdateStepRun(ctx, sqlc.UpdateStepRunParams{
				RunID: sql.NullString{String: runID, Valid: true},
				Status: "failed", CompletedAt: now, ID: stepRunID,
			})
			return
		}
	}
}

func (o *Orchestrator) updateStepStatus(stepRunID, status, runID string) {
	now := sql.NullTime{Time: time.Now(), Valid: true}
	var rid sql.NullString
	if runID != "" {
		rid = sql.NullString{String: runID, Valid: true}
	}
	_ = o.store.UpdateStepRun(context.Background(), sqlc.UpdateStepRunParams{
		RunID: rid, Status: status, CompletedAt: now, ID: stepRunID,
	})
}

func (o *Orchestrator) failWorkflow(wfRunID, errMsg string) {
	slog.Error("workflow failed", "workflow_run_id", wfRunID, "error", errMsg)
	now := sql.NullTime{Time: time.Now(), Valid: true}
	_ = o.store.UpdateWorkflowRunStatus(context.Background(), sqlc.UpdateWorkflowRunStatusParams{
		Status: "failed", CompletedAt: now,
		ErrorMessage: sql.NullString{String: errMsg, Valid: true},
		ID:           wfRunID,
	})
}

func (o *Orchestrator) completeWorkflow(wfRunID string, stepOutputs map[string]string) {
	slog.Info("workflow completed", "workflow_run_id", wfRunID)

	// Concatenate step outputs as the workflow output
	var parts []string
	for name, output := range stepOutputs {
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", name, output))
	}
	combined := strings.Join(parts, "\n\n---\n\n")

	now := sql.NullTime{Time: time.Now(), Valid: true}
	_ = o.store.UpdateWorkflowRunStatus(context.Background(), sqlc.UpdateWorkflowRunStatusParams{
		Status: "succeeded", CompletedAt: now,
		OutputText: sql.NullString{String: combined, Valid: true},
		ID:         wfRunID,
	})
}

// resolveAPIKey finds the user's API key for a given provider.
func resolveAPIKey(ctx context.Context, store *db.Store, enc *auth.Encryptor, userID, provider string) (string, string, error) {
	key, err := store.GetDefaultAPIKey(ctx, sqlc.GetDefaultAPIKeyParams{
		UserID:   userID,
		Provider: provider,
	})
	if err != nil {
		return "", "", err
	}

	plaintext, err := enc.Decrypt(key.KeyEnc)
	if err != nil {
		return "", "", err
	}

	return plaintext, key.BaseUrl, nil
}

// Stop cancels all active workflow runs.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, cancel := range o.active {
		cancel()
		delete(o.active, id)
	}
}
