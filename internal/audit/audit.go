// Package audit provides structured audit logging for user actions.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// Action constants for audit entries.
const (
	ActionLogin          = "auth.login"
	ActionRegister       = "auth.register"
	ActionOAuthLogin     = "auth.oauth_login"
	ActionAgentCreate    = "agent.create"
	ActionAgentUpdate    = "agent.update"
	ActionAgentDelete    = "agent.delete"
	ActionRunCreate      = "run.create"
	ActionRunCancel      = "run.cancel"
	ActionAPIKeyCreate   = "api_key.create"
	ActionAPIKeyDelete   = "api_key.delete"
	ActionSkillAttach    = "skill.attach"
	ActionSkillDetach      = "skill.detach"
	ActionCredentialCreate = "credential.create"
	ActionCredentialDelete = "credential.delete"
	ActionScheduleCreate = "schedule.create"
	ActionScheduleUpdate = "schedule.update"
	ActionScheduleDelete = "schedule.delete"
	ActionTeamCreate     = "team.create"
	ActionTeamInvite     = "team.invite"
	ActionTeamRemove     = "team.remove_member"
)

// Logger writes audit entries to the database.
type Logger struct {
	q *sqlc.Queries
}

// NewLogger creates an audit logger.
func NewLogger(q *sqlc.Queries) *Logger {
	return &Logger{q: q}
}

// Log writes an audit entry. Metadata is optional key-value pairs.
func (l *Logger) Log(ctx context.Context, userID, action, resourceID, ipAddress string, meta map[string]any) {
	var metaJSON sql.NullString
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = sql.NullString{String: string(b), Valid: true}
		}
	}

	err := l.q.InsertAuditLog(ctx, sqlc.InsertAuditLogParams{
		UserID:     sql.NullString{String: userID, Valid: userID != ""},
		Action:     action,
		ResourceID: sql.NullString{String: resourceID, Valid: resourceID != ""},
		Metadata:   metaJSON,
		IpAddress:  sql.NullString{String: ipAddress, Valid: ipAddress != ""},
	})
	if err != nil {
		slog.Error("audit log write failed", "action", action, "user_id", userID, "error", err)
	}
}
