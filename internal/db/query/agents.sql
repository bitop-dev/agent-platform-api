-- name: CreateAgent :one
INSERT INTO agents (id, user_id, name, description, system_prompt, model_provider, model_name, config_yaml, max_turns, timeout_seconds)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAgent :one
SELECT * FROM agents WHERE id = ?;

-- name: ListAgentsByUser :many
SELECT * FROM agents WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateAgent :exec
UPDATE agents
SET name = ?, description = ?, system_prompt = ?, model_provider = ?, model_name = ?,
    config_yaml = ?, max_turns = ?, timeout_seconds = ?, enabled = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = ?;

-- name: CountAgentsByUser :one
SELECT COUNT(*) FROM agents WHERE user_id = ?;
