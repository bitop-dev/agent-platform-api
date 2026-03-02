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

// ── DTOs ────────────────────────────────────────────────────────────

type workflowDTO struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	TeamID      string `json:"team_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toWorkflowDTO(w sqlc.Workflow) workflowDTO {
	var tid string
	if w.TeamID.Valid {
		tid = w.TeamID.String
	}
	return workflowDTO{
		ID: w.ID, UserID: w.UserID, TeamID: tid,
		Name: w.Name, Description: w.Description, Enabled: w.Enabled,
		CreatedAt: w.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: w.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

type workflowRunDTO struct {
	ID           string  `json:"id"`
	WorkflowID   string  `json:"workflow_id"`
	UserID       string  `json:"user_id"`
	Status       string  `json:"status"`
	InputText    string  `json:"input_text"`
	OutputText   *string `json:"output_text"`
	ErrorMessage *string `json:"error_message"`
	CreatedAt    string  `json:"created_at"`
	StartedAt    *string `json:"started_at"`
	CompletedAt  *string `json:"completed_at"`
}

func toWorkflowRunDTO(r sqlc.WorkflowRun) workflowRunDTO {
	dto := workflowRunDTO{
		ID: r.ID, WorkflowID: r.WorkflowID, UserID: r.UserID,
		Status: r.Status, InputText: r.InputText,
		CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.OutputText.Valid {
		dto.OutputText = &r.OutputText.String
	}
	if r.ErrorMessage.Valid {
		dto.ErrorMessage = &r.ErrorMessage.String
	}
	if r.StartedAt.Valid {
		s := r.StartedAt.Time.Format("2006-01-02T15:04:05Z")
		dto.StartedAt = &s
	}
	if r.CompletedAt.Valid {
		s := r.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
		dto.CompletedAt = &s
	}
	return dto
}

type stepRunDTO struct {
	ID            string  `json:"id"`
	WorkflowRunID string  `json:"workflow_run_id"`
	StepID        string  `json:"step_id"`
	RunID         *string `json:"run_id"`
	Status        string  `json:"status"`
	StepName      string  `json:"step_name"`
	AgentID       string  `json:"agent_id"`
	DependsOn     string  `json:"depends_on"`
	StartedAt     *string `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
}

func toStepRunDTO(sr sqlc.ListStepRunsRow) stepRunDTO {
	dto := stepRunDTO{
		ID: sr.ID, WorkflowRunID: sr.WorkflowRunID, StepID: sr.StepID,
		Status: sr.Status, StepName: sr.StepName, AgentID: sr.AgentID,
		DependsOn: sr.DependsOn,
	}
	if sr.RunID.Valid {
		dto.RunID = &sr.RunID.String
	}
	if sr.StartedAt.Valid {
		s := sr.StartedAt.Time.Format("2006-01-02T15:04:05Z")
		dto.StartedAt = &s
	}
	if sr.CompletedAt.Valid {
		s := sr.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
		dto.CompletedAt = &s
	}
	return dto
}

type stepDTO struct {
	ID              string `json:"id"`
	WorkflowID      string `json:"workflow_id"`
	AgentID         string `json:"agent_id"`
	AgentName       string `json:"agent_name"`
	Name            string `json:"name"`
	Position        int64  `json:"position"`
	MissionTemplate string `json:"mission_template"`
	DependsOn       string `json:"depends_on"`
	CreatedAt       string `json:"created_at"`
}

func toStepDTO(s sqlc.ListWorkflowStepsRow) stepDTO {
	return stepDTO{
		ID: s.ID, WorkflowID: s.WorkflowID, AgentID: s.AgentID,
		AgentName: s.AgentName, Name: s.Name, Position: s.Position,
		MissionTemplate: s.MissionTemplate, DependsOn: s.DependsOn,
		CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ── Handler ─────────────────────────────────────────────────────────

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

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workflow": toWorkflowDTO(wf), "steps": steps})
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

	stepDTOs := make([]stepDTO, len(steps))
	for i, s := range steps {
		stepDTOs[i] = toStepDTO(s)
	}
	return c.JSON(fiber.Map{"workflow": toWorkflowDTO(wf), "steps": stepDTOs})
}

// List returns all workflows for the current user.
func (h *WorkflowHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	wfs, err := h.store.ListWorkflowsByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list workflows"})
	}

	dtos := make([]workflowDTO, len(wfs))
	for i, w := range wfs {
		dtos[i] = toWorkflowDTO(w)
	}
	return c.JSON(fiber.Map{"workflows": dtos})
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

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workflow_run": toWorkflowRunDTO(wfRun)})
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

	srDTOs := make([]stepRunDTO, len(stepRuns))
	for i, sr := range stepRuns {
		srDTOs[i] = toStepRunDTO(sr)
	}
	return c.JSON(fiber.Map{"workflow_run": toWorkflowRunDTO(wfRun), "step_runs": srDTOs})
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

	dtos := make([]workflowRunDTO, len(runs))
	for i, r := range runs {
		dtos[i] = toWorkflowRunDTO(r)
	}
	return c.JSON(fiber.Map{"workflow_runs": dtos})
}
