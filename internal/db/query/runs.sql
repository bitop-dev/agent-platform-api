-- name: CreateRun :one
INSERT INTO runs (id, agent_id, mission, model_provider, model_name, status)
VALUES (?, ?, ?, ?, ?, 'queued')
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = ?;

-- name: ListRunsByUser :many
SELECT r.* FROM runs r
JOIN agents a ON r.agent_id = a.id
WHERE a.user_id = ?
ORDER BY r.created_at DESC LIMIT ? OFFSET ?;

-- name: CountRunsByUser :one
SELECT COUNT(*) FROM runs r
JOIN agents a ON r.agent_id = a.id
WHERE a.user_id = ?;

-- name: ListRunsByUserFiltered :many
SELECT r.* FROM runs r
JOIN agents a ON r.agent_id = a.id
WHERE a.user_id = ?
  AND (CAST(? AS TEXT) = '' OR r.status = CAST(? AS TEXT))
  AND (CAST(? AS TEXT) = '' OR r.agent_id = CAST(? AS TEXT))
ORDER BY r.created_at DESC LIMIT ? OFFSET ?;

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

-- name: ListRunsByUserOrTeam :many
SELECT r.* FROM runs r
JOIN agents a ON r.agent_id = a.id
LEFT JOIN team_members tm ON a.team_id = tm.team_id AND tm.user_id = ?
WHERE a.user_id = ? OR tm.user_id IS NOT NULL
ORDER BY r.created_at DESC LIMIT ? OFFSET ?;

-- name: CountRunsByUserOrTeam :one
SELECT COUNT(*) FROM runs r
JOIN agents a ON r.agent_id = a.id
LEFT JOIN team_members tm ON a.team_id = tm.team_id AND tm.user_id = ?
WHERE a.user_id = ? OR tm.user_id IS NOT NULL;


