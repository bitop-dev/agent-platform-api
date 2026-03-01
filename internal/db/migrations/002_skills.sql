-- +goose Up

CREATE TABLE skills (
    id          TEXT PRIMARY KEY,
    user_id     TEXT REFERENCES users(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tier        TEXT NOT NULL DEFAULT 'workspace',
    version     TEXT NOT NULL DEFAULT '1.0.0',
    skill_md    TEXT NOT NULL DEFAULT '',
    tags        TEXT NOT NULL DEFAULT '',
    source_url  TEXT,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE agent_skills (
    agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    skill_id    TEXT NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    position    INTEGER NOT NULL DEFAULT 0,
    config_json TEXT,
    PRIMARY KEY (agent_id, skill_id)
);

CREATE INDEX idx_skills_tier ON skills(tier, enabled);

-- +goose Down

DROP TABLE IF EXISTS agent_skills;
DROP TABLE IF EXISTS skills;
