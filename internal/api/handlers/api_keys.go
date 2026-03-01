package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// APIKeyHandler handles LLM provider API key management.
type APIKeyHandler struct {
	store     *db.Store
	encryptor *auth.Encryptor
}

func NewAPIKeyHandler(store *db.Store, enc *auth.Encryptor) *APIKeyHandler {
	return &APIKeyHandler{store: store, encryptor: enc}
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
