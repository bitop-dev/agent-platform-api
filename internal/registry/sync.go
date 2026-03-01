// Package registry syncs skills from git-native skill registries.
// Each registry is a GitHub repo with a registry.json index and skills/ directory.
package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	sqlc "github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// RegistryJSON is the top-level structure of registry.json.
type RegistryJSON struct {
	Version   string          `json:"version"`
	UpdatedAt string          `json:"updated_at"`
	Skills    []RegistrySkill `json:"skills"`
}

// RegistrySkill is one entry in registry.json.
type RegistrySkill struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Path         string   `json:"path"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Tags         []string `json:"tags"`
	Tier         string   `json:"tier"`
	HasTools     bool     `json:"has_tools"`
	RequiresBins []string `json:"requires_bins"`
	RequiresEnv  []string `json:"requires_env"`
}

// DefaultRegistryURL is the official skills registry.
const DefaultRegistryURL = "github.com/bitop-dev/agent-platform-skills"

// Syncer syncs skills from remote registries into the database.
type Syncer struct {
	store  *sqlc.Queries
	client *http.Client
}

// NewSyncer creates a registry syncer.
func NewSyncer(store *sqlc.Queries) *Syncer {
	return &Syncer{
		store:  store,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// EnsureDefaultSource creates the default skill source if it doesn't exist.
func (s *Syncer) EnsureDefaultSource(ctx context.Context) error {
	_, err := s.store.GetDefaultSkillSource(ctx)
	if err == nil {
		return nil // already exists
	}

	_, err = s.store.CreateSkillSource(ctx, sqlc.CreateSkillSourceParams{
		ID:        uuid.NewString(),
		Url:       DefaultRegistryURL,
		Label:     "Official Registry",
		IsDefault: true,
		// user_id is NULL for system-level sources
	})
	return err
}

// SyncAll syncs skills from all registered sources.
func (s *Syncer) SyncAll(ctx context.Context) error {
	if err := s.EnsureDefaultSource(ctx); err != nil {
		slog.Error("failed to ensure default source", "error", err)
	}

	sources, err := s.store.ListSkillSources(ctx)
	if err != nil {
		return fmt.Errorf("list skill sources: %w", err)
	}

	var total int
	for _, src := range sources {
		n, err := s.SyncSource(ctx, src)
		if err != nil {
			slog.Error("sync source failed", "url", src.Url, "label", src.Label, "error", err)
			_ = s.store.UpdateSkillSourceStatus(ctx, sqlc.UpdateSkillSourceStatusParams{
				Status:     "error",
				ErrorMsg:   sql.NullString{String: err.Error(), Valid: true},
				SkillCount: 0,
				ID:         src.ID,
			})
			continue
		}
		total += n
	}

	slog.Info("skill registry sync complete", "sources", len(sources), "total_skills", total)
	return nil
}

// SyncSource syncs skills from a single source. Returns count of synced skills.
func (s *Syncer) SyncSource(ctx context.Context, src sqlc.SkillSource) (int, error) {
	rawBase := toRawURL(src.Url)
	registryURL := rawBase + "/registry.json"

	slog.Info("syncing skill source", "url", src.Url, "label", src.Label)

	reg, err := s.fetchRegistry(ctx, registryURL)
	if err != nil {
		return 0, fmt.Errorf("fetch registry: %w", err)
	}

	var synced int
	for _, skill := range reg.Skills {
		skillMD := s.fetchSkillMD(ctx, rawBase, skill.Path)
		tags := strings.Join(skill.Tags, ",")
		sourceURL := fmt.Sprintf("https://%s/tree/main/%s", src.Url, skill.Path)

		err := s.store.UpsertRegistrySkill(ctx, sqlc.UpsertRegistrySkillParams{
			ID:          fmt.Sprintf("%s:%s", src.ID, skill.Name),
			SourceID:    sql.NullString{String: src.ID, Valid: true},
			Name:        skill.Name,
			Description: skill.Description,
			Tier:        skill.Tier,
			Version:     skill.Version,
			SkillMd:     skillMD,
			Tags:        tags,
			SourceUrl:   sql.NullString{String: sourceURL, Valid: true},
		})
		if err != nil {
			slog.Error("failed to upsert skill", "name", skill.Name, "source", src.Url, "error", err)
			continue
		}
		synced++
	}

	_ = s.store.UpdateSkillSourceStatus(ctx, sqlc.UpdateSkillSourceStatusParams{
		Status:     "synced",
		ErrorMsg:   sql.NullString{},
		SkillCount: int64(synced),
		ID:         src.ID,
	})

	slog.Info("source synced", "url", src.Url, "skills", synced)
	return synced, nil
}

// toRawURL converts a GitHub repo URL to a raw content base URL.
// Accepts: github.com/owner/repo, https://github.com/owner/repo, https://github.com/owner/repo.git
func toRawURL(repoURL string) string {
	u := strings.TrimSuffix(repoURL, ".git")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")

	// github.com/owner/repo -> raw.githubusercontent.com/owner/repo/main
	if strings.HasPrefix(u, "github.com/") {
		parts := strings.SplitN(u, "/", 4)
		if len(parts) >= 3 {
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main", parts[1], parts[2])
		}
	}

	// Fallback: assume it's already a raw URL base
	return "https://" + u
}

func (s *Syncer) fetchRegistry(ctx context.Context, url string) (*RegistryJSON, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var reg RegistryJSON
	if err := json.Unmarshal(body, &reg); err != nil {
		return nil, fmt.Errorf("parse registry.json: %w", err)
	}
	return &reg, nil
}

func (s *Syncer) fetchSkillMD(ctx context.Context, rawBase, skillPath string) string {
	url := fmt.Sprintf("%s/%s/SKILL.md", rawBase, skillPath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Debug("could not fetch SKILL.md", "url", url, "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}
