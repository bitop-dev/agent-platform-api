package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	sqlc "github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/bitop-dev/agent-platform-api/internal/registry"
)

func toNullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// SkillSourceHandler handles /api/v1/skill-sources endpoints.
type SkillSourceHandler struct {
	store  *sqlc.Queries
	syncer *registry.Syncer
}

// NewSkillSourceHandler creates a new handler.
func NewSkillSourceHandler(store *sqlc.Queries, syncer *registry.Syncer) *SkillSourceHandler {
	return &SkillSourceHandler{store: store, syncer: syncer}
}

// SkillSourceDTO is the API response for a skill source.
type SkillSourceDTO struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	Label      string  `json:"label"`
	IsDefault  bool    `json:"is_default"`
	Status     string  `json:"status"`
	SkillCount int64   `json:"skill_count"`
	ErrorMsg   *string `json:"error_msg,omitempty"`
	LastSynced *string `json:"last_synced,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func sourceToDTO(s sqlc.SkillSource) SkillSourceDTO {
	dto := SkillSourceDTO{
		ID:         s.ID,
		URL:        s.Url,
		Label:      s.Label,
		IsDefault:  s.IsDefault,
		Status:     s.Status,
		SkillCount: s.SkillCount,
		CreatedAt:  s.CreatedAt.String(),
	}
	if s.ErrorMsg.Valid {
		dto.ErrorMsg = &s.ErrorMsg.String
	}
	if s.LastSynced.Valid {
		t := s.LastSynced.Time.String()
		dto.LastSynced = &t
	}
	return dto
}

// List returns all skill sources visible to the user (their own + system defaults).
func (h *SkillSourceHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	sources, err := h.store.ListSkillSourcesByUser(c.Context(), toNullStr(userID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list sources"})
	}

	dtos := make([]SkillSourceDTO, len(sources))
	for i, s := range sources {
		dtos[i] = sourceToDTO(s)
	}
	return c.JSON(fiber.Map{"skill_sources": dtos})
}

// Create adds a new skill source and triggers a sync.
func (h *SkillSourceHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var body struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if body.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "url is required"})
	}
	if body.Label == "" {
		body.Label = body.URL
	}

	src, err := h.store.CreateSkillSource(c.Context(), sqlc.CreateSkillSourceParams{
		ID:        uuid.NewString(),
		UserID:    toNullStr(userID),
		Url:       body.URL,
		Label:     body.Label,
		IsDefault: false,
	})
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "source already exists"})
	}

	// Sync in background
	go func() {
		_, _ = h.syncer.SyncSource(c.Context(), src)
	}()

	return c.Status(fiber.StatusCreated).JSON(sourceToDTO(src))
}

// Delete removes a non-default skill source and its skills (if not attached to agents).
func (h *SkillSourceHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")

	src, err := h.store.GetSkillSource(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "source not found"})
	}
	if src.IsDefault {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot delete the default source"})
	}

	// Delete unattached skills from this source
	_ = h.store.DeleteSkillsBySource(c.Context(), toNullStr(id))

	if err := h.store.DeleteSkillSource(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete"})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// Sync triggers a re-sync of a specific source.
func (h *SkillSourceHandler) Sync(c *fiber.Ctx) error {
	id := c.Params("id")

	src, err := h.store.GetSkillSource(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "source not found"})
	}

	// Sync in background
	go func() {
		_, _ = h.syncer.SyncSource(c.Context(), src)
	}()

	return c.JSON(fiber.Map{"message": "sync started"})
}

// SyncAll triggers a re-sync of all sources.
func (h *SkillSourceHandler) SyncAll(c *fiber.Ctx) error {
	go func() {
		_ = h.syncer.SyncAll(c.Context())
	}()

	return c.JSON(fiber.Map{"message": "sync started for all sources"})
}
