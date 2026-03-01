package handlers

import (
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/api/middleware"
	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// TeamHandler handles team CRUD, members, and invitations.
type TeamHandler struct {
	store *db.Store
	audit *audit.Logger
}

func NewTeamHandler(store *db.Store) *TeamHandler {
	return &TeamHandler{store: store, audit: audit.NewLogger(store.Queries)}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type TeamDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	OwnerID   string `json:"owner_id"`
	CreatedAt string `json:"created_at"`
}

type TeamMemberDTO struct {
	TeamID    string `json:"team_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	JoinedAt  string `json:"joined_at"`
}

type InvitationDTO struct {
	ID        string `json:"id"`
	TeamID    string `json:"team_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

func teamToDTO(t sqlc.Team) TeamDTO {
	return TeamDTO{
		ID:        t.ID,
		Name:      t.Name,
		Slug:      t.Slug,
		OwnerID:   t.OwnerID,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
	}
}

// ─── Handlers ────────────────────────────────────────────────────────────────

type createTeamRequest struct {
	Name string `json:"name"`
}

var slugRegexp = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(slugRegexp.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func (h *TeamHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	var req createTeamRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	id := uuid.NewString()
	slug := slugify(req.Name) + "-" + id[:4]

	team, err := h.store.CreateTeam(c.Context(), sqlc.CreateTeamParams{
		ID: id, Name: req.Name, Slug: slug, OwnerID: userID,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create team"})
	}

	// Add creator as owner
	_ = h.store.AddTeamMember(c.Context(), sqlc.AddTeamMemberParams{
		TeamID: id, UserID: userID, Role: "owner",
	})

	h.audit.Log(c.Context(), userID, audit.ActionTeamCreate, id, c.IP(), map[string]any{
		"name": team.Name,
	})

	return c.Status(fiber.StatusCreated).JSON(teamToDTO(team))
}

func (h *TeamHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	teams, err := h.store.ListUserTeams(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list teams"})
	}
	dtos := make([]TeamDTO, len(teams))
	for i, t := range teams {
		dtos[i] = teamToDTO(t)
	}
	return c.JSON(fiber.Map{"teams": dtos})
}

func (h *TeamHandler) Get(c *fiber.Ctx) error {
	team, err := h.store.GetTeam(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "team not found"})
	}
	return c.JSON(teamToDTO(team))
}

func (h *TeamHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	team, err := h.store.GetTeam(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "team not found"})
	}
	if team.OwnerID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only owner can delete team"})
	}
	if err := h.store.DeleteTeam(c.Context(), team.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *TeamHandler) ListMembers(c *fiber.Ctx) error {
	members, err := h.store.ListTeamMembers(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list members"})
	}
	dtos := make([]TeamMemberDTO, len(members))
	for i, m := range members {
		avatar := m.AvatarUrl.String
		dtos[i] = TeamMemberDTO{
			TeamID:    m.TeamID,
			UserID:    m.UserID,
			Role:      m.Role,
			Email:     m.Email,
			Name:      m.UserName,
			AvatarURL: avatar,
			JoinedAt:  m.JoinedAt.Format(time.RFC3339),
		}
	}
	return c.JSON(fiber.Map{"members": dtos})
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *TeamHandler) Invite(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	teamID := c.Params("id")

	var req inviteRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	if req.Role == "" {
		req.Role = "member"
	}

	// Only owner/admin can invite
	member, err := h.store.GetTeamMember(c.Context(), sqlc.GetTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil || (member.Role != "owner" && member.Role != "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
	}

	inv, err := h.store.CreateInvitation(c.Context(), sqlc.CreateInvitationParams{
		ID:        uuid.NewString(),
		TeamID:    teamID,
		Email:     req.Email,
		Role:      req.Role,
		InvitedBy: userID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create invitation"})
	}

	h.audit.Log(c.Context(), userID, audit.ActionTeamInvite, teamID, c.IP(), map[string]any{
		"email": req.Email,
		"role":  req.Role,
	})

	return c.Status(fiber.StatusCreated).JSON(InvitationDTO{
		ID:        inv.ID,
		TeamID:    inv.TeamID,
		Email:     inv.Email,
		Role:      inv.Role,
		Status:    inv.Status,
		ExpiresAt: inv.ExpiresAt.Format(time.RFC3339),
		CreatedAt: inv.CreatedAt.Format(time.RFC3339),
	})
}

func (h *TeamHandler) AcceptInvitation(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	invID := c.Params("invitation_id")

	inv, err := h.store.GetInvitation(c.Context(), invID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invitation not found"})
	}
	if inv.Status != "pending" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invitation already " + inv.Status})
	}
	if time.Now().After(inv.ExpiresAt) {
		_ = h.store.UpdateInvitationStatus(c.Context(), sqlc.UpdateInvitationStatusParams{
			Status: "expired", ID: invID,
		})
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "invitation expired"})
	}

	// Add user to team
	_ = h.store.AddTeamMember(c.Context(), sqlc.AddTeamMemberParams{
		TeamID: inv.TeamID, UserID: userID, Role: inv.Role,
	})
	_ = h.store.UpdateInvitationStatus(c.Context(), sqlc.UpdateInvitationStatusParams{
		Status: "accepted", ID: invID,
	})

	return c.JSON(fiber.Map{"status": "accepted"})
}

func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	teamID := c.Params("id")
	memberID := c.Params("user_id")

	member, err := h.store.GetTeamMember(c.Context(), sqlc.GetTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil || (member.Role != "owner" && member.Role != "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
	}

	if err := h.store.RemoveTeamMember(c.Context(), sqlc.RemoveTeamMemberParams{
		TeamID: teamID, UserID: memberID,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to remove"})
	}
	h.audit.Log(c.Context(), userID, audit.ActionTeamRemove, teamID, c.IP(), map[string]any{
		"removed_user": memberID,
	})
	return c.JSON(fiber.Map{"status": "removed"})
}
