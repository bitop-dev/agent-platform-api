package handlers

import (
	"fmt"
	"runtime"
	"strings"
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

// Metrics returns Prometheus text exposition format metrics.
func (h *HealthHandler) Metrics(c *fiber.Ctx) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startedAt).Seconds()

	var b strings.Builder

	prom := func(name, help, typ string, value any) {
		fmt.Fprintf(&b, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
		switch v := value.(type) {
		case float64:
			fmt.Fprintf(&b, "%s %f\n", name, v)
		default:
			fmt.Fprintf(&b, "%s %v\n", name, v)
		}
	}

	prom("agentops_uptime_seconds", "Seconds since server start.", "gauge", uptime)
	prom("agentops_goroutines", "Number of active goroutines.", "gauge", runtime.NumGoroutine())
	prom("agentops_alloc_bytes", "Bytes of allocated heap.", "gauge", m.Alloc)
	prom("agentops_sys_bytes", "Total bytes obtained from system.", "gauge", m.Sys)
	prom("agentops_gc_cycles_total", "Total GC cycles.", "counter", m.NumGC)
	prom("agentops_runs_started_total", "Total runs started.", "counter", metrics.Global.RunsStarted.Load())
	prom("agentops_runs_finished_total", "Total runs completed successfully.", "counter", metrics.Global.RunsFinished.Load())
	prom("agentops_runs_failed_total", "Total runs that failed.", "counter", metrics.Global.RunsFailed.Load())

	c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	return c.SendString(b.String())
}
