// Package handlers provides HTTP handler types and response DTOs.
// DTOs flatten sql.Null* types into clean JSON for the frontend.
package handlers

import (
	"time"

	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// --- Agent ---

type AgentDTO struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	SystemPrompt   string    `json:"system_prompt"`
	ModelProvider  string    `json:"model_provider"`
	ModelName      string    `json:"model_name"`
	ConfigYAML     string    `json:"config_yaml,omitempty"`
	MaxTurns       int       `json:"max_turns"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func agentToDTO(a sqlc.Agent) AgentDTO {
	return AgentDTO{
		ID:             a.ID,
		UserID:         a.UserID,
		Name:           a.Name,
		Description:    a.Description.String,
		SystemPrompt:   a.SystemPrompt,
		ModelProvider:  a.ModelProvider,
		ModelName:      a.ModelName,
		ConfigYAML:     a.ConfigYaml,
		MaxTurns:       int(a.MaxTurns),
		TimeoutSeconds: int(a.TimeoutSeconds),
		Enabled:        a.Enabled,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

func agentsToDTOs(agents []sqlc.Agent) []AgentDTO {
	out := make([]AgentDTO, len(agents))
	for i, a := range agents {
		out[i] = agentToDTO(a)
	}
	return out
}

// --- Run ---

type RunDTO struct {
	ID            string     `json:"id"`
	AgentID       string     `json:"agent_id"`
	Mission       string     `json:"mission"`
	ModelProvider string     `json:"model_provider"`
	ModelName     string     `json:"model_name"`
	Status        string     `json:"status"`
	OutputText    string     `json:"output_text,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	TotalTurns    int        `json:"total_turns,omitempty"`
	InputTokens   int        `json:"input_tokens,omitempty"`
	OutputTokens  int        `json:"output_tokens,omitempty"`
	CostUSD       float64    `json:"cost_usd,omitempty"`
	DurationMs    int        `json:"duration_ms,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

func runToDTO(r sqlc.Run) RunDTO {
	dto := RunDTO{
		ID:            r.ID,
		AgentID:       r.AgentID,
		Mission:       r.Mission,
		ModelProvider: r.ModelProvider,
		ModelName:     r.ModelName,
		Status:        r.Status,
		OutputText:    r.OutputText.String,
		ErrorMessage:  r.ErrorMessage.String,
		TotalTurns:    int(r.TotalTurns.Int64),
		InputTokens:   int(r.InputTokens.Int64),
		OutputTokens:  int(r.OutputTokens.Int64),
		CostUSD:       r.CostUsd.Float64,
		DurationMs:    int(r.DurationMs.Int64),
		CreatedAt:     r.CreatedAt,
	}
	if r.StartedAt.Valid {
		dto.StartedAt = &r.StartedAt.Time
	}
	if r.CompletedAt.Valid {
		dto.CompletedAt = &r.CompletedAt.Time
	}
	return dto
}

func runsToDTOs(runs []sqlc.Run) []RunDTO {
	out := make([]RunDTO, len(runs))
	for i, r := range runs {
		out[i] = runToDTO(r)
	}
	return out
}

// --- Run with agent name (for dashboard) ---

type RecentRunDTO struct {
	RunDTO
	AgentName string `json:"agent_name"`
}

func recentRunToDTO(r sqlc.RecentRunsRow) RecentRunDTO {
	return RecentRunDTO{
		RunDTO: runToDTO(sqlc.Run{
			ID: r.ID, AgentID: r.AgentID, Mission: r.Mission,
			ModelProvider: r.ModelProvider, ModelName: r.ModelName,
			Status: r.Status, OutputText: r.OutputText, ErrorMessage: r.ErrorMessage,
			TotalTurns: r.TotalTurns, InputTokens: r.InputTokens,
			OutputTokens: r.OutputTokens, CostUsd: r.CostUsd,
			DurationMs: r.DurationMs, CreatedAt: r.CreatedAt,
			StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
		}),
		AgentName: r.AgentName,
	}
}

func recentRunsToDTOs(runs []sqlc.RecentRunsRow) []RecentRunDTO {
	out := make([]RecentRunDTO, len(runs))
	for i, r := range runs {
		out[i] = recentRunToDTO(r)
	}
	return out
}

// --- Skill ---

type SkillDTO struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tier        string    `json:"tier"`
	Version     string    `json:"version"`
	SkillMD     string    `json:"skill_md,omitempty"`
	Tags        string    `json:"tags,omitempty"`
	SourceURL   string    `json:"source_url,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func skillToDTO(s sqlc.Skill) SkillDTO {
	return SkillDTO{
		ID:          s.ID,
		UserID:      s.UserID.String,
		Name:        s.Name,
		Description: s.Description,
		Tier:        s.Tier,
		Version:     s.Version,
		SkillMD:     s.SkillMd,
		Tags:        s.Tags,
		SourceURL:   s.SourceUrl.String,
		Enabled:     s.Enabled,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

func skillsToDTOs(skills []sqlc.Skill) []SkillDTO {
	out := make([]SkillDTO, len(skills))
	for i, s := range skills {
		out[i] = skillToDTO(s)
	}
	return out
}

// --- Run Event ---

type RunEventDTO struct {
	ID         int64     `json:"id"`
	RunID      string    `json:"run_id"`
	Seq        int       `json:"seq"`
	EventType  string    `json:"event_type"`
	Data       string    `json:"data,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func eventToDTO(e sqlc.RunEvent) RunEventDTO {
	return RunEventDTO{
		ID:         e.ID,
		RunID:      e.RunID,
		Seq:        int(e.Seq),
		EventType:  e.EventType,
		Data:       e.DataJson.String,
		OccurredAt: e.OccurredAt,
	}
}

func eventsToDTOs(events []sqlc.RunEvent) []RunEventDTO {
	out := make([]RunEventDTO, len(events))
	for i, e := range events {
		out[i] = eventToDTO(e)
	}
	return out
}
