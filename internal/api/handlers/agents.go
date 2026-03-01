package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// AgentHandler handles agent CRUD.
type AgentHandler struct {
	store *db.Store
}

func NewAgentHandler(store *db.Store) *AgentHandler {
	return &AgentHandler{store: store}
}

type createAgentRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	SystemPrompt  string `json:"system_prompt"`
	ModelProvider string `json:"model_provider"`
	ModelName     string `json:"model_name"`
	ConfigYAML    string `json:"config_yaml"`
	MaxTurns      int    `json:"max_turns"`
	Timeout       int    `json:"timeout_seconds"`
}

type updateAgentRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	SystemPrompt  string `json:"system_prompt"`
	ModelProvider string `json:"model_provider"`
	ModelName     string `json:"model_name"`
	ConfigYAML    string `json:"config_yaml"`
	MaxTurns      int    `json:"max_turns"`
	Timeout       int    `json:"timeout_seconds"`
	Enabled       bool   `json:"enabled"`
}

// Create creates a new agent.
func (h *AgentHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	var req createAgentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" || req.SystemPrompt == "" || req.ModelName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name, system_prompt, and model_name are required"})
	}

	if req.ModelProvider == "" {
		req.ModelProvider = "openai"
	}
	if req.MaxTurns == 0 {
		req.MaxTurns = 20
	}
	if req.Timeout == 0 {
		req.Timeout = 300
	}

	agent, err := h.store.CreateAgent(c.Context(), sqlc.CreateAgentParams{
		ID:             uuid.NewString(),
		UserID:         userID,
		Name:           req.Name,
		Description:    sql.NullString{String: req.Description, Valid: req.Description != ""},
		SystemPrompt:   req.SystemPrompt,
		ModelProvider:   req.ModelProvider,
		ModelName:       req.ModelName,
		ConfigYaml:     req.ConfigYAML,
		MaxTurns:       int64(req.MaxTurns),
		TimeoutSeconds: int64(req.Timeout),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create agent"})
	}

	return c.Status(fiber.StatusCreated).JSON(agent)
}

// List returns all agents for the authenticated user.
func (h *AgentHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	agents, err := h.store.ListAgentsByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list agents"})
	}

	return c.JSON(fiber.Map{"agents": agents})
}

// Get returns a single agent by ID.
func (h *AgentHandler) Get(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}

	if agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(agent)
}

// Update updates an existing agent.
func (h *AgentHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}
	if agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req updateAgentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	err = h.store.UpdateAgent(c.Context(), sqlc.UpdateAgentParams{
		Name:           req.Name,
		Description:    sql.NullString{String: req.Description, Valid: req.Description != ""},
		SystemPrompt:   req.SystemPrompt,
		ModelProvider:   req.ModelProvider,
		ModelName:       req.ModelName,
		ConfigYaml:     req.ConfigYAML,
		MaxTurns:       int64(req.MaxTurns),
		TimeoutSeconds: int64(req.Timeout),
		Enabled:        req.Enabled,
		ID:             agentID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update agent"})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

// Delete removes an agent.
func (h *AgentHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}
	if agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	if err := h.store.DeleteAgent(c.Context(), agentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete agent"})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}
