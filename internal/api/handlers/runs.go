package handlers

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
)

// RunHandler handles run creation and listing.
type RunHandler struct {
	store  *db.Store
	runner *runner.Runner
}

func NewRunHandler(store *db.Store, r *runner.Runner) *RunHandler {
	return &RunHandler{store: store, runner: r}
}

type createRunRequest struct {
	AgentID string `json:"agent_id"`
	Mission string `json:"mission"`
}

// Create queues a new run for an agent.
func (h *RunHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req createRunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.AgentID == "" || req.Mission == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent_id and mission are required"})
	}

	// Verify agent exists and belongs to user
	agent, err := h.store.GetAgent(c.Context(), req.AgentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}
	if agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	// Create run record
	run, err := h.store.CreateRun(c.Context(), sqlc.CreateRunParams{
		ID:            uuid.NewString(),
		AgentID:       agent.ID,
		Mission:       req.Mission,
		ModelProvider: agent.ModelProvider,
		ModelName:     agent.ModelName,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create run"})
	}

	// Dispatch to runner (async)
	h.runner.Enqueue(runner.RunRequest{
		RunID:    run.ID,
		AgentID:  agent.ID,
		Mission:  req.Mission,
		Provider: agent.ModelProvider,
		Model:    agent.ModelName,
		Config:   agent.ConfigYaml,
	})

	return c.Status(fiber.StatusAccepted).JSON(run)
}

// Get returns a single run.
func (h *RunHandler) Get(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	runID := c.Params("id")

	run, err := h.store.GetRun(c.Context(), runID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "run not found"})
	}

	// Verify ownership via agent
	agent, err := h.store.GetAgent(c.Context(), run.AgentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(run)
}

// List returns runs for an agent.
func (h *RunHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("agent_id")

	// Verify ownership
	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}
	if agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	limit, _ := strconv.ParseInt(c.Query("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.Query("offset", "0"), 10, 64)

	runs, err := h.store.ListRunsByAgent(c.Context(), sqlc.ListRunsByAgentParams{
		AgentID: agentID,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list runs"})
	}

	return c.JSON(fiber.Map{"runs": runs})
}

// Events returns the event log for a run.
func (h *RunHandler) Events(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	runID := c.Params("id")

	run, err := h.store.GetRun(c.Context(), runID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "run not found"})
	}

	agent, err := h.store.GetAgent(c.Context(), run.AgentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	events, err := h.store.ListRunEvents(c.Context(), runID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list events"})
	}

	return c.JSON(fiber.Map{"events": events})
}

// nowSQL returns current time as sql.NullTime.
func nowSQL() sql.NullTime {
	return sql.NullTime{Time: time.Now(), Valid: true}
}
