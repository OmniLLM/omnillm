package alibaba

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"omnimodel/internal/cif"
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
		{"  sk-xyz  ", "k-xyz"},
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
		// qwen3-coder-next must be visible to OAuth users.
		found := false
		for _, m := range resp.Data {
			if m.ID == "qwen3-coder-next" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected qwen3-coder-next to be present in OAuth model list")
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

// ─── IsReasoningModel ────────────────────────────────────────────────────────

func TestIsReasoningModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen3-235b-a22b-instruct", true},
		{"qwq-32b", true},
		{"qwen-plus", true},
		{"qwen3.5-plus", true},
		{"qwen3.6-plus", true},
		{"QWEN3-MAX", true}, // case insensitive
		{"qwen2-5-72b-instruct", false},
		{"qwen2-vl-7b", false},
		{"qwen-mt-plus", false},
		{"qwen-turbo", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := IsReasoningModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsReasoningModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

// ─── BuildOpenAIPayload: enable_thinking ─────────────────────────────────────

func TestBuildOpenAIPayloadEnableThinkingWhenSet(t *testing.T) {
	// enable_thinking=true should be sent when there are no real tools in the request.
	req := &cif.CanonicalRequest{Model: "qwen3-max"}
	payload := BuildOpenAIPayload("qwen3-max", []map[string]interface{}{}, req, false, true)
	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true, got %v (present=%v)", val, ok)
	}
}

func TestBuildOpenAIPayloadNoEnableThinkingByDefault(t *testing.T) {
	req := &cif.CanonicalRequest{Model: "qwen3-max"}
	payload := BuildOpenAIPayload("qwen3-max", []map[string]interface{}{}, req, false, false)
	if val, ok := payload["enable_thinking"]; ok {
		t.Errorf("expected enable_thinking to be absent, got %v", val)
	}
}

// TestBuildOpenAIPayloadEnableThinkingSuppressedWhenToolsPresent verifies that when
// enableThinking=true AND real tools are in the request, neither enable_thinking nor
// thinking_budget is sent.  DashScope China rejects thinking_budget=0 as invalid, and
// enable_thinking=true suppresses tool_calls SSE deltas — so both must be absent.
func TestBuildOpenAIPayloadEnableThinkingSuppressedWhenToolsPresent(t *testing.T) {
	desc := "Read a file"
	req := &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Tools: []cif.CIFTool{
			{
				Name:        "read_file",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}},
				},
			},
		},
	}
	payload := BuildOpenAIPayload("qwen3.6-plus", []map[string]interface{}{}, req, false, true)

	if _, ok := payload["enable_thinking"]; ok {
		t.Error("enable_thinking must be absent when tools are present to avoid breaking tool_calls streaming")
	}
	if _, ok := payload["thinking_budget"]; ok {
		t.Error("thinking_budget must be absent when tools are present (DashScope rejects thinking_budget=0)")
	}
}

// TestBuildOpenAIPayloadEnableThinkingSetWhenNoTools verifies that when enableThinking=true
// and the request has no real tools, enable_thinking=true is still sent normally.
func TestBuildOpenAIPayloadEnableThinkingSetWhenNoTools(t *testing.T) {
	req := &cif.CanonicalRequest{Model: "qwen3-coder-next"}
	payload := BuildOpenAIPayload("qwen3-coder-next", []map[string]interface{}{}, req, false, true)

	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true when no real tools present, got %v (present=%v)", val, ok)
	}
	if _, ok := payload["thinking_budget"]; ok {
		t.Error("thinking_budget must not be set when no real tools present")
	}
}

// ─── BuildOpenAIPayload: default temperature/topP ────────────────────────────

func TestBuildOpenAIPayloadDefaultTemperature(t *testing.T) {
	req := &cif.CanonicalRequest{Model: "qwen3-max"}
	payload := BuildOpenAIPayload("qwen3-max", []map[string]interface{}{}, req, false, false)
	if temp, ok := payload["temperature"]; !ok || temp != 0.55 {
		t.Errorf("expected default temperature=0.55, got %v (present=%v)", temp, ok)
	}
	if topP, ok := payload["top_p"]; !ok || topP != 1.0 {
		t.Errorf("expected default top_p=1.0, got %v (present=%v)", topP, ok)
	}
}

func TestBuildOpenAIPayloadCallerTemperatureRespected(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &cif.CanonicalRequest{
		Model:       "qwen3-max",
		Temperature: &temp,
		TopP:        &topP,
	}
	payload := BuildOpenAIPayload("qwen3-max", []map[string]interface{}{}, req, false, false)
	if got := payload["temperature"]; got != 0.7 {
		t.Errorf("expected caller temperature 0.7, got %v", got)
	}
	if got := payload["top_p"]; got != 0.9 {
		t.Errorf("expected caller top_p 0.9, got %v", got)
	}
}

