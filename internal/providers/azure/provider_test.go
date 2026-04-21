package azure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "azure-provider-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := database.InitializeDatabase(dir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func ptr[T any](v T) *T { return &v }

// ─── API shape routing ───────────────────────────────────────────────────────
// Azure OpenAI always uses the Responses API for all models.
// Routing is governed by internal/providers/providermodels.

func TestAzureAlwaysUsesResponsesAPI(t *testing.T) {
	models := []string{"gpt-4o", "gpt-4o-mini", "gpt-5.4", "gpt-5.4-turbo", "gpt-5.1-codex"}
	for _, model := range models {
		if !IsResponsesAPIModel(model) {
			t.Errorf("IsResponsesAPIModel(%q) = false, want true (azure always uses Responses)", model)
		}
	}
}

// ─── ToolCallID ──────────────────────────────────────────────────────────────

func TestToolCallID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"fc_abc123", "fc_abc123"},
		{"call_abc123", "fc_abc123"},
		{"call_xyz", "fc_xyz"},
		{"custom_id", "fc_custom_id"},
		{"", "fc_"},
	}
	for _, tc := range cases {
		got := ToolCallID(tc.input)
		if got != tc.want {
			t.Errorf("ToolCallID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── ChatURL ─────────────────────────────────────────────────────────────────

func TestChatURL(t *testing.T) {
	t.Run("builds URL with default api version", func(t *testing.T) {
		url, err := ChatURL("https://my-resource.openai.azure.com", "my-deployment", "")
		if err != nil {
			t.Fatal(err)
		}
		want := "https://my-resource.openai.azure.com/openai/deployments/my-deployment/chat/completions?api-version=2024-08-01-preview"
		if url != want {
			t.Errorf("got %q, want %q", url, want)
		}
	})

	t.Run("uses custom api version", func(t *testing.T) {
		url, err := ChatURL("https://endpoint.azure.com", "gpt-4o", "2025-01-01-preview")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(url, "api-version=2025-01-01-preview") {
			t.Errorf("URL %q missing api-version", url)
		}
	})

	t.Run("returns error when endpoint empty", func(t *testing.T) {
		_, err := ChatURL("", "deployment", "")
		if err == nil {
			t.Error("expected error for empty endpoint")
		}
	})
}

// ─── SetupAuth ───────────────────────────────────────────────────────────────

func TestSetupAuthRequiresAPIKey(t *testing.T) {
	_, _, _, err := SetupAuth("azure-1", &types.AuthOptions{})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestSetupAuthSavesToken(t *testing.T) {
	token, endpoint, cfg, err := SetupAuth("azure-test-1", &types.AuthOptions{
		APIKey:   "test-azure-key",
		Endpoint: "https://my-resource.openai.azure.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-azure-key" {
		t.Errorf("token = %q", token)
	}
	if endpoint != "https://my-resource.openai.azure.com" {
		t.Errorf("endpoint = %q", endpoint)
	}
	if cfg == nil {
		t.Error("config should not be nil")
	}

	store := database.NewTokenStore()
	rec, err := store.Get("azure-test-1")
	if err != nil || rec == nil {
		t.Fatalf("token not persisted: err=%v", err)
	}
}

// ─── GetModels ───────────────────────────────────────────────────────────────

func TestGetModelsDefaultCatalog(t *testing.T) {
	resp := GetModels("azure-1", map[string]interface{}{})
	if len(resp.Data) == 0 {
		t.Error("expected at least one model")
	}
	for _, m := range resp.Data {
		if m.Provider != "azure-1" {
			t.Errorf("model %q has provider %q, want azure-1", m.ID, m.Provider)
		}
	}
}

func TestGetModelsFromDeployments(t *testing.T) {
	cfg := map[string]interface{}{
		"deployments": []interface{}{"my-gpt4o", "my-gpt4o-mini"},
	}
	resp := GetModels("azure-2", cfg)
	if len(resp.Data) != 2 {
		t.Fatalf("got %d models, want 2", len(resp.Data))
	}
	if resp.Data[0].ID != "my-gpt4o" {
		t.Errorf("model[0].ID = %q, want my-gpt4o", resp.Data[0].ID)
	}
}

// ─── BuildResponsesPayload ────────────────────────────────────────────────────

func TestBuildResponsesPayloadBasic(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model: "gpt-5.4",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello"},
			}},
		},
		SystemPrompt: ptr("You are helpful."),
		MaxTokens:    ptr(500),
	}

	payload := BuildResponsesPayload(request, "gpt-5.4")

	if payload["model"] != "gpt-5.4" {
		t.Errorf("model = %v", payload["model"])
	}
	if payload["instructions"] != "You are helpful." {
		t.Errorf("instructions = %v", payload["instructions"])
	}
	if payload["max_output_tokens"].(int) != 500 {
		t.Errorf("max_output_tokens = %v", payload["max_output_tokens"])
	}
	if payload["store"] != false {
		t.Error("store should be false")
	}
}

func TestBuildResponsesPayloadWithTools(t *testing.T) {
	desc := "Get weather"
	request := &cif.CanonicalRequest{
		Model:    "gpt-5.4",
		Messages: []cif.CIFMessage{},
		Tools: []cif.CIFTool{
			{
				Name:        "get_weather",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	payload := BuildResponsesPayload(request, "gpt-5.4")

	tools, ok := payload["tools"].([]map[string]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", payload["tools"])
	}
	if tools[0]["name"] != "get_weather" {
		t.Errorf("tool name = %v", tools[0]["name"])
	}
	if tools[0]["type"] != "function" {
		t.Errorf("tool type = %v", tools[0]["type"])
	}
	if payload["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v", payload["tool_choice"])
	}
}

func TestBuildResponsesPayloadGPT54ProOmitsTemperature(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model:       "gpt-5.4-pro",
		Messages:    []cif.CIFMessage{},
		Temperature: ptr(0.5),
	}
	payload := BuildResponsesPayload(request, "gpt-5.4-pro")
	if _, ok := payload["temperature"]; ok {
		t.Error("gpt-5.4-pro should not include temperature")
	}
}

// ─── CIFMessagesToResponsesInput ─────────────────────────────────────────────

func TestCIFMessagesToResponsesInputUserText(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFUserMessage{Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "Hello"},
		}},
	}
	input := CIFMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(input))
	}
	if input[0]["type"] != "message" {
		t.Errorf("type = %v", input[0]["type"])
	}
	if input[0]["role"] != "user" {
		t.Errorf("role = %v", input[0]["role"])
	}
}

