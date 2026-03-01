package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/registry"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
	"github.com/gofiber/fiber/v2"
)

// TestE2EAgentRun does a full end-to-end test:
// register → store API key → create agent → trigger run → wait for completion → check result.
// Requires TEST_LLM_API_KEY and TEST_LLM_BASE_URL env vars.
func TestE2EAgentRun(t *testing.T) {
	apiKey := os.Getenv("TEST_LLM_API_KEY")
	baseURL := os.Getenv("TEST_LLM_BASE_URL")
	if apiKey == "" || baseURL == "" {
		t.Skip("Skipping e2e test: set TEST_LLM_API_KEY and TEST_LLM_BASE_URL")
	}

	// Setup
	tmpFile, _ := os.CreateTemp("", "e2e-*.db")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := db.Open("sqlite://" + tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Migrate(context.Background())

	a := auth.New("e2e-test-secret-32chars-minimum!", 60)
	enc, _ := auth.NewEncryptor("") // dev mode
	hub := ws.NewHub()

	r := runner.New(store, hub, 2)
	r.Start()
	defer r.Stop()

	app := NewRouter(store, a, enc, r, hub, registry.NewSyncer(store.Queries))

	// 1. Register user
	token := e2eRegister(t, app, "e2e@test.com", "E2E Tester", "pass123")
	t.Log("✓ registered user")

	// 2. Store API key
	e2eStoreKey(t, app, token, "openai", apiKey)
	t.Log("✓ stored API key")

	// 3. Create agent
	agentID := e2eCreateAgent(t, app, token, "E2E Bot", "You are a helpful assistant. Be very brief.", "gpt-4o", baseURL)
	t.Logf("✓ created agent %s", agentID)

	// 4. Trigger run
	runID := e2eTriggerRun(t, app, token, agentID, "What is 2+2? Reply with just the number.", baseURL)
	t.Logf("✓ triggered run %s", runID)

	// 5. Poll for completion (max 30 seconds)
	var finalRun map[string]any
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		finalRun = e2eGetRun(t, app, token, runID)
		status := finalRun["status"].(string)
		if status == "succeeded" || status == "failed" || status == "cancelled" {
			break
		}
	}

	status := finalRun["status"].(string)
	t.Logf("✓ run completed with status: %s", status)

	if status != "succeeded" {
		errMsg := ""
		if em, ok := finalRun["error_message"].(map[string]any); ok {
			if s, ok := em["String"].(string); ok {
				errMsg = s
			}
		}
		t.Fatalf("run failed: %s", errMsg)
	}

	// 6. Check output contains "4"
	output, _ := finalRun["output_text"].(string)

	t.Logf("✓ output: %q", output)
	if !strings.Contains(output, "4") {
		t.Fatalf("expected output to contain '4', got: %q", output)
	}

	// 7. Check events were persisted
	events := e2eGetEvents(t, app, token, runID)
	t.Logf("✓ %d events persisted", len(events))
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	t.Log("✓ E2E test passed!")
}

// --- helpers ---

func e2eRegister(t *testing.T, app *fiber.App, email, name, password string) string {
	t.Helper()
	body := `{"email":"` + email + `","name":"` + name + `","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result["token"].(string)
}

func e2eStoreKey(t *testing.T, app *fiber.App, token, provider, key string) {
	t.Helper()
	body := `{"provider":"` + provider + `","label":"E2E Key","key":"` + key + `","is_default":true}`
	req := httptest.NewRequest("POST", "/api/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("store key: %d: %s", resp.StatusCode, b)
	}
}

func e2eCreateAgent(t *testing.T, app *fiber.App, token, name, prompt, model, baseURL string) string {
	t.Helper()
	// Store base URL in config_yaml so the runner picks it up
	configYAML := "name: " + name + "\nmodel: " + model + "\nmax_turns: 3\n"
	body := `{"name":"` + name + `","system_prompt":"` + prompt + `","model_name":"` + model + `","model_provider":"openai","config_yaml":` + jsonStr(configYAML) + `}`
	req := httptest.NewRequest("POST", "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create agent: %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result["id"].(string)
}

func e2eTriggerRun(t *testing.T, app *fiber.App, token, agentID, mission, baseURL string) string {
	t.Helper()
	body := `{"agent_id":"` + agentID + `","mission":"` + mission + `","base_url":"` + baseURL + `"}`
	req := httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	if resp.StatusCode != 202 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("trigger run: %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result["id"].(string)
}

func e2eGetRun(t *testing.T, app *fiber.App, token, runID string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func e2eGetEvents(t *testing.T, app *fiber.App, token, runID string) []any {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/runs/"+runID+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	events, _ := result["events"].([]any)
	return events
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
