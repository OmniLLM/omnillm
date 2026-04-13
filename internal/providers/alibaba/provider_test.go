package alibaba

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"omnimodel/internal/database"
	"omnimodel/internal/providers/types"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "alibaba-provider-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := database.InitializeDatabase(dir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// ─── NormalizeAPIPlan ────────────────────────────────────────────────────────

func TestNormalizeAPIPlan(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "standard"},
		{"standard", "standard"},
		{"STANDARD", "standard"},
		{"coding", "coding-plan"},
		{"coding-plan", "coding-plan"},
		{"coding_plan", "coding-plan"},
		{"CODING", "coding-plan"},
		{"other", "standard"},
	}
	for _, tc := range cases {
		got := NormalizeAPIPlan(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeAPIPlan(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── DefaultAPIBaseURL ───────────────────────────────────────────────────────

func TestDefaultAPIBaseURL(t *testing.T) {
	cases := []struct {
		plan   string
		region string
		want   string
	}{
		{"standard", "global", BaseURLGlobal},
		{"standard", "china", BaseURLChina},
		{"standard", "", BaseURLGlobal},
		{"coding-plan", "global", CodingPlanBaseURLGlobal},
		{"coding-plan", "china", CodingPlanBaseURLChina},
		{"coding", "china", CodingPlanBaseURLChina},
	}
	for _, tc := range cases {
		got := DefaultAPIBaseURL(tc.plan, tc.region)
		if got != tc.want {
			t.Errorf("DefaultAPIBaseURL(%q, %q) = %q, want %q", tc.plan, tc.region, got, tc.want)
		}
	}
}

// ─── EnsureBaseURL ───────────────────────────────────────────────────────────

func TestEnsureBaseURL(t *testing.T) {
	cases := []struct {
		raw      string
		forOAuth bool
		want     string
	}{
		{"", false, BaseURLGlobal},
		{"", true, "https://portal.qwen.ai/v1"},
		{"portal.qwen.ai", true, "https://portal.qwen.ai/v1"},
		{"https://portal.qwen.ai/v1", true, "https://portal.qwen.ai/v1"},
		{"https://portal.qwen.ai/v1/", true, "https://portal.qwen.ai/v1"},
		{"https://dashscope-intl.aliyuncs.com/compatible-mode", false, "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
		{"  https://portal.qwen.ai/v1  ", false, "https://portal.qwen.ai/v1"},
	}
	for _, tc := range cases {
		got := EnsureBaseURL(tc.raw, tc.forOAuth)
		if got != tc.want {
			t.Errorf("EnsureBaseURL(%q, %v) = %q, want %q", tc.raw, tc.forOAuth, got, tc.want)
		}
	}
}

// ─── IsJWT ───────────────────────────────────────────────────────────────────

func TestIsJWT(t *testing.T) {
	cases := []struct {
		token string
		want  bool
	}{
		{"aaa.bbb.ccc", true},
		{"eyJhbGciOiJSUzI1NiJ9.eyJlbWFpbCI6InRlc3RAZXhhbXBsZS5jb20ifQ.signature", true},
		{"sk-1234567890abcdef", false},
		{"", false},
		{"only.two", false},
		{"a.b.c.d", false},
	}
	for _, tc := range cases {
		got := IsJWT(tc.token)
		if got != tc.want {
			t.Errorf("IsJWT(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

// ─── ShortTokenSuffix ────────────────────────────────────────────────────────

func TestShortTokenSuffix(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		{"sk-1234567890abcdef", "bcdef"},
		{"abc", "abc"},
		{"", "oauth"},
		{"  sk-xyz  ", "  xyz"},
	}
	for _, tc := range cases {
		got := ShortTokenSuffix(tc.token)
		if got != tc.want {
			t.Errorf("ShortTokenSuffix(%q) = %q, want %q", tc.token, got, tc.want)
		}
	}
}

// ─── ExtractEmailFromJWT ─────────────────────────────────────────────────────

func TestExtractEmailFromJWT(t *testing.T) {
	// Build a minimal JWT with an email claim using standard library
	buildJWT := func(payload string) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		p := base64.RawURLEncoding.EncodeToString([]byte(payload))
		return header + "." + p + ".sig"
	}

	t.Run("extracts email", func(t *testing.T) {
		token := buildJWT(`{"email":"user@example.com","sub":"123"}`)
		got := ExtractEmailFromJWT(token)
		if got != "user@example.com" {
			t.Errorf("got %q, want %q", got, "user@example.com")
		}
	})

	t.Run("returns empty for non-JWT", func(t *testing.T) {
		got := ExtractEmailFromJWT("sk-not-a-jwt")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("returns empty when no email claim", func(t *testing.T) {
		token := buildJWT(`{"sub":"123"}`)
		got := ExtractEmailFromJWT(token)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

// ─── IsChatCompletionsModel ──────────────────────────────────────────────────

func TestIsChatCompletionsModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen-turbo", true},
		{"qwen-realtime-v1", false},
		{"REALTIME-model", false},
	}
	for _, tc := range cases {
		got := IsChatCompletionsModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsChatCompletionsModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

// ─── NormalizeBaseURL ────────────────────────────────────────────────────────

func TestNormalizeBaseURL(t *testing.T) {
	t.Run("api-key standard global", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key", "plan": "standard", "region": "global"}
		got := NormalizeBaseURL(cfg)
		if got != BaseURLGlobal {
			t.Errorf("got %q, want %q", got, BaseURLGlobal)
		}
	})

	t.Run("api-key coding-plan china", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key", "plan": "coding-plan", "region": "china"}
		got := NormalizeBaseURL(cfg)
		if got != CodingPlanBaseURLChina {
			t.Errorf("got %q, want %q", got, CodingPlanBaseURLChina)
		}
	})

	t.Run("oauth uses portal URL", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "oauth"}
		got := NormalizeBaseURL(cfg)
		if got != "https://portal.qwen.ai/v1" {
			t.Errorf("got %q, want %q", got, "https://portal.qwen.ai/v1")
		}
	})

	t.Run("oauth resource_url wins", func(t *testing.T) {
		cfg := map[string]interface{}{
			"auth_type":    "oauth",
			"resource_url": "https://portal.qwen.ai",
		}
		got := NormalizeBaseURL(cfg)
		if got != "https://portal.qwen.ai/v1" {
			t.Errorf("got %q, want %q", got, "https://portal.qwen.ai/v1")
		}
	})
}

// ─── APIKeyProviderName ──────────────────────────────────────────────────────

func TestAPIKeyProviderName(t *testing.T) {
	t.Run("standard plan", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "standard", "region": "global"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba DashScope Standard (global)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("coding plan", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "coding-plan", "region": "china"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba Coding Plan (china)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("empty region defaults to global", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "standard"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba DashScope Standard (global)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// ─── Headers ─────────────────────────────────────────────────────────────────

func TestHeaders(t *testing.T) {
	t.Run("api-key headers", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key"}
		h := Headers("my-token", false, cfg)
		if h["Authorization"] != "Bearer my-token" {
			t.Errorf("Authorization = %q", h["Authorization"])
		}
		if h["Content-Type"] != "application/json" {
			t.Errorf("Content-Type = %q", h["Content-Type"])
		}
		if _, ok := h["User-Agent"]; ok {
			t.Error("api-key should not set User-Agent")
		}
	})

	t.Run("oauth headers include stainless and user-agent", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "oauth"}
		h := Headers("oauth-token", false, cfg)
		if h["User-Agent"] != UserAgent {
			t.Errorf("User-Agent = %q, want %q", h["User-Agent"], UserAgent)
		}
		if h["X-DashScope-AuthType"] != "qwen-oauth" {
			t.Errorf("X-DashScope-AuthType = %q", h["X-DashScope-AuthType"])
		}
		if h["X-DashScope-CacheControl"] != "enable" {
			t.Errorf("X-DashScope-CacheControl = %q", h["X-DashScope-CacheControl"])
		}
	})

	t.Run("stream sets text/event-stream accept", func(t *testing.T) {
		cfg := map[string]interface{}{}
		h := Headers("tok", true, cfg)
		if h["Accept"] != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", h["Accept"])
		}
	})
}

// ─── GetModelsHardcoded ──────────────────────────────────────────────────────

func TestGetModelsHardcoded(t *testing.T) {
	t.Run("standard returns all models", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key"}
		resp := GetModelsHardcoded("alibaba-1", cfg)
		if len(resp.Data) != len(Models) {
			t.Errorf("got %d models, want %d", len(resp.Data), len(Models))
		}
		for _, m := range resp.Data {
			if m.Provider != "alibaba-1" {
				t.Errorf("model %q has provider %q, want alibaba-1", m.ID, m.Provider)
			}
		}
	})

	t.Run("oauth returns only supported models", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "oauth"}
		resp := GetModelsHardcoded("alibaba-oauth-1", cfg)
		if len(resp.Data) != len(OAuthSupportedModels) {
			t.Errorf("got %d models, want %d", len(resp.Data), len(OAuthSupportedModels))
		}
		for _, m := range resp.Data {
			if !OAuthSupportedModels[m.ID] {
				t.Errorf("unexpected model %q in OAuth catalog", m.ID)
			}
		}
	})
}

// ─── FetchModelsFromAPI ──────────────────────────────────────────────────────

func TestFetchModelsFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "qwen3-max"},
				{"id": "qwen-realtime-v1"}, // should be filtered out
				{"id": "unknown-model-xyz"},
			},
		})
	}))
	defer srv.Close()

	resp, err := FetchModelsFromAPI("alibaba-1", "test-token", srv.URL+"/v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// qwen3-max kept, qwen-realtime-v1 filtered, unknown-model-xyz kept
	if len(resp.Data) != 2 {
		t.Errorf("got %d models, want 2; models: %v", len(resp.Data), modelIDs(resp.Data))
	}

	// Verify metadata enrichment for known model
	var qwenMax *types.Model
	for i := range resp.Data {
		if resp.Data[i].ID == "qwen3-max" {
			qwenMax = &resp.Data[i]
			break
		}
	}
	if qwenMax == nil {
		t.Fatal("qwen3-max not found in response")
	}
	if qwenMax.Name != "Qwen3 Max" {
		t.Errorf("qwen3-max name = %q, want %q", qwenMax.Name, "Qwen3 Max")
	}
	if qwenMax.MaxTokens != 32768 {
		t.Errorf("qwen3-max MaxTokens = %d, want 32768", qwenMax.MaxTokens)
	}
}

func TestFetchModelsFromAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchModelsFromAPI("alibaba-1", "bad-token", srv.URL+"/v1")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ─── EnsureOAuthSystemMessage ─────────────────────────────────────────────────

func TestEnsureOAuthSystemMessage(t *testing.T) {
	t.Run("injects qwen header when no system message", func(t *testing.T) {
		messages := []map[string]interface{}{
			{"role": "user", "content": "hello"},
		}
		result := EnsureOAuthSystemMessage(messages)
		if len(result) < 1 {
			t.Fatal("expected at least 1 message")
		}
		if result[0]["role"] != "system" {
			t.Errorf("first message role = %q, want system", result[0]["role"])
		}
		if result[0]["content"] != "You are Qwen Code." {
			t.Errorf("content = %q", result[0]["content"])
		}
	})

	t.Run("merges existing system messages", func(t *testing.T) {
		messages := []map[string]interface{}{
			{"role": "system", "content": "Be concise."},
			{"role": "user", "content": "hello"},
		}
		result := EnsureOAuthSystemMessage(messages)
		want := "You are Qwen Code.\n\nBe concise."
		if result[0]["content"] != want {
			t.Errorf("content = %q, want %q", result[0]["content"], want)
		}
	})

	t.Run("deduplicates qwen header", func(t *testing.T) {
		messages := []map[string]interface{}{
			{"role": "system", "content": "You are Qwen Code."},
			{"role": "user", "content": "hello"},
		}
		result := EnsureOAuthSystemMessage(messages)
		if result[0]["content"] != "You are Qwen Code." {
			t.Errorf("content = %q", result[0]["content"])
		}
	})
}

