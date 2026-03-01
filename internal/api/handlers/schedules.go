package handlers

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/scheduler"
)

type ScheduleHandler struct {
	store     *db.Store
	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	enc       *auth.Encryptor
	audit     *audit.Logger
}

func NewScheduleHandler(store *db.Store, sched *scheduler.Scheduler, r *runner.Runner, enc *auth.Encryptor) *ScheduleHandler {
	return &ScheduleHandler{store: store, scheduler: sched, runner: r, enc: enc, audit: audit.NewLogger(store.Queries)}
}

type CreateScheduleRequest struct {
	AgentID         string `json:"agent_id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleType    string `json:"schedule_type"`    // cron | every | once
	CronExpr        string `json:"cron_expr"`
	IntervalSeconds int64  `json:"interval_seconds"`
	Timezone        string `json:"timezone"`
	Mission         string `json:"mission"`
	Enabled         *bool  `json:"enabled"`
	OverlapPolicy   string `json:"overlap_policy"`
	MaxRetries      int    `json:"max_retries"`
}

func (h *ScheduleHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req CreateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if req.AgentID == "" || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent_id and name required"})
	}
	if req.ScheduleType == "" {
		req.ScheduleType = "cron"
	}
	if req.ScheduleType == "cron" && req.CronExpr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cron_expr required for cron schedules"})
	}
	if req.ScheduleType == "every" && req.IntervalSeconds <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "interval_seconds must be > 0 for every schedules"})
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.OverlapPolicy == "" {
		req.OverlapPolicy = "skip"
	}
	if req.MaxRetries <= 0 {
		req.MaxRetries = 3
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Verify agent belongs to user
	agent, err := h.store.GetAgent(c.Context(), req.AgentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}

	// Compute next run time
	var nextRun sql.NullTime
	if enabled {
		t, err := h.scheduler.ComputeNextRun(req.ScheduleType, req.CronExpr, req.Timezone, req.IntervalSeconds)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if !t.IsZero() {
			nextRun = sql.NullTime{Time: t.UTC().Truncate(time.Second), Valid: true}
		}
	}

	sched, err := h.store.CreateSchedule(c.Context(), sqlc.CreateScheduleParams{
		ID:              uuid.New().String(),
		UserID:          userID,
		AgentID:         req.AgentID,
		Name:            req.Name,
		Description:     req.Description,
		ScheduleType:    req.ScheduleType,
		CronExpr:        req.CronExpr,
		IntervalSeconds: req.IntervalSeconds,
		Timezone:        req.Timezone,
		Mission:         req.Mission,
		Enabled:         enabled,
		OverlapPolicy:   req.OverlapPolicy,
		MaxRetries:      int64(req.MaxRetries),
		NextRunAt:       nextRun,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit.Log(c.Context(), userID, audit.ActionScheduleCreate, sched.ID, c.IP(), map[string]any{
		"name": sched.Name,
		"type": sched.ScheduleType,
	})

	return c.Status(fiber.StatusCreated).JSON(scheduleToDTO(sched))
}

func (h *ScheduleHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	schedules, err := h.store.ListSchedules(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"schedules": schedulesToDTOs(schedules)})
}

func (h *ScheduleHandler) ListByAgent(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("agent_id")
	schedules, err := h.store.ListSchedulesByAgent(c.Context(), sqlc.ListSchedulesByAgentParams{
		AgentID: agentID, UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"schedules": schedulesByAgentToDTOs(schedules)})
}

func (h *ScheduleHandler) Get(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	sched, err := h.store.GetSchedule(c.Context(), sqlc.GetScheduleParams{
		ID: c.Params("id"), UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}
	return c.JSON(scheduleToDTO(sched))
}

type UpdateScheduleRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleType    string `json:"schedule_type"`
	CronExpr        string `json:"cron_expr"`
	IntervalSeconds int64  `json:"interval_seconds"`
	Timezone        string `json:"timezone"`
	Mission         string `json:"mission"`
	Enabled         *bool  `json:"enabled"`
	OverlapPolicy   string `json:"overlap_policy"`
	MaxRetries      int    `json:"max_retries"`
}

func (h *ScheduleHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	schedID := c.Params("id")

	var req UpdateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	// Get existing
	existing, err := h.store.GetSchedule(c.Context(), sqlc.GetScheduleParams{ID: schedID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}

	// Merge
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.ScheduleType == "" {
		req.ScheduleType = existing.ScheduleType
	}
	if req.CronExpr == "" {
		req.CronExpr = existing.CronExpr
	}
	if req.IntervalSeconds == 0 {
		req.IntervalSeconds = existing.IntervalSeconds
	}
	if req.Timezone == "" {
		req.Timezone = existing.Timezone
	}
	if req.OverlapPolicy == "" {
		req.OverlapPolicy = existing.OverlapPolicy
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = int(existing.MaxRetries)
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Recompute next run
	var nextRun sql.NullTime
	if enabled {
		t, err := h.scheduler.ComputeNextRun(req.ScheduleType, req.CronExpr, req.Timezone, req.IntervalSeconds)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if !t.IsZero() {
			nextRun = sql.NullTime{Time: t.UTC().Truncate(time.Second), Valid: true}
		}
	}

	sched, err := h.store.UpdateSchedule(c.Context(), sqlc.UpdateScheduleParams{
		Name:            req.Name,
		Description:     req.Description,
		ScheduleType:    req.ScheduleType,
		CronExpr:        req.CronExpr,
		IntervalSeconds: req.IntervalSeconds,
		Timezone:        req.Timezone,
		Mission:         req.Mission,
		Enabled:         enabled,
		OverlapPolicy:   req.OverlapPolicy,
		MaxRetries:      int64(req.MaxRetries),
		NextRunAt:       nextRun,
		ID:              schedID,
		UserID:          userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(scheduleToDTO(sched))
}

func (h *ScheduleHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	schedID := c.Params("id")
	err := h.store.DeleteSchedule(c.Context(), sqlc.DeleteScheduleParams{
		ID: schedID, UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	h.audit.Log(c.Context(), userID, audit.ActionScheduleDelete, schedID, c.IP(), nil)
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *ScheduleHandler) Enable(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	schedID := c.Params("id")

	// Re-compute next_run before enabling
	sched, err := h.store.GetSchedule(c.Context(), sqlc.GetScheduleParams{ID: schedID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}

	if err := h.store.EnableSchedule(c.Context(), sqlc.EnableScheduleParams{ID: schedID, UserID: userID}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Set next_run_at
	t, _ := h.scheduler.ComputeNextRun(sched.ScheduleType, sched.CronExpr, sched.Timezone, sched.IntervalSeconds)
	if !t.IsZero() {
		_ = h.store.UpdateScheduleAfterRun(c.Context(), sqlc.UpdateScheduleAfterRunParams{
			LastRunAt:         sched.LastRunAt,
			LastRunStatus:     sched.LastRunStatus,
			LastRunID:         sched.LastRunID,
			LastError:         sched.LastError,
			ConsecutiveErrors: 0,
			NextRunAt:         sql.NullTime{Time: t.UTC().Truncate(time.Second), Valid: true},
			ID:                schedID,
		})
	}

	return c.JSON(fiber.Map{"status": "enabled"})
}

func (h *ScheduleHandler) Disable(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if err := h.store.DisableSchedule(c.Context(), sqlc.DisableScheduleParams{
		ID: c.Params("id"), UserID: userID,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "disabled"})
}

// Trigger fires a schedule immediately regardless of next_run_at.
func (h *ScheduleHandler) Trigger(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	sched, err := h.store.GetSchedule(c.Context(), sqlc.GetScheduleParams{
		ID: c.Params("id"), UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}

	// Get agent details
	agent, err := h.store.GetAgent(c.Context(), sched.AgentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}

	// Resolve API key — try default for provider, then fallback to openai
	var apiKey, baseURL string
	key, err := h.store.GetDefaultAPIKey(c.Context(), sqlc.GetDefaultAPIKeyParams{UserID: userID, Provider: agent.ModelProvider})
	if err != nil {
		key, err = h.store.GetDefaultAPIKey(c.Context(), sqlc.GetDefaultAPIKeyParams{UserID: userID, Provider: "openai"})
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no API key for provider"})
	}
	apiKey, _ = h.enc.Decrypt(key.KeyEnc)
	baseURL = key.BaseUrl
	if apiKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to decrypt API key"})
	}

	mission := sched.Mission
	if mission == "" {
		mission = fmt.Sprintf("Scheduled run: %s", sched.Name)
	}

	runID := uuid.New().String()
	_, err = h.store.CreateRun(c.Context(), sqlc.CreateRunParams{
		ID:            runID,
		AgentID:       sched.AgentID,
		Mission:       mission,
		ModelProvider: agent.ModelProvider,
		ModelName:     agent.ModelName,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.runner.Enqueue(runner.RunRequest{
		RunID:    runID,
		AgentID:  sched.AgentID,
		Mission:  mission,
		Provider: agent.ModelProvider,
		Model:    agent.ModelName,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})

	return c.JSON(fiber.Map{"status": "triggered", "run_id": runID})
}
