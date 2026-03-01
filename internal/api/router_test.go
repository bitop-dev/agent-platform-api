package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/registry"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

func setupTest(t *testing.T) (*db.Store, *auth.Auth, func()) {
	t.Helper()

	// Use temp file for SQLite
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	store, err := db.Open("sqlite://" + tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatal(err)
	}

	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		os.Remove(tmpFile.Name())
		t.Fatal(err)
	}

	a := auth.New("test-secret-32chars-minimum!!!!!", 60)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, a, cleanup
}

func newTestEncryptor(t *testing.T) *auth.Encryptor {
	t.Helper()
	enc, err := auth.NewEncryptor("") // dev mode
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestHealthCheck(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Register
	body := `{"email":"test@example.com","name":"Test User","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: expected 201, got %d: %s", resp.StatusCode, b)
	}

	var regResp map[string]any
	json.NewDecoder(resp.Body).Decode(&regResp)
	if regResp["token"] == nil {
		t.Fatal("register: no token in response")
	}

	// Login
	body = `{"email":"test@example.com","password":"secret123"}`
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var loginResp map[string]any
	json.NewDecoder(resp.Body).Decode(&loginResp)
	if loginResp["token"] == nil {
		t.Fatal("login: no token in response")
	}

	// Login with wrong password
	body = `{"email":"test@example.com","password":"wrong"}`
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("bad login: expected 401, got %d", resp.StatusCode)
	}
}

func TestRefreshToken(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Register — should get both tokens
	body := `{"email":"refresh@test.com","name":"Refresh","password":"pass123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	var regResp map[string]any
	json.NewDecoder(resp.Body).Decode(&regResp)
	refreshToken := regResp["refresh_token"].(string)
	if refreshToken == "" {
		t.Fatal("expected refresh_token in register response")
	}

	// Use refresh token to get new access token
	body = `{"refresh_token":"` + refreshToken + `"}`
	req = httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("refresh: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var refreshResp map[string]any
	json.NewDecoder(resp.Body).Decode(&refreshResp)
	newToken := refreshResp["token"].(string)
	if newToken == "" {
		t.Fatal("expected new access token from refresh")
	}

	// New access token should work on protected routes
	req = httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+newToken)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("new token on /me: expected 200, got %d", resp.StatusCode)
	}

	// Refresh token should NOT work as access token
	req = httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	resp, _ = app.Test(req)
	if resp.StatusCode != 401 {
		t.Fatalf("refresh token on /me: expected 401, got %d", resp.StatusCode)
	}

	// Access token should NOT work as refresh token
	accessToken := regResp["token"].(string)
	body = `{"refresh_token":"` + accessToken + `"}`
	req = httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	if resp.StatusCode != 401 {
		t.Fatalf("access token as refresh: expected 401, got %d", resp.StatusCode)
	}
}

func TestAgentCRUD(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Register user and get token
	token := registerUser(t, app, "crud@test.com", "Test", "pass123")

	// Create agent
	body := `{"name":"Research Bot","system_prompt":"You are helpful.","model_name":"gpt-4o"}`
	req := httptest.NewRequest("POST", "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: expected 201, got %d: %s", resp.StatusCode, b)
	}

	var agent map[string]any
	json.NewDecoder(resp.Body).Decode(&agent)
	agentID := agent["id"].(string)

	if agent["name"] != "Research Bot" {
		t.Fatalf("expected name 'Research Bot', got %v", agent["name"])
	}

	// List agents
	req = httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var listResp map[string]any
	json.NewDecoder(resp.Body).Decode(&listResp)
	agents := listResp["agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// Get agent
	req = httptest.NewRequest("GET", "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}

	// Delete agent
	req = httptest.NewRequest("DELETE", "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("delete: expected 200, got %d", resp.StatusCode)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	var afterDelete map[string]any
	json.NewDecoder(resp.Body).Decode(&afterDelete)
	agentsAfter := afterDelete["agents"].([]any)
	if len(agentsAfter) != 0 {
		t.Fatalf("expected 0 agents after delete, got %d", len(agentsAfter))
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// No token
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// Bad token
	req = httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	resp, _ = app.Test(req)
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for bad token, got %d", resp.StatusCode)
	}
}

func TestAgentIsolation(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Two users
	token1 := registerUser(t, app, "user1@test.com", "User1", "pass1")
	token2 := registerUser(t, app, "user2@test.com", "User2", "pass2")

	// User1 creates agent
	body := `{"name":"User1 Bot","system_prompt":"hello","model_name":"gpt-4o"}`
	req := httptest.NewRequest("POST", "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token1)
	resp, _ := app.Test(req)
	var agent map[string]any
	json.NewDecoder(resp.Body).Decode(&agent)
	agentID := agent["id"].(string)

	// User2 cannot see User1's agent
	req = httptest.NewRequest("GET", "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	resp, _ = app.Test(req)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for other user's agent, got %d", resp.StatusCode)
	}

	// User2 cannot delete User1's agent
	req = httptest.NewRequest("DELETE", "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	resp, _ = app.Test(req)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for delete, got %d", resp.StatusCode)
	}
}

func TestRunCreation(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	// Don't start the runner — we just test the API layer
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	token := registerUser(t, app, "runner@test.com", "Runner", "pass123")

	// Create agent
	body := `{"name":"Test Agent","system_prompt":"Hi","model_name":"gpt-4o"}`
	req := httptest.NewRequest("POST", "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	var agent map[string]any
	json.NewDecoder(resp.Body).Decode(&agent)
	agentID := agent["id"].(string)

	// Create run
	body = `{"agent_id":"` + agentID + `","mission":"Say hello"}`
	req = httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create run: expected 202, got %d: %s", resp.StatusCode, b)
	}

	var run map[string]any
	json.NewDecoder(resp.Body).Decode(&run)
	runID := run["id"].(string)

	if run["status"] != "queued" {
		t.Fatalf("expected status 'queued', got %v", run["status"])
	}
	if run["mission"] != "Say hello" {
		t.Fatalf("expected mission 'Say hello', got %v", run["mission"])
	}

	// Get run
	req = httptest.NewRequest("GET", "/api/v1/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("get run: expected 200, got %d", resp.StatusCode)
	}

	// List runs for agent
	req = httptest.NewRequest("GET", "/api/v1/agents/"+agentID+"/runs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("list runs: expected 200, got %d", resp.StatusCode)
	}

	var listResp map[string]any
	json.NewDecoder(resp.Body).Decode(&listResp)
	runs := listResp["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestAPIKeyCRUD(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	token := registerUser(t, app, "keys@test.com", "Key User", "pass123")

	// Create API key
	body := `{"provider":"openai","label":"My OpenAI Key","key":"sk-test-1234567890","is_default":true}`
	req := httptest.NewRequest("POST", "/api/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key: expected 201, got %d: %s", resp.StatusCode, b)
	}

	var keyResp map[string]any
	json.NewDecoder(resp.Body).Decode(&keyResp)

	if keyResp["key_hint"] != "...7890" {
		t.Fatalf("expected hint ...7890, got %v", keyResp["key_hint"])
	}
	if keyResp["provider"] != "openai" {
		t.Fatalf("expected provider openai, got %v", keyResp["provider"])
	}
	keyID := keyResp["id"].(string)

	// List API keys (should NOT contain the actual key)
	req = httptest.NewRequest("GET", "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)

	var listResp map[string]any
	json.NewDecoder(resp.Body).Decode(&listResp)
	keys := listResp["api_keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/api/v1/api-keys/"+keyID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("delete key: expected 200, got %d", resp.StatusCode)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = app.Test(req)
	json.NewDecoder(resp.Body).Decode(&listResp)
	keys = listResp["api_keys"].([]any)
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestRateLimiting(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Auth rate limit is 10/min — send 11 requests
	for i := range 10 {
		body := `{"email":"rl` + fmt.Sprintf("%d", i) + `@test.com","name":"RL","password":"pass"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		if resp.StatusCode == 429 {
			t.Fatalf("hit rate limit too early at request %d", i+1)
		}
	}

	// 11th should be rate limited
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"rl11@test.com","name":"RL","password":"pass"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 429 {
		t.Fatalf("expected 429 on 11th request, got %d", resp.StatusCode)
	}
}

func TestMeEndpoint(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	token := registerUser(t, app, "me@test.com", "Me User", "pass123")

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var user map[string]any
	json.NewDecoder(resp.Body).Decode(&user)
	if user["email"] != "me@test.com" {
		t.Fatalf("expected me@test.com, got %v", user["email"])
	}
	if user["name"] != "Me User" {
		t.Fatalf("expected 'Me User', got %v", user["name"])
	}
}

func TestModelsEndpoint(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Public — no auth needed
	req := httptest.NewRequest("GET", "/api/v1/models", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	models := result["models"].([]any)
	if len(models) < 5 {
		t.Fatalf("expected at least 5 models, got %d", len(models))
	}

	// Filter by provider
	req = httptest.NewRequest("GET", "/api/v1/models?provider=anthropic", nil)
	resp, _ = app.Test(req)
	json.NewDecoder(resp.Body).Decode(&result)
	models = result["models"].([]any)
	for _, m := range models {
		model := m.(map[string]any)
		if model["provider"] != "anthropic" {
			t.Fatalf("expected anthropic, got %v", model["provider"])
		}
	}
}

func TestDashboard(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	token := registerUser(t, app, "dash@test.com", "Dashboard", "pass123")

	// Create an agent
	body := `{"name":"Dash Agent","system_prompt":"Hi","model_name":"gpt-4o"}`
	req := httptest.NewRequest("POST", "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.Test(req)

	// Dashboard stats
	req = httptest.NewRequest("GET", "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var stats map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)

	agentCount, ok := stats["agents"].(float64)
	if !ok || int(agentCount) != 1 {
		t.Fatalf("expected 1 agent, got %v", stats["agents"])
	}

	totalRuns, ok := stats["total_runs"].(float64)
	if !ok || int(totalRuns) != 0 {
		t.Fatalf("expected 0 runs, got %v", stats["total_runs"])
	}
}

func TestRequestID(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, newTestEncryptor(t), r, hub, registry.NewSyncer(store.Queries), nil)

	// Should get a request ID back
	req := httptest.NewRequest("GET", "/health", nil)
	resp, _ := app.Test(req)
	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Fatal("expected X-Request-ID header")
	}

	// Should echo back provided request ID
	req = httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "custom-id-123")
	resp, _ = app.Test(req)
	if resp.Header.Get("X-Request-ID") != "custom-id-123" {
		t.Fatalf("expected custom-id-123, got %s", resp.Header.Get("X-Request-ID"))
	}
}

// registerUser is a test helper that registers a user and returns the JWT token.
func registerUser(t *testing.T, app *fiber.App, email, name, password string) string {
	t.Helper()
	body := `{"email":"` + email + `","name":"` + name + `","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s: expected 201, got %d: %s", email, resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result["token"].(string)
}
