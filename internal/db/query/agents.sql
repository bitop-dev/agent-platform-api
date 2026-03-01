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

-- name: ListAgentsByTeam :many
SELECT * FROM agents WHERE team_id = ? ORDER BY created_at DESC;

-- name: SetAgentTeam :exec
UPDATE agents SET team_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListAgentsByUserOrTeam :many
SELECT DISTINCT a.* FROM agents a
LEFT JOIN team_members tm ON a.team_id = tm.team_id AND tm.user_id = ?
WHERE a.user_id = ? OR tm.user_id IS NOT NULL
ORDER BY a.created_at DESC;