func TestCIFMessagesToResponsesInputToolResult(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFUserMessage{Content: []cif.CIFContentPart{
			cif.CIFToolResultPart{
				Type:       "tool_result",
				ToolCallID: "call_abc",
				ToolName:   "my_tool",
				Content:    "tool result text",
			},
		}},
	}
	input := CIFMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(input))
	}
	if input[0]["type"] != "function_call_output" {
		t.Errorf("type = %v", input[0]["type"])
	}
	if input[0]["call_id"] != "fc_abc" {
		t.Errorf("call_id = %v", input[0]["call_id"])
	}
	if input[0]["output"] != "tool result text" {
		t.Errorf("output = %v", input[0]["output"])
	}
}

func TestCIFMessagesToResponsesInputAssistantToolCall(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFAssistantMessage{Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "call_xyz",
				ToolName:      "search",
				ToolArguments: map[string]interface{}{"q": "test"},
			},
		}},
	}
	input := CIFMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(input))
	}
	if input[0]["type"] != "function_call" {
		t.Errorf("type = %v", input[0]["type"])
	}
	if input[0]["name"] != "search" {
		t.Errorf("name = %v", input[0]["name"])
	}
	if input[0]["call_id"] != "fc_xyz" {
		t.Errorf("call_id = %v", input[0]["call_id"])
	}
}

// ─── ResponsesRespToCIF ──────────────────────────────────────────────────────