// ─── SetupAPIKeyAuth ─────────────────────────────────────────────────────────

func TestSetupAPIKeyAuth(t *testing.T) {
	t.Run("requires API key", func(t *testing.T) {
		_, _, _, _, err := SetupAPIKeyAuth("alibaba-1", &types.AuthOptions{})
		if err == nil {
			t.Error("expected error for missing API key")
		}
	})

	t.Run("saves token and returns correct values", func(t *testing.T) {
		token, baseURL, name, cfg, err := SetupAPIKeyAuth("alibaba-test-1", &types.AuthOptions{
			APIKey: "sk-test-key",
			Region: "global",
			Plan:   "standard",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "sk-test-key" {
			t.Errorf("token = %q, want %q", token, "sk-test-key")
		}
		if baseURL != BaseURLGlobal {
			t.Errorf("baseURL = %q, want %q", baseURL, BaseURLGlobal)
		}
		if name != "Alibaba DashScope Standard (global)" {
			t.Errorf("name = %q", name)
		}
		if cfg["auth_type"] != "api-key" {
			t.Errorf("config auth_type = %v", cfg["auth_type"])
		}

		// Verify persisted
		store := database.NewTokenStore()
		rec, err := store.Get("alibaba-test-1")
		if err != nil || rec == nil {
			t.Fatalf("token not persisted: err=%v", err)
		}
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func modelIDs(models []types.Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = fmt.Sprintf("%s(%s)", m.ID, m.Name)
	}
	return ids
}
