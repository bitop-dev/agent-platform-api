-- name: CreateRun :one
INSERT INTO runs (id, agent_id, mission, model_provider, model_name, status)
VALUES (?, ?, ?, ?, ?, 'queued')
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = ?;

-- name: ListRunsByAgent :many
SELECT * FROM runs WHERE agent_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: UpdateRunStatus :exec
UPDATE runs SET status = ?, started_at = ?, completed_at = ? WHERE id = ?;

-- name: UpdateRunResult :exec
UPDATE runs
SET status = ?, output_text = ?, error_message = ?,
    total_turns = ?, input_tokens = ?, output_tokens = ?,
    cost_usd = ?, duration_ms = ?, completed_at = ?
WHERE id = ?;

-- name: CountRunsByAgent :one
SELECT COUNT(*) FROM runs WHERE agent_id = ?;

-- name: InsertRunEvent :exec
INSERT INTO run_events (run_id, seq, event_type, data_json)
VALUES (?, ?, ?, ?);

-- name: ListRunEvents :many
SELECT * FROM run_events WHERE run_id = ? ORDER BY seq;
