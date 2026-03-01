package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// AuditHandler serves audit log entries.
type AuditHandler struct {
	store *db.Store
}

func NewAuditHandler(store *db.Store) *AuditHandler {
	return &AuditHandler{store: store}
}

// List returns paginated audit log entries for the current user.
func (h *AuditHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 50)
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	uid := sql.NullString{String: userID, Valid: true}
	entries, err := h.store.ListAuditLog(c.Context(), sqlc.ListAuditLogParams{
		UserID: uid,
		Limit:  int64(perPage),
		Offset: int64(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list audit log"})
	}

	total, _ := h.store.CountAuditLog(c.Context(), uid)

	return c.JSON(fiber.Map{
		"entries":  entries,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
