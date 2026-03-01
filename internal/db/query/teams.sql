-- name: CreateTeam :one
INSERT INTO teams (id, name, slug, owner_id)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams WHERE id = ?;

-- name: GetTeamBySlug :one
SELECT * FROM teams WHERE slug = ?;

-- name: ListUserTeams :many
SELECT t.* FROM teams t
JOIN team_members tm ON tm.team_id = t.id
WHERE tm.user_id = ?
ORDER BY t.name;

-- name: UpdateTeam :exec
UPDATE teams SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteTeam :exec
DELETE FROM teams WHERE id = ?;

-- name: AddTeamMember :exec
INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?);

-- name: RemoveTeamMember :exec
DELETE FROM team_members WHERE team_id = ? AND user_id = ?;

-- name: UpdateTeamMemberRole :exec
UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?;

-- name: ListTeamMembers :many
SELECT tm.*, u.email, u.name as user_name, u.avatar_url
FROM team_members tm
JOIN users u ON u.id = tm.user_id
WHERE tm.team_id = ?
ORDER BY tm.joined_at;

-- name: GetTeamMember :one
SELECT * FROM team_members WHERE team_id = ? AND user_id = ?;

-- name: CreateInvitation :one
INSERT INTO team_invitations (id, team_id, email, role, invited_by, expires_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInvitation :one
SELECT * FROM team_invitations WHERE id = ?;

-- name: ListPendingInvitations :many
SELECT * FROM team_invitations WHERE team_id = ? AND status = 'pending'
ORDER BY created_at DESC;

-- name: UpdateInvitationStatus :exec
UPDATE team_invitations SET status = ? WHERE id = ?;

-- name: ListChildRuns :many
SELECT * FROM runs WHERE parent_run_id = ? ORDER BY created_at;

-- name: CountChildRuns :one
SELECT COUNT(*) FROM runs WHERE parent_run_id = ?;
