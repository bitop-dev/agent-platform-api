package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

type SkillHandler struct {
	store *db.Store
}

func NewSkillHandler(store *db.Store) *SkillHandler {
	return &SkillHandler{store: store}
}

type createSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	Version     string `json:"version"`
	SkillMD     string `json:"skill_md"`
	Tags        string `json:"tags"`
	SourceURL   string `json:"source_url"`
}

type updateSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SkillMD     string `json:"skill_md"`
	Tags        string `json:"tags"`
	Enabled     bool   `json:"enabled"`
}

// Create creates a new skill.
func (h *SkillHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req createSkillRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	if req.Tier == "" {
		req.Tier = "workspace"
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}

	skill, err := h.store.CreateSkill(c.Context(), sqlc.CreateSkillParams{
		ID:          uuid.NewString(),
		UserID:      sql.NullString{String: userID, Valid: true},
		Name:        req.Name,
		Description: req.Description,
		Tier:        req.Tier,
		Version:     req.Version,
		SkillMd:     req.SkillMD,
		Tags:        req.Tags,
		SourceUrl:   sql.NullString{String: req.SourceURL, Valid: req.SourceURL != ""},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create skill"})
	}

	return c.Status(fiber.StatusCreated).JSON(skillToDTO(skill))
}

// List returns all enabled skills (public browse).
func (h *SkillHandler) List(c *fiber.Ctx) error {
	tier := c.Query("tier")

	var skills []sqlc.Skill
	var err error
	if tier != "" {
		skills, err = h.store.ListSkillsByTier(c.Context(), tier)
	} else {
		skills, err = h.store.ListSkills(c.Context())
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list skills"})
	}

	return c.JSON(fiber.Map{"skills": skillsToDTOs(skills)})
}

// Get returns a single skill.
func (h *SkillHandler) Get(c *fiber.Ctx) error {
	skill, err := h.store.GetSkill(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "skill not found"})
	}

	return c.JSON(skillToDTO(skill))
}

// Update updates a skill (owner only).
func (h *SkillHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	skillID := c.Params("id")

	skill, err := h.store.GetSkill(c.Context(), skillID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "skill not found"})
	}
	if !skill.UserID.Valid || skill.UserID.String != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req updateSkillRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	err = h.store.UpdateSkill(c.Context(), sqlc.UpdateSkillParams{
		Name:        req.Name,
		Description: req.Description,
		SkillMd:     req.SkillMD,
		Tags:        req.Tags,
		Enabled:     req.Enabled,
		ID:          skillID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update skill"})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

// Delete removes a skill (owner only).
func (h *SkillHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	skillID := c.Params("id")

	skill, err := h.store.GetSkill(c.Context(), skillID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "skill not found"})
	}
	if !skill.UserID.Valid || skill.UserID.String != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	if err := h.store.DeleteSkill(c.Context(), skillID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete skill"})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

// AttachToAgent adds a skill to an agent.
func (h *SkillHandler) AttachToAgent(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req struct {
		SkillID  string `json:"skill_id"`
		Position int    `json:"position"`
		Config   string `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	err = h.store.AddAgentSkill(c.Context(), sqlc.AddAgentSkillParams{
		AgentID:    agentID,
		SkillID:    req.SkillID,
		Position:   int64(req.Position),
		ConfigJson: sql.NullString{String: req.Config, Valid: req.Config != ""},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to attach skill"})
	}

	return c.JSON(fiber.Map{"status": "attached"})
}

// DetachFromAgent removes a skill from an agent.
func (h *SkillHandler) DetachFromAgent(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	skillID := c.Params("skill_id")
	err = h.store.RemoveAgentSkill(c.Context(), sqlc.RemoveAgentSkillParams{
		AgentID: agentID,
		SkillID: skillID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to detach skill"})
	}

	return c.JSON(fiber.Map{"status": "detached"})
}

// ListAgentSkills returns skills attached to an agent.
func (h *SkillHandler) ListAgentSkills(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	agentID := c.Params("id")

	agent, err := h.store.GetAgent(c.Context(), agentID)
	if err != nil || agent.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	skills, err := h.store.ListAgentSkills(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list skills"})
	}

	return c.JSON(fiber.Map{"skills": skills})
}