// ─── IsAnthropicMode ─────────────────────────────────────────────────────────

func TestIsAnthropicMode(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		{"empty config", map[string]interface{}{}, false},
		{"api_format anthropic", map[string]interface{}{"api_format": "anthropic"}, true},
		{"apiFormat anthropic", map[string]interface{}{"apiFormat": "anthropic"}, true},
		{"case insensitive", map[string]interface{}{"api_format": "ANTHROPIC"}, true},
		{"mixed case", map[string]interface{}{"api_format": "Anthropic"}, true},
		{"other format", map[string]interface{}{"api_format": "openai"}, false},
		{"nil config", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAnthropicMode(tc.config)
			if got != tc.want {
				t.Errorf("IsAnthropicMode(%v) = %v, want %v", tc.config, got, tc.want)
			}
		})
	}
}

// ─── AnthropicMessagesURL ────────────────────────────────────────────────────

func TestAnthropicMessagesURL(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"", AnthropicBaseURL + "/messages"},
		{"https://dashscope.aliyuncs.com/apps/anthropic/v1", "https://dashscope.aliyuncs.com/apps/anthropic/v1/messages"},
		{"https://dashscope.aliyuncs.com/apps/anthropic/v1/", "https://dashscope.aliyuncs.com/apps/anthropic/v1/messages"},
		{"  https://example.com/v1  ", "https://example.com/v1/messages"},
	}
	for _, tc := range cases {
		got := AnthropicMessagesURL(tc.baseURL)
		if got != tc.want {
			t.Errorf("AnthropicMessagesURL(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}

// ─── GetModelsAnthropicMode ──────────────────────────────────────────────────

func TestGetModelsAnthropicMode(t *testing.T) {
	resp := GetModelsAnthropicMode("alibaba-anthropic-1")
	if len(resp.Data) != len(AnthropicModels) {
		t.Errorf("got %d models, want %d", len(resp.Data), len(AnthropicModels))
	}
	for _, m := range resp.Data {
		if m.Provider != "alibaba-anthropic-1" {
			t.Errorf("model %q has provider %q, want alibaba-anthropic-1", m.ID, m.Provider)
		}
	}
	// Verify claude-sonnet-4.5 is present.
	found := false
	for _, m := range resp.Data {
		if m.ID == "claude-sonnet-4.5" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-sonnet-4.5 in AnthropicMode model list")
	}
}

func TestRemapModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    string
	}{
		{"claude-opus-4.5", "qwen3-max"},
		{"claude-sonnet-4.5", "qwen3.6-plus"},
		{"claude-sonnet-4-5-20250929", "qwen3.6-plus"},
		{"claude-haiku-4.5", "qwen3-coder-flash"},
		{"claude-haiku-4-5-20251001", "qwen3-coder-flash"},
		{"qwen3.6-plus", "qwen3.6-plus"},
	}

	for _, tc := range cases {
		got := RemapModel(tc.modelID)
		if got != tc.want {
			t.Errorf("RemapModel(%q) = %q, want %q", tc.modelID, got, tc.want)
		}
	}
}

// ─── NormalizeBaseURL: anthropic mode ────────────────────────────────────────

func TestNormalizeBaseURLAnthropicMode(t *testing.T) {
	t.Run("anthropic mode returns AnthropicBaseURL", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key", "api_format": "anthropic"}
		got := NormalizeBaseURL(cfg)
		if got != AnthropicBaseURL {
			t.Errorf("got %q, want %q", got, AnthropicBaseURL)
		}
	})

	t.Run("anthropic mode with explicit base_url", func(t *testing.T) {
		cfg := map[string]interface{}{
			"auth_type":  "api-key",
			"api_format": "anthropic",
			"base_url":   "https://custom.example.com/v1",
		}
		got := NormalizeBaseURL(cfg)
		if got != "https://custom.example.com/v1" {
			t.Errorf("got %q, want https://custom.example.com/v1", got)
		}
	})
}

// ─── GetModels: anthropic mode short-circuit ─────────────────────────────────

func TestGetModelsAnthropicModeShortCircuit(t *testing.T) {
	cfg := map[string]interface{}{"auth_type": "api-key", "api_format": "anthropic"}
	resp, err := GetModels("alibaba-ant-1", "sk-test", "https://dashscope.aliyuncs.com/apps/anthropic/v1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != len(AnthropicModels) {
		t.Errorf("got %d models, want %d (AnthropicModels)", len(resp.Data), len(AnthropicModels))
	}
}

// ─── APIKeyProviderName: anthropic mode ──────────────────────────────────────

func TestAPIKeyProviderNameAnthropicMode(t *testing.T) {
	cfg := map[string]interface{}{"auth_type": "api-key", "api_format": "anthropic"}
	got := APIKeyProviderName(cfg)
	want := "Alibaba Anthropic API"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
