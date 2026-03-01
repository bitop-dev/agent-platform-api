-- +goose Up
-- Phase 7: Multi-agent orchestration — parent/child run tracking
ALTER TABLE runs ADD COLUMN parent_run_id TEXT REFERENCES runs(id);
ALTER TABLE runs ADD COLUMN depth INTEGER NOT NULL DEFAULT 0;

-- Index for quickly fetching child runs
CREATE INDEX idx_runs_parent_run_id ON runs(parent_run_id) WHERE parent_run_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_runs_parent_run_id;
-- SQLite doesn't support DROP COLUMN; these columns will persist
