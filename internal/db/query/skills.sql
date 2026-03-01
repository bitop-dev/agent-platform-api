-- name: CreateSkill :one
INSERT INTO skills (id, user_id, name, description, tier, version, skill_md, tags, source_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertRegistrySkill :exec
INSERT INTO skills (id, user_id, source_id, name, description, tier, version, skill_md, tags, source_url, enabled)
VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, true)
ON CONFLICT(id) DO UPDATE SET
  source_id = excluded.source_id,
  description = excluded.description,
  tier = excluded.tier,
  version = excluded.version,
  skill_md = excluded.skill_md,
  tags = excluded.tags,
  source_url = excluded.source_url,
  updated_at = CURRENT_TIMESTAMP;

-- name: DeleteSkillsBySource :exec
DELETE FROM skills WHERE source_id = ? AND id NOT IN (SELECT skill_id FROM agent_skills);

-- name: GetSkill :one
SELECT * FROM skills WHERE id = ?;

-- name: ListSkills :many
SELECT * FROM skills WHERE enabled = true ORDER BY name;

-- name: ListSkillsByTier :many
SELECT * FROM skills WHERE tier = ? AND enabled = true ORDER BY name;

-- name: ListSkillsByUser :many
SELECT * FROM skills WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateSkill :exec
UPDATE skills
SET name = ?, description = ?, skill_md = ?, tags = ?, enabled = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteSkill :exec
DELETE FROM skills WHERE id = ?;

-- name: AddAgentSkill :exec
INSERT INTO agent_skills (agent_id, skill_id, position, config_json)
VALUES (?, ?, ?, ?)
ON CONFLICT(agent_id, skill_id) DO UPDATE SET position = excluded.position, config_json = excluded.config_json;

-- name: RemoveAgentSkill :exec
DELETE FROM agent_skills WHERE agent_id = ? AND skill_id = ?;

-- name: ListAgentSkills :many
SELECT s.*, ags.position, ags.config_json
FROM skills s
JOIN agent_skills ags ON ags.skill_id = s.id
WHERE ags.agent_id = ?
ORDER BY ags.position;
