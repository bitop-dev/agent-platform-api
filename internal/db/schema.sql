CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    email       TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    password_hash TEXT,
    avatar_url  TEXT,
    oauth_provider TEXT,
    oauth_id    TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE teams (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT UNIQUE NOT NULL,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE team_members (
    team_id     TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE team_invitations (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'member',
    invited_by  TEXT NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL DEFAULT 'pending',
    expires_at  TIMESTAMP NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE api_keys (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    label       TEXT NOT NULL,
    key_enc     BLOB NOT NULL,
    key_hint    TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    base_url    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE agents (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id         TEXT REFERENCES teams(id),
    name            TEXT NOT NULL,
    description     TEXT,
    system_prompt   TEXT NOT NULL,
    model_provider  TEXT NOT NULL,
    model_name      TEXT NOT NULL,
    config_yaml     TEXT NOT NULL DEFAULT '',
    max_turns       INTEGER NOT NULL DEFAULT 20,
    timeout_seconds INTEGER NOT NULL DEFAULT 300,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agents(id),
    team_id         TEXT REFERENCES teams(id),
    parent_run_id   TEXT REFERENCES runs(id),
    depth           INTEGER NOT NULL DEFAULT 0,
    mission         TEXT NOT NULL,
    model_provider  TEXT NOT NULL,
    model_name      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    output_text     TEXT,
    error_message   TEXT,
    total_turns     INTEGER,
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    cost_usd        REAL,
    duration_ms     INTEGER,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP
);

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

CREATE TABLE skills (
    id          TEXT PRIMARY KEY,
    user_id     TEXT REFERENCES users(id),
    source_id   TEXT REFERENCES skill_sources(id),
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

CREATE TABLE run_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    event_type  TEXT NOT NULL,
    data_json   TEXT,
    occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE schedules (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    schedule_type   TEXT NOT NULL DEFAULT 'cron',
    cron_expr       TEXT NOT NULL DEFAULT '',
    interval_seconds INTEGER NOT NULL DEFAULT 0,
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    mission         TEXT NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    overlap_policy  TEXT NOT NULL DEFAULT 'skip',
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

CREATE TABLE user_credentials (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value_enc   BLOB NOT NULL,
    value_hint  TEXT NOT NULL DEFAULT '',
    skill_name  TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name, skill_name)
);

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     TEXT REFERENCES users(id),
    action      TEXT NOT NULL,
    resource_id TEXT,
    metadata    TEXT,
    ip_address  TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
