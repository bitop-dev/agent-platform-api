package handlers

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
)

// RunHandler handles run creation and listing.
type RunHandler struct {
	store     *db.Store
	runner    *runner.Runner
	encryptor *auth.Encryptor
	audit     *audit.Logger
}

func NewRunHandler(store *db.Store, r *runner.Runner, enc *auth.Encryptor) *RunHandler {
	return &RunHandler{store: store, runner: r, encryptor: enc, audit: audit.NewLogger(store.Queries)}
}

type createRunRequest struct {
	AgentID string `json:"agent_id"`
	Mission string `json:"mission"`
	BaseURL string `json:"base_url,omitempty"` // Optional LLM API base URL override
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

	// Look up user's API key for this provider
	var apiKey, baseURL string
	dbKey, err := h.store.GetDefaultAPIKey(c.Context(), sqlc.GetDefaultAPIKeyParams{
		UserID:   userID,
		Provider: agent.ModelProvider,
	})
	if err == nil {
		if decrypted, err := h.encryptor.Decrypt(dbKey.KeyEnc); err == nil {
			apiKey = decrypted
		}
		baseURL = dbKey.BaseUrl
	}

	// Run request base_url overrides stored key base_url
	if req.BaseURL != "" {
		baseURL = req.BaseURL
	}

	// Dispatch to runner (async)
	h.runner.Enqueue(runner.RunRequest{
		RunID:    run.ID,
		AgentID:  agent.ID,
		Mission:  req.Mission,
		Provider: agent.ModelProvider,
		Model:    agent.ModelName,
		Config:   agent.ConfigYaml,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})

	h.audit.Log(c.Context(), userID, audit.ActionRunCreate, run.ID, c.IP(), map[string]any{
		"agent_id": agent.ID,
		"agent":    agent.Name,
	})

	return c.Status(fiber.StatusAccepted).JSON(runToDTO(run))
}

// List returns all runs for the current user.
func (h *RunHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "25"))
	status := c.Query("status", "")
	agentID := c.Query("agent_id", "")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}
	offset := int64((page - 1) * perPage)

	// Use filtered query if filters are set, otherwise plain list
	var runs []sqlc.Run
	var err error
	if status != "" || agentID != "" {
		runs, err = h.store.ListRunsByUserFiltered(c.Context(), sqlc.ListRunsByUserFilteredParams{
			UserID:   userID,
			Column2:  status,
			Column3:  status,
			Column4:  agentID,
			Column5:  agentID,
			Limit:    int64(perPage),
			Offset:   offset,
		})
	} else {
		runs, err = h.store.ListRunsByUser(c.Context(), sqlc.ListRunsByUserParams{
			UserID: userID,
			Limit:  int64(perPage),
			Offset: offset,
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list runs"})
	}

	total, _ := h.store.CountRunsByUser(c.Context(), userID)

	return c.JSON(fiber.Map{
		"runs":     runsToDTOs(runs),
		"page":     page,
		"per_page": perPage,
		"total":    total,
	})
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

	return c.JSON(runToDTO(run))
}

// List returns runs for an agent.
// ListByAgent returns runs for a specific agent.
func (h *RunHandler) ListByAgent(c *fiber.Ctx) error {
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

	return c.JSON(fiber.Map{"runs": runsToDTOs(runs)})
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

	return c.JSON(fiber.Map{"events": eventsToDTOs(events)})
}

// Cancel cancels an in-flight run.
func (h *RunHandler) Cancel(c *fiber.Ctx) error {
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

	if run.Status != "running" && run.Status != "queued" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "run is not active"})
	}

	if h.runner.Cancel(runID) {
		return c.JSON(fiber.Map{"status": "cancelling"})
	}

	// Not in runner (maybe still queued) — mark directly
	_ = h.store.UpdateRunResult(c.Context(), sqlc.UpdateRunResultParams{
		Status:      "cancelled",
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		ID:          runID,
	})

	return c.JSON(fiber.Map{"status": "cancelled"})
}

// ListChildren returns child runs (sub-agent runs) for a parent run.
func (h *RunHandler) ListChildren(c *fiber.Ctx) error {
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

	children, err := h.store.ListChildRuns(c.Context(), sql.NullString{String: runID, Valid: true})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list children"})
	}

	return c.JSON(fiber.Map{"children": runsToDTOs(children)})
}
