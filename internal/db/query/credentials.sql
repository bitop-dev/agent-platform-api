-- name: CreateCredential :one
INSERT INTO user_credentials (id, user_id, name, value_enc, value_hint, skill_name, description)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCredentialsByUser :many
SELECT id, user_id, name, value_hint, skill_name, description, created_at, updated_at
FROM user_credentials WHERE user_id = ? ORDER BY name, skill_name;

-- name: GetCredential :one
SELECT * FROM user_credentials WHERE id = ? AND user_id = ?;

-- name: GetCredentialByName :one
SELECT * FROM user_credentials WHERE user_id = ? AND name = ? AND skill_name = ?;

-- name: DeleteCredential :exec
DELETE FROM user_credentials WHERE id = ? AND user_id = ?;

-- name: ListCredentialsForSkill :many
SELECT * FROM user_credentials
WHERE user_id = ? AND (skill_name = ? OR skill_name = '')
ORDER BY skill_name DESC;

-- name: ListAllCredentialValues :many
SELECT name, value_enc, skill_name FROM user_credentials
WHERE user_id = ?
ORDER BY skill_name DESC, name;

-- name: UpdateCredential :exec
UPDATE user_credentials
SET value_enc = ?, value_hint = ?, description = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;