func TestResponsesRespToCIFTextOutput(t *testing.T) {
	resp := map[string]interface{}{
		"id": "resp_123",
		"output": []interface{}{
			map[string]interface{}{
				"type": "message",
				"content": []interface{}{
					map[string]interface{}{"type": "output_text", "text": "Hello world"},
				},
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  float64(10),
			"output_tokens": float64(5),
		},
	}
	cifResp := ResponsesRespToCIF(resp, "gpt-5.4")
	if len(cifResp.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(cifResp.Content))
	}
	textPart, ok := cifResp.Content[0].(cif.CIFTextPart)
	if !ok {
		t.Fatalf("expected CIFTextPart, got %T", cifResp.Content[0])
	}
	if textPart.Text != "Hello world" {
		t.Errorf("text = %q, want %q", textPart.Text, "Hello world")
	}
	if cifResp.Usage.InputTokens != 10 || cifResp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %v", cifResp.Usage)
	}
}

func TestResponsesRespToCIFFunctionCall(t *testing.T) {
	resp := map[string]interface{}{
		"id": "resp_456",
		"output": []interface{}{
			map[string]interface{}{
				"type":      "function_call",
				"id":        "fc_abc",
				"name":      "search",
				"arguments": `{"q":"test"}`,
			},
		},
	}
	cifResp := ResponsesRespToCIF(resp, "gpt-5.4")
	if len(cifResp.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(cifResp.Content))
	}
	tc, ok := cifResp.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart, got %T", cifResp.Content[0])
	}
	if tc.ToolName != "search" {
		t.Errorf("ToolName = %q", tc.ToolName)
	}
	if cifResp.StopReason != cif.StopReasonToolUse {
		t.Errorf("StopReason = %v", cifResp.StopReason)
	}
}

func TestResponsesRespToCIFIncompleteMaxTokens(t *testing.T) {
	resp := map[string]interface{}{
		"id":     "resp_789",
		"status": "incomplete",
		"output": []interface{}{},
	}
	cifResp := ResponsesRespToCIF(resp, "gpt-5.4")
	if cifResp.StopReason != cif.StopReasonMaxTokens {
		t.Errorf("StopReason = %v, want max_tokens", cifResp.StopReason)
	}
}

// ─── ExecuteResponses end-to-end ─────────────────────────────────────────────

func TestExecuteResponsesEndToEnd(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "resp_e2e",
			"output": []interface{}{
				map[string]interface{}{
					"type": "message",
					"content": []interface{}{
						map[string]interface{}{"type": "output_text", "text": "test response"},
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  float64(20),
				"output_tokens": float64(10),
			},
		})
	}))
	defer srv.Close()

	request := &cif.CanonicalRequest{
		Model: "gpt-5.4",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello"},
			}},
		},
		MaxTokens: ptr(100),
	}

	resp, err := ExecuteResponses(context.Background(), srv.URL, "test-api-key", request, "gpt-5.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(resp.Content))
	}
	textPart, ok := resp.Content[0].(cif.CIFTextPart)
	if !ok || textPart.Text != "test response" {
		t.Errorf("unexpected content: %v", resp.Content[0])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("invalid body: %v", err)
	}
	if body["model"] != "gpt-5.4" {
		t.Errorf("request model = %v", body["model"])
	}
	if body["store"] != false {
		t.Error("store should be false")
	}
}

// ─── StreamResponses ─────────────────────────────────────────────────────────

func TestStreamResponsesEmitsEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "resp_stream",
			"output": []interface{}{
				map[string]interface{}{
					"type": "message",
					"content": []interface{}{
						map[string]interface{}{"type": "output_text", "text": "streamed response"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	request := &cif.CanonicalRequest{
		Model:    "gpt-5.4",
		Messages: []cif.CIFMessage{},
	}

	ch, err := StreamResponses(context.Background(), srv.URL, "test-key", request, "gpt-5.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []cif.CIFStreamEvent
	for event := range ch {
		events = append(events, event)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (start, delta, end), got %d", len(events))
	}
	if _, ok := events[0].(cif.CIFStreamStart); !ok {
		t.Errorf("first event should be CIFStreamStart, got %T", events[0])
	}
	if _, ok := events[len(events)-1].(cif.CIFStreamEnd); !ok {
		t.Errorf("last event should be CIFStreamEnd, got %T", events[len(events)-1])
	}
}

// ─── Headers ─────────────────────────────────────────────────────────────────

func TestAzureHeaders(t *testing.T) {
	h := Headers("my-api-key")
	if h["api-key"] != "my-api-key" {
		t.Errorf("api-key = %q", h["api-key"])
	}
	if h["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", h["Content-Type"])
	}
}
