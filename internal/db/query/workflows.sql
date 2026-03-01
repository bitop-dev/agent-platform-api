-- name: CreateWorkflow :one
INSERT INTO workflows (id, user_id, team_id, name, description)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkflow :one
SELECT * FROM workflows WHERE id = ?;

-- name: ListWorkflowsByUser :many
SELECT * FROM workflows WHERE user_id = ?
ORDER BY created_at DESC;

-- name: UpdateWorkflow :exec
UPDATE workflows SET name = ?, description = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: DeleteWorkflow :exec
DELETE FROM workflows WHERE id = ? AND user_id = ?;

-- name: CreateWorkflowStep :one
INSERT INTO workflow_steps (id, workflow_id, agent_id, name, position, mission_template, depends_on)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkflowSteps :many
SELECT ws.*, a.name as agent_name FROM workflow_steps ws
JOIN agents a ON ws.agent_id = a.id
WHERE ws.workflow_id = ?
ORDER BY ws.position;

-- name: GetWorkflowStep :one
SELECT * FROM workflow_steps WHERE id = ?;

-- name: UpdateWorkflowStep :exec
UPDATE workflow_steps SET agent_id = ?, name = ?, position = ?, mission_template = ?, depends_on = ?
WHERE id = ?;

-- name: DeleteWorkflowStep :exec
DELETE FROM workflow_steps WHERE id = ?;

-- name: DeleteWorkflowSteps :exec
DELETE FROM workflow_steps WHERE workflow_id = ?;

-- name: CreateWorkflowRun :one
INSERT INTO workflow_runs (id, workflow_id, user_id, status, input_text)
VALUES (?, ?, ?, 'pending', ?)
RETURNING *;

-- name: GetWorkflowRun :one
SELECT * FROM workflow_runs WHERE id = ?;

-- name: ListWorkflowRuns :many
SELECT * FROM workflow_runs WHERE workflow_id = ?
ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: UpdateWorkflowRunStatus :exec
UPDATE workflow_runs SET status = ?, started_at = ?, completed_at = ?, output_text = ?, error_message = ?
WHERE id = ?;

-- name: CreateStepRun :one
INSERT INTO workflow_step_runs (id, workflow_run_id, step_id, status)
VALUES (?, ?, ?, 'pending')
RETURNING *;

-- name: UpdateStepRun :exec
UPDATE workflow_step_runs SET run_id = ?, status = ?, started_at = ?, completed_at = ?
WHERE id = ?;

-- name: ListStepRuns :many
SELECT sr.*, ws.name as step_name, ws.agent_id, ws.depends_on
FROM workflow_step_runs sr
JOIN workflow_steps ws ON sr.step_id = ws.id
WHERE sr.workflow_run_id = ?
ORDER BY ws.position;

-- name: GetStepRunByStepID :one
SELECT * FROM workflow_step_runs
WHERE workflow_run_id = ? AND step_id = ?;

-- name: ListPendingStepRuns :many
SELECT sr.*, ws.name as step_name, ws.agent_id, ws.mission_template, ws.depends_on
FROM workflow_step_runs sr
JOIN workflow_steps ws ON sr.step_id = ws.id
WHERE sr.workflow_run_id = ? AND sr.status = 'pending'
ORDER BY ws.position;
