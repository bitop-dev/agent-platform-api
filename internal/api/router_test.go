package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
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

func TestHealthCheck(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, r, hub)

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
	app := NewRouter(store, a, r, hub)

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

func TestAgentCRUD(t *testing.T) {
	store, a, cleanup := setupTest(t)
	defer cleanup()

	hub := ws.NewHub()
	r := runner.New(store, hub, 1)
	app := NewRouter(store, a, r, hub)

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
	app := NewRouter(store, a, r, hub)

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
	app := NewRouter(store, a, r, hub)

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
