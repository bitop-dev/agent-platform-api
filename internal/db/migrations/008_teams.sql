-- +goose Up
-- Phase 9: Multi-user — teams, roles, invitations
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
    role        TEXT NOT NULL DEFAULT 'member',   -- 'owner', 'admin', 'member', 'viewer'
    joined_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE team_invitations (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'member',
    invited_by  TEXT NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'accepted', 'declined', 'expired'
    expires_at  TIMESTAMP NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Add team_id to agents and runs for team-scoped resources
ALTER TABLE agents ADD COLUMN team_id TEXT REFERENCES teams(id);
ALTER TABLE runs ADD COLUMN team_id TEXT REFERENCES teams(id);

-- Add OAuth fields to users
ALTER TABLE users ADD COLUMN avatar_url TEXT;
ALTER TABLE users ADD COLUMN oauth_provider TEXT;
ALTER TABLE users ADD COLUMN oauth_id TEXT;

CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE INDEX idx_team_invitations_email ON team_invitations(email, status);
CREATE INDEX idx_agents_team ON agents(team_id) WHERE team_id IS NOT NULL;
CREATE INDEX idx_runs_team ON runs(team_id) WHERE team_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_runs_team;
DROP INDEX IF EXISTS idx_agents_team;
DROP INDEX IF EXISTS idx_team_invitations_email;
DROP INDEX IF EXISTS idx_team_members_user;
DROP TABLE IF EXISTS team_invitations;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
