-- +goose Up

-- User credentials for skill tools (e.g., GITHUB_TOKEN, SLACK_WEBHOOK_URL).
-- Values are AES-256-GCM encrypted at rest, same as api_keys.
-- Scoped by user + credential name. Optionally scoped to a specific skill.
CREATE TABLE user_credentials (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,              -- credential name (e.g., GITHUB_TOKEN)
    value_enc   BLOB NOT NULL,             -- encrypted value
    value_hint  TEXT NOT NULL DEFAULT '',   -- last 4 chars for display
    skill_name  TEXT NOT NULL DEFAULT '',   -- scope to skill (empty = available to all skills)
    description TEXT NOT NULL DEFAULT '',   -- user-facing label
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name, skill_name)
);

-- +goose Down
DROP TABLE IF EXISTS user_credentials;
