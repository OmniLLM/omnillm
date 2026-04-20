package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"omnillm/internal/database"
	"omnillm/internal/lib/ratelimit"
	"omnillm/internal/routes"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "server-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := database.InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	// Configure routes with a no-op rate limiter
	rl := ratelimit.NewRateLimiter(0, false)
	routes.ConfigureChatCompletionOptions(rl, false)

	os.Exit(m.Run())
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	r := buildRouter(0, "test-api-key")
	return httptest.NewServer(r)
}

func newAuthenticatedRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-api-key")
	return req
}

// ─── Health endpoints ───

func TestHealthRoot(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["status"] != "healthy" {
		t.Errorf("expected status=healthy, got %v", result["status"])
	}
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["status"] != "healthy" {
		t.Errorf("expected status=healthy, got %v", result["status"])
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("expected timestamp field in /health response")
	}
}

func TestHealthzEndpoint(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "OK" {
		t.Errorf("expected body=OK, got %q", string(body))
	}
}

// ─── Models endpoints ───

func TestGetModels_V1Path(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodGet, srv.URL+"/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	defer resp.Body.Close()

	// No active providers → expect 200 with empty data or a valid models structure
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, string(body))
	}
	// Must have "data" and "object" fields (OpenAI models list format)
	if _, ok := result["data"]; !ok {
		t.Error("expected 'data' field in models response")
	}
	if result["object"] != "list" {
		t.Errorf("expected object=list, got %v", result["object"])
	}
}

func TestGetModels_RootPath(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodGet, srv.URL+"/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /models: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// ─── Admin endpoints ───

func TestAdminGetProviders(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodGet, srv.URL+"/api/admin/providers", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/admin/providers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	// Endpoint returns a JSON array of provider objects
	var result []interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON array: %v\nbody: %s", err, string(body))
	}
}

func TestAdminGetStatus(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodGet, srv.URL+"/api/admin/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/admin/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestAdminGetAuthStatus(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodGet, srv.URL+"/api/admin/auth-status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/admin/auth-status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestAdminGetInfo(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/admin/info")
	if err != nil {
		t.Fatalf("GET /api/admin/info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/admin/providers")
	if err != nil {
		t.Fatalf("GET /api/admin/providers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestChatCompletions_NoActiveProvider(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reqBody := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}`
	req := newAuthenticatedRequest(t, http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	defer resp.Body.Close()

	// Without an active provider we expect a 4xx or 5xx error, not a panic
	if resp.StatusCode < 400 {
		t.Errorf("expected error status (>=400) with no active provider, got %d", resp.StatusCode)
	}
}

func TestChatCompletions_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := newAuthenticatedRequest(t, http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected error status for invalid JSON, got %d", resp.StatusCode)
	}
}

// ─── Admin redirect ───

func TestAdminRedirect(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}
	resp, err := client.Get(srv.URL + "/admin")
	if err != nil {
		t.Fatalf("GET /admin: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected 301 redirect from /admin, got %d", resp.StatusCode)
	}
}
