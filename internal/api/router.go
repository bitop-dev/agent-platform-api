// Package api sets up the Fiber router with all routes and middleware.
package api

import (
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/bitop-dev/agent-platform-api/internal/api/handlers"
	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/config"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/orchestrator"
	"github.com/bitop-dev/agent-platform-api/internal/registry"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/scheduler"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

// NewRouter creates the Fiber app with all routes configured.
func NewRouter(store *db.Store, a *auth.Auth, enc *auth.Encryptor, r *runner.Runner, hub *ws.Hub, syncer *registry.Syncer, sched *scheduler.Scheduler, orch *orchestrator.Orchestrator, cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "agent-platform-api",
		ErrorHandler: errorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${locals:request_id}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
	}))

	// Health / Readiness / Metrics (unauthenticated)
	healthHandler := handlers.NewHealthHandler(store)
	app.Get("/health", healthHandler.Healthz)
	app.Get("/healthz", healthHandler.Healthz)
	app.Get("/readyz", healthHandler.Readyz)
	app.Get("/metrics", healthHandler.Metrics)

	// Rate limiters
	authLimiter := middleware.NewRateLimiter(10, time.Minute)   // 10 auth attempts/min
	apiLimiter := middleware.NewRateLimiter(120, time.Minute)   // 120 API calls/min

	// --- Public routes ---
	app.Get("/api/v1/models", handlers.ListModels)

	authHandler := handlers.NewAuthHandler(store, a)
	authGroup := app.Group("/api/v1/auth", authLimiter.Middleware())
	authGroup.Post("/register", authHandler.Register)
	authGroup.Post("/login", authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)

	// OAuth routes (public — redirects to providers)
	oauthHandler := handlers.NewOAuthHandler(store, a, cfg)
	authGroup.Get("/github", oauthHandler.GitHubLogin)
	authGroup.Get("/github/callback", oauthHandler.GitHubCallback)
	authGroup.Get("/google", oauthHandler.GoogleLogin)
	authGroup.Get("/google/callback", oauthHandler.GoogleCallback)

	// --- Protected routes ---
	api := app.Group("/api/v1", apiLimiter.Middleware(), middleware.AuthMiddleware(a))

	// Dashboard
	dashHandler := handlers.NewDashboardHandler(store)
	api.Get("/dashboard", dashHandler.Stats)

	// User info
	api.Get("/me", func(c *fiber.Ctx) error {
		userID := middleware.GetUserID(c)
		user, err := store.GetUserByID(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.JSON(fiber.Map{
			"id":             user.ID,
			"email":          user.Email,
			"name":           user.Name,
			"avatar_url":     user.AvatarUrl.String,
			"oauth_provider": user.OauthProvider.String,
			"created_at":     user.CreatedAt,
		})
	})

	// Agents
	agentHandler := handlers.NewAgentHandler(store)
	api.Post("/agents", agentHandler.Create)
	api.Get("/agents", agentHandler.List)
	api.Get("/agents/:id", agentHandler.Get)
	api.Put("/agents/:id", agentHandler.Update)
	api.Delete("/agents/:id", agentHandler.Delete)
	api.Put("/agents/:id/team", agentHandler.SetTeam)

	// API Keys
	apiKeyHandler := handlers.NewAPIKeyHandler(store, enc)
	api.Post("/api-keys", apiKeyHandler.Create)
	api.Get("/api-keys", apiKeyHandler.List)
	api.Put("/api-keys/:id", apiKeyHandler.Update)
	api.Delete("/api-keys/:id", apiKeyHandler.Delete)

	// Credentials (skill secrets — GITHUB_TOKEN, SLACK_WEBHOOK_URL, etc.)
	credHandler := handlers.NewCredentialHandler(store.Queries, enc)
	api.Post("/credentials", credHandler.Create)
	api.Get("/credentials", credHandler.List)
	api.Put("/credentials/:id", credHandler.Update)
	api.Delete("/credentials/:id", credHandler.Delete)

	// Skills
	skillHandler := handlers.NewSkillHandler(store)
	api.Post("/skills", skillHandler.Create)
	api.Get("/skills", skillHandler.List)
	api.Get("/skills/:id", skillHandler.Get)
	api.Put("/skills/:id", skillHandler.Update)
	api.Delete("/skills/:id", skillHandler.Delete)
	api.Post("/agents/:id/skills", skillHandler.AttachToAgent)
	api.Delete("/agents/:id/skills/:skill_id", skillHandler.DetachFromAgent)
	api.Get("/agents/:id/skills", skillHandler.ListAgentSkills)

	// Skill Sources
	srcHandler := handlers.NewSkillSourceHandler(store.Queries, syncer)
	api.Get("/skill-sources", srcHandler.List)
	api.Post("/skill-sources", srcHandler.Create)
	api.Delete("/skill-sources/:id", srcHandler.Delete)
	api.Post("/skill-sources/:id/sync", srcHandler.Sync)
	api.Post("/skill-sources/sync", srcHandler.SyncAll)

	// Runs
	runHandler := handlers.NewRunHandler(store, r, enc)
	api.Post("/runs", runHandler.Create)
	api.Get("/runs", runHandler.List)
	api.Get("/runs/:id", runHandler.Get)
	api.Get("/agents/:agent_id/runs", runHandler.ListByAgent)
	api.Get("/runs/:id/events", runHandler.Events)
	api.Post("/runs/:id/cancel", runHandler.Cancel)

	// Schedules
	schedHandler := handlers.NewScheduleHandler(store, sched, r, enc)
	api.Post("/schedules", schedHandler.Create)
	api.Get("/schedules", schedHandler.List)
	api.Get("/schedules/:id", schedHandler.Get)
	api.Put("/schedules/:id", schedHandler.Update)
	api.Delete("/schedules/:id", schedHandler.Delete)
	api.Post("/schedules/:id/enable", schedHandler.Enable)
	api.Post("/schedules/:id/disable", schedHandler.Disable)
	api.Post("/schedules/:id/trigger", schedHandler.Trigger)
	api.Get("/agents/:agent_id/schedules", schedHandler.ListByAgent)

	// Teams
	teamHandler := handlers.NewTeamHandler(store)
	api.Post("/teams", teamHandler.Create)
	api.Get("/teams", teamHandler.List)
	api.Get("/teams/:id", teamHandler.Get)
	api.Delete("/teams/:id", teamHandler.Delete)
	api.Get("/teams/:id/members", teamHandler.ListMembers)
	api.Post("/teams/:id/invitations", teamHandler.Invite)
	api.Post("/invitations/:invitation_id/accept", teamHandler.AcceptInvitation)
	api.Delete("/teams/:id/members/:user_id", teamHandler.RemoveMember)

	// Workflows (AI Teams)
	wfHandler := handlers.NewWorkflowHandler(store.Queries, orch)
	api.Post("/workflows", wfHandler.Create)
	api.Get("/workflows", wfHandler.List)
	api.Get("/workflows/:id", wfHandler.Get)
	api.Put("/workflows/:id", wfHandler.Update)
	api.Delete("/workflows/:id", wfHandler.Delete)
	api.Post("/workflows/:id/run", wfHandler.Run)
	api.Get("/workflows/:id/runs", wfHandler.ListRuns)
	api.Get("/workflow-runs/:run_id", wfHandler.GetRun)

	// Audit log
	auditHandler := handlers.NewAuditHandler(store)
	api.Get("/audit-log", auditHandler.List)

	// Child runs (orchestration)
	api.Get("/runs/:id/children", runHandler.ListChildren)

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
