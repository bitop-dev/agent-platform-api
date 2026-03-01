package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// DashboardHandler provides aggregate stats for the dashboard UI.
type DashboardHandler struct {
	store *db.Store
}

func NewDashboardHandler(store *db.Store) *DashboardHandler {
	return &DashboardHandler{store: store}
}

// Stats returns dashboard overview data.
func (h *DashboardHandler) Stats(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	// Agent count
	agentCount, err := h.store.CountAgentsByUser(c.Context(), userID)
	if err != nil {
		agentCount = 0
	}

	// Run status counts
	statusCounts, _ := h.store.CountRunsByStatus(c.Context(), userID)
	statuses := make(map[string]int64)
	var totalRuns int64
	for _, sc := range statusCounts {
		statuses[sc.Status] = sc.Count
		totalRuns += sc.Count
	}

	// Recent runs
	recent, _ := h.store.RecentRuns(c.Context(), sqlc.RecentRunsParams{
		UserID: userID,
		Limit:  5,
	})

	return c.JSON(fiber.Map{
		"agents":      agentCount,
		"total_runs":  totalRuns,
		"run_status":  statuses,
		"recent_runs": recentRunsToDTOs(recent),
	})
}
