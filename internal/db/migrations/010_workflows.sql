-- +goose Up

-- AI Team workflows — multi-agent pipelines.
-- A workflow defines a sequence of agent steps with dependencies.
CREATE TABLE workflows (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id     TEXT REFERENCES teams(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Each step references an agent and defines its mission template.
-- depends_on is a JSON array of step IDs that must complete before this step runs.
-- mission_template supports {{input}} (workflow input) and {{steps.NAME.output}} (sibling step output).
CREATE TABLE workflow_steps (
    id                TEXT PRIMARY KEY,
    workflow_id       TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    agent_id          TEXT NOT NULL REFERENCES agents(id),
    name              TEXT NOT NULL,
    position          INTEGER NOT NULL DEFAULT 0,
    mission_template  TEXT NOT NULL,
    depends_on        TEXT NOT NULL DEFAULT '[]',
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Tracks each execution of a workflow.
CREATE TABLE workflow_runs (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflows(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'pending',
    input_text      TEXT NOT NULL DEFAULT '',
    output_text     TEXT,
    error_message   TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP
);

-- Maps workflow steps to actual agent runs within a workflow execution.
CREATE TABLE workflow_step_runs (
    id              TEXT PRIMARY KEY,
    workflow_run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_id         TEXT NOT NULL REFERENCES workflow_steps(id),
    run_id          TEXT REFERENCES runs(id),
    status          TEXT NOT NULL DEFAULT 'pending',
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS workflow_step_runs;
DROP TABLE IF EXISTS workflow_runs;
DROP TABLE IF EXISTS workflow_steps;
DROP TABLE IF EXISTS workflows;
