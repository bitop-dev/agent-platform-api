-- +goose Up

CREATE TABLE skill_sources (
    id          TEXT PRIMARY KEY,
    user_id     TEXT REFERENCES users(id),
    url         TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    is_default  BOOLEAN NOT NULL DEFAULT false,
    last_synced TIMESTAMP,
    skill_count INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending',
    error_msg   TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_skill_sources_user_url ON skill_sources(user_id, url);

-- Add source_id to skills so we know which source each skill came from
ALTER TABLE skills ADD COLUMN source_id TEXT REFERENCES skill_sources(id);

-- +goose Down

-- SQLite doesn't support DROP COLUMN, but goose handles this
ALTER TABLE skills DROP COLUMN source_id;
DROP TABLE IF EXISTS skill_sources;
