// Package scheduler provides a cron-based scheduler for triggering agent runs.
// It polls the database for due schedules and enqueues runs via the runner.
package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
)

// Scheduler checks for due schedules and enqueues runs.
type Scheduler struct {
	store    *db.Store
	runner   *runner.Runner
	enc      *auth.Encryptor
	parser   cron.Parser
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a scheduler that polls every interval.
func New(store *db.Store, r *runner.Runner, enc *auth.Encryptor, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		store:    store,
		runner:   r,
		enc:      enc,
		parser:   cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		interval: interval,
	}
}

// Start begins the polling loop.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go s.loop(ctx)
	slog.Info("scheduler started", "interval", s.interval.String())
}

// Stop halts the polling loop and waits for it to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	slog.Info("scheduler stopped")
}

func (s *Scheduler) loop(ctx context.Context) {
	defer s.wg.Done()

	// Run once immediately on startup
	s.tick(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now().UTC()

	due, err := s.store.ListDueSchedules(ctx, sql.NullTime{Time: now, Valid: true})
	if err != nil {
		slog.Error("scheduler: list due schedules", "error", err)
		return
	}

	for _, sched := range due {
		s.fire(ctx, sched, now)
	}
}

func (s *Scheduler) fire(ctx context.Context, sched sqlc.ListDueSchedulesRow, now time.Time) {
	log := slog.With("schedule_id", sched.ID, "agent_id", sched.AgentID, "name", sched.Name)

	// Check overlap policy — if "skip" and last run is still active, skip
	if sched.OverlapPolicy == "skip" && sched.LastRunID.Valid {
		lastRun, err := s.store.GetRun(ctx, sched.LastRunID.String)
		if err == nil && (lastRun.Status == "queued" || lastRun.Status == "running") {
			log.Info("skipping — previous run still active", "last_run", sched.LastRunID.String)
			// Still advance next_run_at so we don't fire every tick
			nextRun := s.computeNextRun(sched, now)
			_ = s.store.UpdateScheduleAfterRun(ctx, sqlc.UpdateScheduleAfterRunParams{
				LastRunAt:         sched.LastRunAt,
				LastRunStatus:     sched.LastRunStatus,
				LastRunID:         sched.LastRunID,
				LastError:         sched.LastError,
				ConsecutiveErrors: sched.ConsecutiveErrors,
				NextRunAt:         sql.NullTime{Time: nextRun, Valid: true},
				ID:                sched.ID,
			})
			return
		}
	}

	// Resolve the API key for this agent's provider
	apiKey, baseURL, err := s.resolveAPIKey(ctx, sched.UserID, sched.ModelProvider)
	if err != nil {
		log.Error("no API key for provider", "provider", sched.ModelProvider, "error", err)
		s.recordFailure(ctx, sched, now, fmt.Sprintf("no API key: %v", err))
		return
	}

	// Determine mission
	mission := sched.Mission
	if mission == "" {
		mission = fmt.Sprintf("Scheduled run for agent %q", sched.Name)
	}

	// Create a run record
	runID := uuid.New().String()
	_, err = s.store.CreateRun(ctx, sqlc.CreateRunParams{
		ID:            runID,
		AgentID:       sched.AgentID,
		Mission:       mission,
		ModelProvider: sched.ModelProvider,
		ModelName:     sched.ModelName,
	})
	if err != nil {
		log.Error("create run record", "error", err)
		s.recordFailure(ctx, sched, now, fmt.Sprintf("create run: %v", err))
		return
	}

	// Enqueue in the runner
	s.runner.Enqueue(runner.RunRequest{
		RunID:    runID,
		AgentID:  sched.AgentID,
		Mission:  mission,
		Provider: sched.ModelProvider,
		Model:    sched.ModelName,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})

	log.Info("scheduled run enqueued", "run_id", runID)

	// Update schedule state
	nextRun := s.computeNextRun(sched, now)
	_ = s.store.UpdateScheduleAfterRun(ctx, sqlc.UpdateScheduleAfterRunParams{
		LastRunAt:         sql.NullTime{Time: now, Valid: true},
		LastRunStatus:     sql.NullString{String: "queued", Valid: true},
		LastRunID:         sql.NullString{String: runID, Valid: true},
		LastError:         sql.NullString{},
		ConsecutiveErrors: 0,
		NextRunAt:         sql.NullTime{Time: nextRun, Valid: !nextRun.IsZero()},
		ID:                sched.ID,
	})
}

func (s *Scheduler) recordFailure(ctx context.Context, sched sqlc.ListDueSchedulesRow, now time.Time, errMsg string) {
	consecutive := sched.ConsecutiveErrors + 1
	nextRun := s.computeNextRun(sched, now)

	// Auto-disable after max retries
	if consecutive >= sched.MaxRetries {
		slog.Warn("schedule auto-disabled after max retries", "schedule_id", sched.ID, "errors", consecutive)
		_ = s.store.DisableSchedule(ctx, sqlc.DisableScheduleParams{ID: sched.ID, UserID: sched.UserID})
	}

	_ = s.store.UpdateScheduleAfterRun(ctx, sqlc.UpdateScheduleAfterRunParams{
		LastRunAt:         sql.NullTime{Time: now, Valid: true},
		LastRunStatus:     sql.NullString{String: "failed", Valid: true},
		LastRunID:         sql.NullString{},
		LastError:         sql.NullString{String: errMsg, Valid: true},
		ConsecutiveErrors: consecutive,
		NextRunAt:         sql.NullTime{Time: nextRun, Valid: !nextRun.IsZero()},
		ID:                sched.ID,
	})
}

func (s *Scheduler) computeNextRun(sched sqlc.ListDueSchedulesRow, after time.Time) time.Time {
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}
	after = after.In(loc)

	switch sched.ScheduleType {
	case "cron":
		sch, err := s.parser.Parse(sched.CronExpr)
		if err != nil {
			slog.Error("invalid cron expression", "schedule_id", sched.ID, "expr", sched.CronExpr, "error", err)
			return time.Time{} // disable effectively
		}
		return sch.Next(after).UTC()

	case "every":
		if sched.IntervalSeconds <= 0 {
			return time.Time{}
		}
		return after.Add(time.Duration(sched.IntervalSeconds) * time.Second).UTC()

	case "once":
		// One-shot: no next run
		return time.Time{}

	default:
		return time.Time{}
	}
}

