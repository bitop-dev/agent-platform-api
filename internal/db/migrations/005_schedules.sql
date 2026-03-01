-- +goose Up
CREATE TABLE schedules (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    schedule_type   TEXT NOT NULL DEFAULT 'cron',   -- cron | every | once
    cron_expr       TEXT NOT NULL DEFAULT '',        -- e.g. "0 9 * * 1-5"
    interval_seconds INTEGER NOT NULL DEFAULT 0,    -- for "every" type
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    mission         TEXT NOT NULL DEFAULT '',        -- override mission (empty = use agent default)
    enabled         BOOLEAN NOT NULL DEFAULT true,
    overlap_policy  TEXT NOT NULL DEFAULT 'skip',    -- skip | queue | parallel
    max_retries     INTEGER NOT NULL DEFAULT 3,
    next_run_at     TIMESTAMP,
    last_run_at     TIMESTAMP,
    last_run_status TEXT,
    last_run_id     TEXT,
    last_error      TEXT,
    consecutive_errors INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_schedules_user ON schedules(user_id);
CREATE INDEX idx_schedules_agent ON schedules(agent_id);
CREATE INDEX idx_schedules_next_run ON schedules(next_run_at) WHERE enabled = true;

-- +goose Down
DROP TABLE IF EXISTS schedules;
