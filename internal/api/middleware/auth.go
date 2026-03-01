package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
)

// AuthMiddleware validates JWT tokens and injects claims into context.
func AuthMiddleware(a *auth.Auth) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		token := strings.TrimPrefix(header, "Bearer ")
		if token == header {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization format (expected Bearer <token>)",
			})
		}

		claims, err := a.ValidateToken(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		return c.Next()
	}
}

// GetUserID extracts the authenticated user ID from Fiber context.
func GetUserID(c *fiber.Ctx) string {
	if id, ok := c.Locals("user_id").(string); ok {
		return id
	}
	return ""
}
