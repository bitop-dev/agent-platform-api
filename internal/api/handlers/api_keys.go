package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// APIKeyHandler handles LLM provider API key management.
type APIKeyHandler struct {
	store     *db.Store
	encryptor *auth.Encryptor
	audit     *audit.Logger
}

func NewAPIKeyHandler(store *db.Store, enc *auth.Encryptor) *APIKeyHandler {
	return &APIKeyHandler{store: store, encryptor: enc, audit: audit.NewLogger(store.Queries)}
}

type createAPIKeyRequest struct {
	Provider  string `json:"provider"`            // openai, anthropic, ollama
	Label     string `json:"label"`               // "My OpenAI Key"
	Key       string `json:"key"`                 // The actual API key (stored encrypted)
	IsDefault bool   `json:"is_default"`
	BaseURL   string `json:"base_url,omitempty"`  // Optional custom endpoint
}

// Create stores a new encrypted API key.
func (h *APIKeyHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req createAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Provider == "" || req.Key == "" || req.Label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider, label, and key are required"})
	}

	// Encrypt the key
	encrypted, err := h.encryptor.Encrypt(req.Key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "encryption error"})
	}

	// Clear existing default if setting this as default
	if req.IsDefault {
		_ = h.store.ClearDefaultAPIKey(c.Context(), sqlc.ClearDefaultAPIKeyParams{
			UserID:   userID,
			Provider: req.Provider,
		})
	}

	apiKey, err := h.store.CreateAPIKey(c.Context(), sqlc.CreateAPIKeyParams{
		ID:        uuid.NewString(),
		UserID:    userID,
		Provider:  req.Provider,
		Label:     req.Label,
		KeyEnc:    encrypted,
		KeyHint:   auth.KeyHint(req.Key),
		IsDefault: req.IsDefault,
		BaseUrl:   req.BaseURL,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to store key"})
	}

	h.audit.Log(c.Context(), userID, audit.ActionAPIKeyCreate, apiKey.ID, c.IP(), map[string]any{
		"provider": apiKey.Provider,
		"label":    apiKey.Label,
	})

	// Return without the encrypted key
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         apiKey.ID,
		"provider":   apiKey.Provider,
		"label":      apiKey.Label,
		"key_hint":   apiKey.KeyHint,
		"is_default": apiKey.IsDefault,
		"base_url":   apiKey.BaseUrl,
		"created_at": apiKey.CreatedAt,
	})
}

// List returns all API keys for the user (without the actual keys).
func (h *APIKeyHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	keys, err := h.store.ListAPIKeysByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list keys"})
	}

	return c.JSON(fiber.Map{"api_keys": keys})
}

// Update modifies an API key's label, base_url, default status, and optionally the key itself.
func (h *APIKeyHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	keyID := c.Params("id")

	var req struct {
		Label     string `json:"label"`
		Key       string `json:"key"`        // optional — if empty, key is not changed
		BaseURL   string `json:"base_url"`
		IsDefault bool   `json:"is_default"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Get existing key to know provider
	existing, err := h.store.GetAPIKey(c.Context(), sqlc.GetAPIKeyParams{ID: keyID, UserID: userID})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "key not found"})
	}

	if req.Label == "" {
		req.Label = existing.Label
	}

	// Clear existing default if setting this as default
	if req.IsDefault {
		_ = h.store.ClearDefaultAPIKey(c.Context(), sqlc.ClearDefaultAPIKeyParams{
			UserID:   userID,
			Provider: existing.Provider,
		})
	}

	if req.Key != "" {
		// Update with new encrypted key
		encrypted, err := h.encryptor.Encrypt(req.Key)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "encryption error"})
		}
		err = h.store.UpdateAPIKeyWithKey(c.Context(), sqlc.UpdateAPIKeyWithKeyParams{
			Label: req.Label, KeyEnc: encrypted, KeyHint: auth.KeyHint(req.Key),
			BaseUrl: req.BaseURL, IsDefault: req.IsDefault, ID: keyID, UserID: userID,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update"})
		}
	} else {
		// Update without changing key
		err = h.store.UpdateAPIKey(c.Context(), sqlc.UpdateAPIKeyParams{
			Label: req.Label, BaseUrl: req.BaseURL, IsDefault: req.IsDefault,
			ID: keyID, UserID: userID,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update"})
		}
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

// Delete removes an API key.
func (h *APIKeyHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	keyID := c.Params("id")

	err := h.store.DeleteAPIKey(c.Context(), sqlc.DeleteAPIKeyParams{
		ID:     keyID,
		UserID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "key not found"})
	}

	h.audit.Log(c.Context(), userID, audit.ActionAPIKeyDelete, keyID, c.IP(), nil)

	return c.JSON(fiber.Map{"status": "deleted"})
}

// GetDecryptedKey retrieves and decrypts an API key for use in runs.
// This is called internally by the runner, not exposed as an API endpoint.
func (h *APIKeyHandler) GetDecryptedKey(userID, provider string) (string, error) {
	apiKey, err := h.store.GetDefaultAPIKey(nil, sqlc.GetDefaultAPIKeyParams{
		UserID:   userID,
		Provider: provider,
	})
	if err != nil {
		return "", err
	}

	return h.encryptor.Decrypt(apiKey.KeyEnc)
}
