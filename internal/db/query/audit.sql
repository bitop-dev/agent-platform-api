-- name: InsertAuditLog :exec
INSERT INTO audit_log (user_id, action, resource_id, metadata, ip_address)
VALUES (?, ?, ?, ?, ?);

-- name: ListAuditLog :many
SELECT * FROM audit_log
WHERE user_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountAuditLog :one
SELECT COUNT(*) FROM audit_log WHERE user_id = ?;
