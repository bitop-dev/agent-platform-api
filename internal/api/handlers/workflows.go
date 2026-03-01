package handlers

import (
	"database/sql"
	"encoding/json"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	sqlc "github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/orchestrator"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// WorkflowHandler manages AI Team workflows.
type WorkflowHandler struct {
	store *sqlc.Queries
	orch  *orchestrator.Orchestrator
	audit *audit.Logger
}

func NewWorkflowHandler(store *sqlc.Queries, orch *orchestrator.Orchestrator) *WorkflowHandler {
	return &WorkflowHandler{store: store, orch: orch, audit: audit.NewLogger(store)}
}

// Create creates a new workflow with steps.
func (h *WorkflowHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		TeamID      string `json:"team_id"`
		Steps       []struct {
			AgentID         string   `json:"agent_id"`
			Name            string   `json:"name"`
			MissionTemplate string   `json:"mission_template"`
			DependsOn       []string `json:"depends_on"` // step names
		} `json:"steps"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" || len(req.Steps) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name and at least one step required"})
	}

	// Create workflow
	var teamID sql.NullString
	if req.TeamID != "" {
		teamID = sql.NullString{String: req.TeamID, Valid: true}
	}

	wf, err := h.store.CreateWorkflow(c.Context(), sqlc.CreateWorkflowParams{
		ID:          uuid.New().String(),
		UserID:      userID,
		TeamID:      teamID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create workflow"})
	}

	// Create steps
	steps := make([]sqlc.WorkflowStep, 0, len(req.Steps))
	for i, s := range req.Steps {
		depsJSON, _ := json.Marshal(s.DependsOn)
		step, err := h.store.CreateWorkflowStep(c.Context(), sqlc.CreateWorkflowStepParams{
			ID:              uuid.New().String(),
			WorkflowID:      wf.ID,
			AgentID:         s.AgentID,
			Name:            s.Name,
			Position:        int64(i),
			MissionTemplate: s.MissionTemplate,
			DependsOn:       string(depsJSON),
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create step: " + err.Error()})
		}
		steps = append(steps, step)
	}

	h.audit.Log(c.Context(), userID, audit.ActionWorkflowCreate, wf.ID, c.IP(), map[string]any{
		"name":  req.Name,
		"steps": len(req.Steps),
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workflow": wf, "steps": steps})
}

// Get returns a workflow with its steps.
func (h *WorkflowHandler) Get(c *fiber.Ctx) error {
	wfID := c.Params("id")

	wf, err := h.store.GetWorkflow(c.Context(), wfID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "workflow not found"})
	}

	steps, err := h.store.ListWorkflowSteps(c.Context(), wfID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list steps"})
	}

	return c.JSON(fiber.Map{"workflow": wf, "steps": steps})
}

// List returns all workflows for the current user.
func (h *WorkflowHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	wfs, err := h.store.ListWorkflowsByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list workflows"})
	}

	return c.JSON(fiber.Map{"workflows": wfs})
}

// Update updates workflow name/description/enabled.
func (h *WorkflowHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	wfID := c.Params("id")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	err := h.store.UpdateWorkflow(c.Context(), sqlc.UpdateWorkflowParams{
		Name: req.Name, Description: req.Description, Enabled: req.Enabled,
		ID: wfID, UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update"})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

// Delete removes a workflow.
func (h *WorkflowHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	wfID := c.Params("id")

	err := h.store.DeleteWorkflow(c.Context(), sqlc.DeleteWorkflowParams{ID: wfID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "workflow not found"})
	}

	h.audit.Log(c.Context(), userID, audit.ActionWorkflowDelete, wfID, c.IP(), nil)

	return c.JSON(fiber.Map{"status": "deleted"})
}

// Run triggers execution of a workflow.
func (h *WorkflowHandler) Run(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	wfID := c.Params("id")

	var req struct {
		Input string `json:"input"` // the mission/prompt for this workflow run
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	wfRun, err := h.orch.StartWorkflow(c.Context(), wfID, userID, req.Input)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit.Log(c.Context(), userID, audit.ActionWorkflowRun, wfRun.ID, c.IP(), map[string]any{
		"workflow_id": wfID,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workflow_run": wfRun})
}

// GetRun returns a workflow run with its step statuses.
func (h *WorkflowHandler) GetRun(c *fiber.Ctx) error {
	runID := c.Params("run_id")

	wfRun, err := h.store.GetWorkflowRun(c.Context(), runID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "workflow run not found"})
	}

	stepRuns, err := h.store.ListStepRuns(c.Context(), runID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list step runs"})
	}

	return c.JSON(fiber.Map{"workflow_run": wfRun, "step_runs": stepRuns})
}

// ListRuns returns all runs for a workflow.
func (h *WorkflowHandler) ListRuns(c *fiber.Ctx) error {
	wfID := c.Params("id")

	runs, err := h.store.ListWorkflowRuns(c.Context(), sqlc.ListWorkflowRunsParams{
		WorkflowID: wfID, Limit: 50, Offset: 0,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list runs"})
	}

	return c.JSON(fiber.Map{"workflow_runs": runs})
}
