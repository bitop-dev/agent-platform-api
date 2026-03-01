-- name: CreateSkillSource :one
INSERT INTO skill_sources (id, user_id, url, label, is_default)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSkillSource :one
SELECT * FROM skill_sources WHERE id = ?;

-- name: ListSkillSources :many
SELECT * FROM skill_sources ORDER BY is_default DESC, label;

-- name: ListSkillSourcesByUser :many
SELECT * FROM skill_sources
WHERE user_id = ? OR user_id IS NULL
ORDER BY is_default DESC, label;

-- name: UpdateSkillSourceStatus :exec
UPDATE skill_sources
SET status = ?, error_msg = ?, skill_count = ?, last_synced = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteSkillSource :exec
DELETE FROM skill_sources WHERE id = ? AND is_default = false;

-- name: GetDefaultSkillSource :one
SELECT * FROM skill_sources WHERE is_default = true LIMIT 1;
