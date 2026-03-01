package handlers

import (
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/metrics"
)

// HealthHandler provides health and metrics endpoints.
type HealthHandler struct {
	store     *db.Store
	startedAt time.Time
}

func NewHealthHandler(store *db.Store) *HealthHandler {
	return &HealthHandler{store: store, startedAt: time.Now()}
}

// Healthz is a simple liveness probe.
func (h *HealthHandler) Healthz(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// Readyz checks DB connectivity.
func (h *HealthHandler) Readyz(c *fiber.Ctx) error {
	if err := h.store.Ping(c.Context()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unavailable",
			"error":  err.Error(),
		})
	}
	return c.JSON(fiber.Map{"status": "ready"})
}

func (h *HealthHandler) Metrics(c *fiber.Ctx) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startedAt)

	return c.JSON(fiber.Map{
		"uptime_seconds":   int(uptime.Seconds()),
		"goroutines":       runtime.NumGoroutine(),
		"alloc_mb":         float64(m.Alloc) / 1024 / 1024,
		"sys_mb":           float64(m.Sys) / 1024 / 1024,
		"gc_cycles":        m.NumGC,
		"runs_started":     metrics.Global.RunsStarted.Load(),
		"runs_finished":    metrics.Global.RunsFinished.Load(),
		"runs_failed":      metrics.Global.RunsFailed.Load(),
	})
}