func (s *Scheduler) resolveAPIKey(ctx context.Context, userID, provider string) (string, string, error) {
	// Try default key for exact provider
	key, err := s.store.GetDefaultAPIKey(ctx, sqlc.GetDefaultAPIKeyParams{UserID: userID, Provider: provider})
	if err == nil {
		dec, err := s.enc.Decrypt(key.KeyEnc)
		if err != nil {
			return "", "", err
		}
		return dec, key.BaseUrl, nil
	}

	// Try default openai key (proxy compatibility)
	if provider != "openai" {
		key, err = s.store.GetDefaultAPIKey(ctx, sqlc.GetDefaultAPIKeyParams{UserID: userID, Provider: "openai"})
		if err == nil {
			dec, err := s.enc.Decrypt(key.KeyEnc)
			if err != nil {
				return "", "", err
			}
			return dec, key.BaseUrl, nil
		}
	}

	return "", "", fmt.Errorf("no API key found for provider %q", provider)
}

// ComputeNextRun is exported for use by handlers when creating/updating schedules.
func (s *Scheduler) ComputeNextRun(schedType, cronExpr, tz string, intervalSec int64) (time.Time, error) {
	now := time.Now().UTC()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)

	switch schedType {
	case "cron":
		sch, err := s.parser.Parse(cronExpr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron: %w", err)
		}
		return sch.Next(now).UTC(), nil

	case "every":
		if intervalSec <= 0 {
			return time.Time{}, fmt.Errorf("interval must be > 0")
		}
		return now.Add(time.Duration(intervalSec) * time.Second).UTC(), nil

	case "once":
		return time.Time{}, nil

	default:
		return time.Time{}, fmt.Errorf("unknown schedule type: %s", schedType)
	}
}
