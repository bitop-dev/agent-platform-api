-- +goose Up
-- Phase 8: Hardening — audit log for observability
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     TEXT REFERENCES users(id),
    action      TEXT NOT NULL,         -- 'agent.create', 'run.start', 'schedule.trigger', etc.
    resource_id TEXT,                   -- ID of the affected resource
    metadata    TEXT,                   -- JSON blob with extra context
    ip_address  TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_log_user ON audit_log(user_id, created_at);
CREATE INDEX idx_audit_log_action ON audit_log(action, created_at);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
