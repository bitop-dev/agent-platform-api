-- name: CreateSchedule :one
INSERT INTO schedules (id, user_id, agent_id, name, description, schedule_type, cron_expr, interval_seconds, timezone, mission, enabled, overlap_policy, max_retries, next_run_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSchedule :one
SELECT * FROM schedules WHERE id = ? AND user_id = ?;

-- name: ListSchedules :many
SELECT * FROM schedules WHERE user_id = ? ORDER BY created_at DESC;

-- name: ListSchedulesByAgent :many
SELECT * FROM schedules WHERE agent_id = ? AND user_id = ? ORDER BY created_at DESC;

-- name: UpdateSchedule :one
UPDATE schedules SET
    name = ?,
    description = ?,
    schedule_type = ?,
    cron_expr = ?,
    interval_seconds = ?,
    timezone = ?,
    mission = ?,
    enabled = ?,
    overlap_policy = ?,
    max_retries = ?,
    next_run_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?
RETURNING *;

-- name: DeleteSchedule :exec
DELETE FROM schedules WHERE id = ? AND user_id = ?;

-- name: EnableSchedule :exec
UPDATE schedules SET enabled = true, consecutive_errors = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?;

-- name: DisableSchedule :exec
UPDATE schedules SET enabled = false, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?;

-- name: ListDueSchedules :many
SELECT s.*, a.model_provider, a.model_name, a.system_prompt, a.max_turns, a.timeout_seconds
FROM schedules s
JOIN agents a ON s.agent_id = a.id
WHERE s.enabled = true AND s.next_run_at IS NOT NULL AND s.next_run_at <= ?
ORDER BY s.next_run_at ASC;

-- name: UpdateScheduleAfterRun :exec
UPDATE schedules SET
    last_run_at = ?,
    last_run_status = ?,
    last_run_id = ?,
    last_error = ?,
    consecutive_errors = ?,
    next_run_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
