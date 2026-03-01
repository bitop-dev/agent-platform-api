-- name: CountRunsByStatus :many
SELECT status, COUNT(*) as count
FROM runs
WHERE agent_id IN (SELECT id FROM agents WHERE user_id = ?)
GROUP BY status;

-- name: RecentRuns :many
SELECT r.*, a.name as agent_name
FROM runs r
JOIN agents a ON a.id = r.agent_id
WHERE a.user_id = ?
ORDER BY r.created_at DESC
LIMIT ?;

-- name: CountSchedules :one
SELECT COUNT(*) as total,
       SUM(CASE WHEN enabled = true THEN 1 ELSE 0 END) as active
FROM schedules WHERE user_id = ?;
