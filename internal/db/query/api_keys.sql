-- name: CreateAPIKey :one
INSERT INTO api_keys (id, user_id, provider, label, key_enc, key_hint, is_default, base_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAPIKeysByUser :many
SELECT id, user_id, provider, label, key_hint, is_default, base_url, created_at
FROM api_keys WHERE user_id = ? ORDER BY created_at DESC;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = ? AND user_id = ?;

-- name: GetDefaultAPIKey :one
SELECT * FROM api_keys WHERE user_id = ? AND provider = ? AND is_default = true;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = ? AND user_id = ?;

-- name: ClearDefaultAPIKey :exec
UPDATE api_keys SET is_default = false WHERE user_id = ? AND provider = ?;
