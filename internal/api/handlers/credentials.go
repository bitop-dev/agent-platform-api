package handlers

import (
	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	sqlc "github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CredentialHandler manages user skill credentials.
type CredentialHandler struct {
	store *sqlc.Queries
	enc   *auth.Encryptor
	audit *audit.Logger
}

func NewCredentialHandler(store *sqlc.Queries, enc *auth.Encryptor) *CredentialHandler {
	return &CredentialHandler{store: store, enc: enc, audit: audit.NewLogger(store)}
}

// Create stores a new encrypted credential.
func (h *CredentialHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		Name        string `json:"name"`        // e.g., GITHUB_TOKEN
		Value       string `json:"value"`       // the secret value
		SkillName   string `json:"skill_name"`  // optional: scope to skill
		Description string `json:"description"` // user-facing label
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" || req.Value == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name and value are required"})
	}

	enc, err := h.enc.Encrypt(req.Value)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to encrypt credential"})
	}

	cred, err := h.store.CreateCredential(c.Context(), sqlc.CreateCredentialParams{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        req.Name,
		ValueEnc:    enc,
		ValueHint:   auth.KeyHint(req.Value),
		SkillName:   req.SkillName,
		Description: req.Description,
	})
	if err != nil {
		// Unique constraint violation = credential already exists
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "credential already exists for this name/skill combination",
		})
	}

	h.audit.Log(c.Context(), userID, audit.ActionCredentialCreate, cred.ID, c.IP(), map[string]any{
		"name":       req.Name,
		"skill_name": req.SkillName,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"credential": map[string]any{
			"id":          cred.ID,
			"name":        cred.Name,
			"value_hint":  cred.ValueHint,
			"skill_name":  cred.SkillName,
			"description": cred.Description,
			"created_at":  cred.CreatedAt,
		},
	})
}

// List returns all credentials for the current user (values hidden).
func (h *CredentialHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	creds, err := h.store.ListCredentialsByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list credentials"})
	}

	return c.JSON(fiber.Map{"credentials": creds})
}

// Update replaces the value of an existing credential.
func (h *CredentialHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	credID := c.Params("id")

	var req struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Verify ownership
	existing, err := h.store.GetCredential(c.Context(), sqlc.GetCredentialParams{ID: credID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "credential not found"})
	}

	// If value provided, re-encrypt
	valueEnc := existing.ValueEnc
	valueHint := existing.ValueHint
	if req.Value != "" {
		valueEnc, err = h.enc.Encrypt(req.Value)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to encrypt"})
		}
		valueHint = auth.KeyHint(req.Value)
	}

	desc := existing.Description
	if req.Description != "" {
		desc = req.Description
	}

	err = h.store.UpdateCredential(c.Context(), sqlc.UpdateCredentialParams{
		ValueEnc:    valueEnc,
		ValueHint:   valueHint,
		Description: desc,
		ID:          credID,
		UserID:      userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update credential"})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

// Delete removes a credential.
func (h *CredentialHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	credID := c.Params("id")

	err := h.store.DeleteCredential(c.Context(), sqlc.DeleteCredentialParams{ID: credID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "credential not found"})
	}

	h.audit.Log(c.Context(), userID, audit.ActionCredentialDelete, credID, c.IP(), nil)

	return c.JSON(fiber.Map{"status": "deleted"})
}
