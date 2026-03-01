package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/bitop-dev/agent-platform-api/internal/audit"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/config"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// OAuthHandler handles GitHub and Google OAuth login flows.
type OAuthHandler struct {
	store *db.Store
	auth  *auth.Auth
	cfg   *config.Config
	audit *audit.Logger
}

func NewOAuthHandler(store *db.Store, a *auth.Auth, cfg *config.Config) *OAuthHandler {
	return &OAuthHandler{store: store, auth: a, cfg: cfg, audit: audit.NewLogger(store.Queries)}
}

// ── GitHub OAuth ─────────────────────────────────────────────────────────────

// GitHubLogin redirects to GitHub's OAuth authorization page.
func (h *OAuthHandler) GitHubLogin(c *fiber.Ctx) error {
	if h.cfg.GitHubClientID == "" {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "GitHub OAuth not configured"})
	}

	redirectURI := fmt.Sprintf("%s/api/auth/github/callback", h.cfg.BaseURL)
	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=user:email",
		url.QueryEscape(h.cfg.GitHubClientID),
		url.QueryEscape(redirectURI),
	)
	return c.Redirect(authURL)
}

// GitHubCallback handles the OAuth callback from GitHub.
func (h *OAuthHandler) GitHubCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code parameter"})
	}

	// Exchange code for access token
	token, err := h.exchangeGitHubCode(code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": fmt.Sprintf("github token exchange: %v", err)})
	}

	// Fetch user info from GitHub
	ghUser, err := h.fetchGitHubUser(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": fmt.Sprintf("github user fetch: %v", err)})
	}

	// Upsert user
	user, err := h.store.UpsertOAuthUser(c.Context(), sqlc.UpsertOAuthUserParams{
		ID:            uuid.NewString(),
		Email:         ghUser.Email,
		Name:          ghUser.Name,
		AvatarUrl:     sql.NullString{String: ghUser.AvatarURL, Valid: ghUser.AvatarURL != ""},
		OauthProvider: sql.NullString{String: "github", Valid: true},
		OauthID:       sql.NullString{String: fmt.Sprintf("%d", ghUser.ID), Valid: true},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create user"})
	}

	h.audit.Log(c.Context(), user.ID, audit.ActionOAuthLogin, user.ID, c.IP(), map[string]any{
		"provider": "github",
		"email":    user.Email,
	})

	// Generate JWT
	access, refresh, err := h.auth.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "token generation failed"})
	}

	// Redirect to frontend with token in URL fragment
	frontendURL := fmt.Sprintf("%s/login?token=%s&refresh_token=%s",
		strings.TrimSuffix(h.cfg.BaseURL, "/api"),
		url.QueryEscape(access),
		url.QueryEscape(refresh),
	)
	return c.Redirect(frontendURL)
}

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func (h *OAuthHandler) exchangeGitHubCode(code string) (string, error) {
	data := url.Values{
		"client_id":     {h.cfg.GitHubClientID},
		"client_secret": {h.cfg.GitHubClientSecret},
		"code":          {code},
	}

	req, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func (h *OAuthHandler) fetchGitHubUser(token string) (*githubUser, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user request: %w", err)
	}
	defer resp.Body.Close()

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	// If email is private, fetch from /user/emails
	if user.Email == "" {
		user.Email, _ = h.fetchGitHubPrimaryEmail(token)
	}

	// Use login as name fallback
	if user.Name == "" {
		user.Name = user.Login
	}

	if user.Email == "" {
		return nil, fmt.Errorf("no email found on GitHub account")
	}

	return &user, nil
}

func (h *OAuthHandler) fetchGitHubPrimaryEmail(token string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no verified primary email")
}

// ── Google OAuth ─────────────────────────────────────────────────────────────

// GoogleLogin redirects to Google's OAuth authorization page.
func (h *OAuthHandler) GoogleLogin(c *fiber.Ctx) error {
	if h.cfg.GoogleClientID == "" {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "Google OAuth not configured"})
	}

	redirectURI := fmt.Sprintf("%s/api/auth/google/callback", h.cfg.BaseURL)
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid+email+profile",
		url.QueryEscape(h.cfg.GoogleClientID),
		url.QueryEscape(redirectURI),
	)
	return c.Redirect(authURL)
}

// GoogleCallback handles the OAuth callback from Google.
func (h *OAuthHandler) GoogleCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code parameter"})
	}

	// Exchange code for tokens
	token, err := h.exchangeGoogleCode(code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": fmt.Sprintf("google token exchange: %v", err)})
	}

	// Fetch user info
	gUser, err := h.fetchGoogleUser(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": fmt.Sprintf("google user fetch: %v", err)})
	}

	// Upsert user
	user, err := h.store.UpsertOAuthUser(c.Context(), sqlc.UpsertOAuthUserParams{
		ID:            uuid.NewString(),
		Email:         gUser.Email,
		Name:          gUser.Name,
		AvatarUrl:     sql.NullString{String: gUser.Picture, Valid: gUser.Picture != ""},
		OauthProvider: sql.NullString{String: "google", Valid: true},
		OauthID:       sql.NullString{String: gUser.Sub, Valid: true},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create user"})
	}

	h.audit.Log(c.Context(), user.ID, audit.ActionOAuthLogin, user.ID, c.IP(), map[string]any{
		"provider": "google",
		"email":    user.Email,
	})

	access, refresh, err := h.auth.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "token generation failed"})
	}

	frontendURL := fmt.Sprintf("%s/login?token=%s&refresh_token=%s",
		strings.TrimSuffix(h.cfg.BaseURL, "/api"),
		url.QueryEscape(access),
		url.QueryEscape(refresh),
	)
	return c.Redirect(frontendURL)
}

type googleUser struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (h *OAuthHandler) exchangeGoogleCode(code string) (string, error) {
	redirectURI := fmt.Sprintf("%s/api/auth/google/callback", h.cfg.BaseURL)
	data := url.Values{
		"client_id":     {h.cfg.GoogleClientID},
		"client_secret": {h.cfg.GoogleClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("google error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func (h *OAuthHandler) fetchGoogleUser(token string) (*googleUser, error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user request: %w", err)
	}
	defer resp.Body.Close()

	var user googleUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	if user.Email == "" {
		return nil, fmt.Errorf("no email in Google profile")
	}
	return &user, nil
}
