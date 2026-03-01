-- name: CreateUser :one
INSERT INTO users (id, email, name, password_hash)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ?;

-- name: UpdateUser :exec
UPDATE users SET name = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: GetUserByOAuth :one
SELECT * FROM users WHERE oauth_provider = ? AND oauth_id = ?;

-- name: UpsertOAuthUser :one
INSERT INTO users (id, email, name, avatar_url, oauth_provider, oauth_id)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (email) DO UPDATE SET
    name = excluded.name,
    avatar_url = excluded.avatar_url,
    oauth_provider = excluded.oauth_provider,
    oauth_id = excluded.oauth_id,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;
