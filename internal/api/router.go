// Package api sets up the Fiber router with all routes and middleware.
package api

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/bitop-dev/agent-platform-api/internal/api/handlers"
	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

// NewRouter creates the Fiber app with all routes configured.
func NewRouter(store *db.Store, a *auth.Auth, enc *auth.Encryptor, r *runner.Runner, hub *ws.Hub) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "agent-platform-api",
		ErrorHandler: errorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(fiberlogger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// --- Public routes ---
	authHandler := handlers.NewAuthHandler(store, a)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)

	// --- Protected routes ---
	api := app.Group("/api/v1", middleware.AuthMiddleware(a))

	// Agents
	agentHandler := handlers.NewAgentHandler(store)
	api.Post("/agents", agentHandler.Create)
	api.Get("/agents", agentHandler.List)
	api.Get("/agents/:id", agentHandler.Get)
	api.Put("/agents/:id", agentHandler.Update)
	api.Delete("/agents/:id", agentHandler.Delete)

	// API Keys
	apiKeyHandler := handlers.NewAPIKeyHandler(store, enc)
	api.Post("/api-keys", apiKeyHandler.Create)
	api.Get("/api-keys", apiKeyHandler.List)
	api.Delete("/api-keys/:id", apiKeyHandler.Delete)

	// Runs
	runHandler := handlers.NewRunHandler(store, r, enc)
	api.Post("/runs", runHandler.Create)
	api.Get("/runs/:id", runHandler.Get)
	api.Get("/agents/:agent_id/runs", runHandler.List)
	api.Get("/runs/:id/events", runHandler.Events)

	// WebSocket — stream run events in real time
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws/runs/:id", websocket.New(func(conn *websocket.Conn) {
		runID := conn.Params("id")
		hub.Subscribe(runID, conn)
		defer hub.Unsubscribe(runID, conn)

		// Keep connection alive — read messages (ping/pong handled by Fiber)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))

	return app
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
