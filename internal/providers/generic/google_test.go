package generic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"strings"
	"testing"
)

// ─── Google provider auth tests ───

func TestGoogleSetupAuthRequiresAPIKey(t *testing.T) {
	provider := NewGenericProvider("google", "google-test", "Google")
	err := provider.SetupAuth(&types.AuthOptions{Method: "api-key"})
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGoogleSetupAuthAPIKey(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	provider := NewGenericProvider("google", "google-test", "Google")
	err := provider.SetupAuth(&types.AuthOptions{Method: "api-key", APIKey: "test-api-key-123"})
	if err != nil {
		t.Fatalf("SetupAuth() error = %v", err)
	}

	if provider.GetToken() != "test-api-key-123" {
		t.Fatalf("expected token 'test-api-key-123', got %q", provider.GetToken())
	}
	if provider.baseURL != "https://generativelanguage.googleapis.com" {
		t.Fatalf("expected base URL 'https://generativelanguage.googleapis.com', got %q", provider.baseURL)
	}
}

func TestGoogleLoadFromDB(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("google-1", "google", map[string]any{
		"access_token": "saved-api-key",
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	provider := NewGenericProvider("google", "google-1", "Google")
	if err := provider.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}
	if provider.GetToken() != "saved-api-key" {
		t.Fatalf("expected token 'saved-api-key', got %q", provider.GetToken())
	}
}

// ─── Google provider headers tests ───

func TestGoogleGetHeadersUsesXGoogAPIKey(t *testing.T) {
	provider := NewGenericProvider("google", "google-test", "Google")
	provider.token = "my-gemini-key"
	headers := provider.GetHeaders(false)

	if headers["x-goog-api-key"] != "my-gemini-key" {
		t.Fatalf("expected x-goog-api-key header 'my-gemini-key', got %q", headers["x-goog-api-key"])
	}
	if headers["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type 'application/json', got %q", headers["Content-Type"])
	}
}

// ─── Google provider models tests ───

func TestGoogleGetModelsFetchesFromAPI(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	// Serve a mock Google list-models response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-2.5-flash",
					"displayName":                "Gemini 2.5 Flash",
					"outputTokenLimit":           65536,
					"supportedGenerationMethods": []string{"generateContent", "countTokens"},
				},
				{
					"name":                       "models/gemini-2.0-flash",
					"displayName":                "Gemini 2.0 Flash",
					"outputTokenLimit":           8192,
					"supportedGenerationMethods": []string{"generateContent"},
				},
				{
					// TTS: must be filtered out
					"name":                       "models/gemini-2.5-flash-preview-tts",
					"displayName":                "TTS",
					"outputTokenLimit":           16000,
					"supportedGenerationMethods": []string{"generateAudio"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("google-1", "google", map[string]any{
		"access_token": "test-api-key",
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	provider := NewGenericProvider("google", "google-1", "Google")
	if err := provider.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}
	// Point to the mock server
	provider.baseURL = server.URL

	models, err := provider.GetModels()
	if err != nil {
		t.Fatalf("GetModels() error = %v", err)
	}
	if models.Object != "list" {
		t.Fatalf("expected object='list', got %q", models.Object)
	}

	// Only the two generateContent models should be returned
	if len(models.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models.Data))
	}

	byID := make(map[string]types.Model)
	for _, m := range models.Data {
		byID[m.ID] = m
	}

	flash, ok := byID["gemini-2.5-flash"]
	if !ok {
		t.Fatal("expected gemini-2.5-flash in results")
	}
	if flash.MaxTokens != 65536 {
		t.Fatalf("expected maxTokens=65536, got %d", flash.MaxTokens)
	}
	if flash.Provider != "google-1" {
		t.Fatalf("expected provider='google-1', got %q", flash.Provider)
	}

	if _, ok := byID["gemini-2.0-flash"]; !ok {
		t.Fatal("expected gemini-2.0-flash in results")
	}
}

// ─── Google adapter payload tests ───

func TestGoogleBuildOpenAIPayloadUsesGeminiFormat(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	adapter := &GenericAdapter{provider: provider}

	systemPrompt := "You are a helpful assistant."
	maxTokens := 4096
	temp := 0.7
	stop := []string{"STOP", "END"}
	request := &cif.CanonicalRequest{
		Model:        "gemini-2.5-flash",
		SystemPrompt: &systemPrompt,
		MaxTokens:    &maxTokens,
		Temperature:  &temp,
		Stop:         stop,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Hello"},
				},
			},
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Hi there!"},
				},
			},
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "How are you?"},
				},
			},
		},
	}

	payload := adapter.buildGooglePayload(request)

	// Check model
	if payload["model"] != "gemini-2.5-flash" {
		t.Fatalf("expected model 'gemini-2.5-flash', got %v", payload["model"])
	}

	// Check system instruction
	sysInst, ok := payload["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("expected systemInstruction to be present")
	}
	parts, ok := sysInst["parts"].([]map[string]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("expected systemInstruction.parts with 1 element, got %#v", sysInst["parts"])
	}
	if parts[0]["text"] != systemPrompt {
		t.Fatalf("expected system prompt %q, got %v", systemPrompt, parts[0]["text"])
	}

	// Check contents
	contents, ok := payload["contents"].([]map[string]any)
	if !ok {
		t.Fatalf("expected contents to be present")
	}
	if len(contents) != 3 {
		t.Fatalf("expected 3 content blocks (user + model + user), got %d", len(contents))
	}

	// Check generationConfig
	genConfig, ok := payload["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("expected generationConfig to be present")
	}
	if genConfig["maxOutputTokens"].(int) != 4096 {
		t.Fatalf("expected maxOutputTokens=4096, got %v", genConfig["maxOutputTokens"])
	}
	if genConfig["temperature"].(float64) != 0.7 {
		t.Fatalf("expected temperature=0.7, got %v", genConfig["temperature"])
	}
	stopSeqs, ok := genConfig["stopSequences"].([]string)
	if !ok {
		t.Fatal("expected stopSequences to be present")
	}
	if len(stopSeqs) != 2 {
		t.Fatalf("expected 2 stopSequences, got %d", len(stopSeqs))
	}
}

func TestGoogleBuildOpenAIPayloadIncludesTools(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	adapter := &GenericAdapter{provider: provider}

	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Check the weather"},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "get_weather",
				Description: ptr("Get current weather for a location"),
				ParametersSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	payload := adapter.buildGooglePayload(request)

	tools, ok := payload["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in payload, got %#v", payload["tools"])
	}

	funcDecls, ok := tools[0]["functionDeclarations"].([]map[string]any)
	if !ok || len(funcDecls) != 1 {
		t.Fatalf("expected functionDeclarations, got %#v", tools[0])
	}

	decl := funcDecls[0]
	if decl["name"] != "get_weather" {
		t.Fatalf("expected tool name 'get_weather', got %v", decl["name"])
	}
	if decl["description"] != "Get current weather for a location" {
		t.Fatalf("expected description, got %v", decl["description"])
	}

	params, ok := decl["parameters"].(map[string]any)
	if !ok {
		t.Fatal("expected parameters to be present")
	}
	if params["type"] != "object" {
		t.Fatalf("expected params type 'object', got %v", params["type"])
	}
}

func TestGoogleURL(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	provider.baseURL = "https://generativelanguage.googleapis.com"
	adapter := &GenericAdapter{provider: provider}

	url := adapter.googleURL("gemini-2.5-flash")
	expected := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse"
	if url != expected {
		t.Fatalf("expected URL %q, got %q", expected, url)
	}
}

func TestGoogleURLFallsBackToDefault(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	provider.baseURL = ""
	adapter := &GenericAdapter{provider: provider}

	url := adapter.googleURL("gemini-3-pro-high")
	if !strings.Contains(url, "generativelanguage.googleapis.com") {
		t.Fatalf("expected default base URL in %q", url)
	}
}

// ─── Google SSE parser tests ───

func TestParseGoogleGeminiSSETextStream(t *testing.T) {
	// Simulate a Gemini streaming response with multiple text chunks
	sseData := `{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":""}]}`
	sseData2 := `{"candidates":[{"content":{"parts":[{"text":" there!"}],"role":"model"},"finishReason":""}]}`
	sseData3 := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP","finishMessage":""}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`

	body := io.NopCloser(strings.NewReader("data: " + sseData + "\n\ndata: " + sseData2 + "\n\ndata: " + sseData3 + "\n\n"))

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseGoogleGeminiSSE(body, eventCh)

	var events []cif.CIFStreamEvent
	for e := range eventCh {
		events = append(events, e)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (start, deltas, end), got %d", len(events))
	}

	// First event should be stream start
	start, ok := events[0].(cif.CIFStreamStart)
	if !ok {
		t.Fatalf("expected first event to be CIFStreamStart, got %T", events[0])
	}
	if start.Model != "google" {
		t.Fatalf("expected model 'google', got %q", start.Model)
	}

	// Find the stream end event
	var foundEnd bool
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			foundEnd = true
			if end.StopReason != cif.StopReasonEndTurn {
				t.Fatalf("expected stop reason 'end_turn', got %q", end.StopReason)
			}
			if end.Usage == nil {
				t.Fatal("expected usage to be present")
			}
			if end.Usage.InputTokens != 10 {
				t.Fatalf("expected 10 input tokens, got %d", end.Usage.InputTokens)
			}
			if end.Usage.OutputTokens != 5 {
				t.Fatalf("expected 5 output tokens, got %d", end.Usage.OutputTokens)
			}
		}
	}
	if !foundEnd {
		t.Fatal("expected CIFStreamEnd event")
	}
}

func TestParseGoogleGeminiSSEToolCall(t *testing.T) {
	sseData := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"location":"Seattle"}}}],"role":"model"},"finishReason":"FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":8}}`

	body := io.NopCloser(strings.NewReader("data: " + sseData + "\n\n"))

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseGoogleGeminiSSE(body, eventCh)

	var events []cif.CIFStreamEvent
	for e := range eventCh {
		events = append(events, e)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// Find content delta with tool call
	var foundToolCall bool
	for _, e := range events {
		if delta, ok := e.(cif.CIFContentDelta); ok {
			if tc, ok := delta.ContentBlock.(cif.CIFToolCallPart); ok {
				foundToolCall = true
				if tc.ToolName != "get_weather" {
					t.Fatalf("expected tool name 'get_weather', got %q", tc.ToolName)
				}
				args, _ := tc.ToolArguments["location"].(string)
				if args != "Seattle" {
					t.Fatalf("expected location 'Seattle', got %v", tc.ToolArguments)
				}
			}
		}
	}
	if !foundToolCall {
		t.Fatal("expected tool call in events")
	}

	// Verify stop reason
	var foundEnd bool
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			foundEnd = true
			if end.StopReason != cif.StopReasonToolUse {
				t.Fatalf("expected stop reason 'tool_use', got %q", end.StopReason)
			}
		}
	}
	if !foundEnd {
		t.Fatal("expected CIFStreamEnd event")
	}
}

func TestParseGoogleGeminiSSESafetyFilter(t *testing.T) {
	sseData := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"SAFETY"}]}`

	body := io.NopCloser(strings.NewReader("data: " + sseData + "\n\n"))

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseGoogleGeminiSSE(body, eventCh)

	var foundEnd bool
	for e := range eventCh {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			foundEnd = true
			if end.StopReason != cif.StopReasonContentFilter {
				t.Fatalf("expected stop reason 'content_filter', got %q", end.StopReason)
			}
		}
	}
	if !foundEnd {
		t.Fatal("expected CIFStreamStreamEnd event")
	}
}

func TestGoogleStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   cif.CIFStopReason
	}{
		{"STOP", cif.StopReasonEndTurn},
		{"MAX_TOKENS", cif.StopReasonMaxTokens},
		{"FUNCTION_CALL", cif.StopReasonToolUse},
		{"SAFETY", cif.StopReasonContentFilter},
		{"RECITATION", cif.StopReasonContentFilter},
		{"LANGUAGE", cif.StopReasonContentFilter},
		{"UNKNOWN", cif.StopReasonEndTurn},
		{"", cif.StopReasonEndTurn},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			got := googleStopReason(tt.reason)
			if got != tt.want {
				t.Fatalf("googleStopReason(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

// ─── Google CIF to Gemini message conversion tests ───

func TestCIFMessagesToGeminiForGoogle(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFUserMessage{
			Role: "user",
			Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello"},
			},
		},
		cif.CIFAssistantMessage{
			Role: "assistant",
			Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hi!"},
			},
		},
		cif.CIFUserMessage{
			Role: "user",
			Content: []cif.CIFContentPart{
				cif.CIFToolResultPart{
					ToolCallID: "call_123",
					ToolName:   "get_weather",
					Content:    `{"temp": 72}`,
				},
			},
		},
	}

	contents := cifMessagesToGemini(messages)

	if len(contents) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(contents))
	}

	// First: user message
	if contents[0]["role"] != "user" {
		t.Fatalf("expected first role 'user', got %v", contents[0]["role"])
	}

	// Second: model message
	if contents[1]["role"] != "model" {
		t.Fatalf("expected second role 'model', got %v", contents[1]["role"])
	}

	// Third: user message with function response
	if contents[2]["role"] != "user" {
		t.Fatalf("expected third role 'user', got %v", contents[2]["role"])
	}
	parts := contents[2]["parts"].([]map[string]any)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part in function response, got %d", len(parts))
	}
	if _, ok := parts[0]["functionResponse"]; !ok {
		t.Fatal("expected functionResponse part")
	}
}

// ─── Google end-to-end streaming test ───

func TestGoogleStreamEndToEnd(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	var receivedPath string
	var receivedAuth string
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("x-goog-api-key")

		// Decode request body
		json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")

		// Send streaming response
		flusher := w.(http.Flusher)

		chunk1 := `{"candidates":[{"content":{"parts":[{"text":"The "}],"role":"model"},"finishReason":""}]}`
		chunk2 := `{"candidates":[{"content":{"parts":[{"text":"weather is sunny."}],"role":"model"},"finishReason":""}]}`
		chunk3 := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":6}}`

		for _, chunk := range []string{chunk1, chunk2, chunk3} {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("google-e2e", "google", map[string]any{
		"access_token": "e2e-test-key",
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	provider := NewGenericProvider("google", "google-e2e", "Google")
	if err := provider.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	// Override baseURL to point to test server
	provider.baseURL = server.URL
	adapter := provider.GetAdapter().(*GenericAdapter)

	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "What's the weather?"},
				},
			},
		},
	}

	resp, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify request was sent correctly
	if !strings.HasSuffix(receivedPath, ":streamGenerateContent") {
		t.Fatalf("expected URL to end with ':streamGenerateContent', got %q", receivedPath)
	}
	if receivedAuth != "e2e-test-key" {
		t.Fatalf("expected x-goog-api-key 'e2e-test-key', got %q", receivedAuth)
	}

	// Verify request body - Google doesn't put model in body for streaming
	if !strings.Contains(receivedPath, "gemini-2.5-flash") {
		t.Fatalf("expected URL to contain 'gemini-2.5-flash', got %q", receivedPath)
	}

	// Verify response
	if resp.StopReason != cif.StopReasonEndTurn {
		t.Fatalf("expected stop reason 'end_turn', got %q", resp.StopReason)
	}

	// Check content was assembled from stream
	var textParts []string
	for _, part := range resp.Content {
		if tp, ok := part.(cif.CIFTextPart); ok {
			textParts = append(textParts, tp.Text)
		}
	}
	if len(textParts) == 0 {
		t.Fatal("expected text content in response")
	}
	fullText := strings.Join(textParts, "")
	if !strings.Contains(fullText, "weather") {
		t.Fatalf("expected response to contain 'weather', got %q", fullText)
	}

	// Verify usage
	if resp.Usage == nil {
		t.Fatal("expected usage to be present")
	}
	if resp.Usage.InputTokens != 5 {
		t.Fatalf("expected 5 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 6 {
		t.Fatalf("expected 6 output tokens, got %d", resp.Usage.OutputTokens)
	}
}

func TestGoogleStreamEndToEndWithTools(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		chunk := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"location":"Portland"}}}],"role":"model"},"finishReason":"FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
		_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("google-tools", "google", map[string]any{
		"access_token": "tool-test-key",
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	provider := NewGenericProvider("google", "google-tools", "Google")
	if err := provider.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	provider.baseURL = server.URL
	adapter := provider.GetAdapter().(*GenericAdapter)

	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "What's the weather in Portland?"},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "get_weather",
				Description: ptr("Get weather info"),
				ParametersSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	resp, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify tools were sent in request
	tools, ok := receivedBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in request, got %#v", receivedBody["tools"])
	}

	// Verify response has tool call
	var foundToolCall bool
	for _, part := range resp.Content {
		if tc, ok := part.(cif.CIFToolCallPart); ok {
			foundToolCall = true
			if tc.ToolName != "get_weather" {
				t.Fatalf("expected tool name 'get_weather', got %q", tc.ToolName)
			}
			loc, _ := tc.ToolArguments["location"].(string)
			if loc != "Portland" {
				t.Fatalf("expected location 'Portland', got %v", tc.ToolArguments)
			}
		}
	}
	if !foundToolCall {
		t.Fatal("expected tool call in response")
	}
	if resp.StopReason != cif.StopReasonToolUse {
		t.Fatalf("expected stop reason 'tool_use', got %q", resp.StopReason)
	}
}

func TestGoogleStreamEndToEndWithSystemPrompt(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GetDatabase().Close()
	})

	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		chunk := `{"candidates":[{"content":{"parts":[{"text":"OK"}],"role":"model"},"finishReason":"STOP"}]}`
		_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("google-sys", "google", map[string]any{
		"access_token": "sys-test-key",
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	provider := NewGenericProvider("google", "google-sys", "Google")
	if err := provider.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	provider.baseURL = server.URL
	adapter := provider.GetAdapter().(*GenericAdapter)

	systemPrompt := "Always respond concisely."
	request := &cif.CanonicalRequest{
		Model:        "gemini-2.0-flash",
		SystemPrompt: &systemPrompt,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "hi"},
				},
			},
		},
	}

	_, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	sysInst, ok := receivedBody["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("expected systemInstruction in request body")
	}
	parts, ok := sysInst["parts"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("expected systemInstruction.parts, got %#v", receivedBody["systemInstruction"])
	}
	partMap, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected part to be map, got %T", parts[0])
	}
	if partMap["text"] != systemPrompt {
		t.Fatalf("expected system prompt %q, got %v", systemPrompt, partMap["text"])
	}
}

func TestGoogleExecuteRequiresAuth(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	adapter := provider.GetAdapter().(*GenericAdapter)

	request := &cif.CanonicalRequest{
		Model: "gemini-2.5-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "hello"},
				},
			},
		},
	}

	_, err := adapter.Execute(context.Background(), request)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── fetchGoogleModels tests ───

func TestFetchGoogleModelsFiltersToGenerateContent(t *testing.T) {
	// Mock the Google /v1beta/models endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("x-goog-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1beta/models" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":             "models/gemini-2.5-flash",
					"displayName":      "Gemini 2.5 Flash",
					"description":      "Fast model",
					"outputTokenLimit": 65536,
					"supportedGenerationMethods": []string{
						"generateContent",
						"countTokens",
					},
				},
				{
					"name":             "models/gemini-2.5-pro",
					"displayName":      "Gemini 2.5 Pro",
					"description":      "Pro model",
					"outputTokenLimit": 65536,
					"supportedGenerationMethods": []string{
						"generateContent",
					},
				},
				{
					// TTS model — should be filtered out (no generateContent)
					"name":             "models/gemini-2.5-flash-preview-tts",
					"displayName":      "Gemini 2.5 Flash Preview TTS",
					"description":      "TTS model",
					"outputTokenLimit": 16000,
					"supportedGenerationMethods": []string{
						"generateAudio",
					},
				},
				{
					// Embedding model — should be filtered out
					"name":             "models/text-embedding-004",
					"displayName":      "Text Embedding 004",
					"description":      "Embedding model",
					"outputTokenLimit": 0,
					"supportedGenerationMethods": []string{
						"embedContent",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGenericProvider("google", "google-1", "Google")
	provider.token = "test-key"
	provider.baseURL = server.URL

	models, err := provider.fetchGoogleModels()
	if err != nil {
		t.Fatalf("fetchGoogleModels() error = %v", err)
	}
	if models.Object != "list" {
		t.Fatalf("expected object='list', got %q", models.Object)
	}

	// Only generateContent models should be returned
	if len(models.Data) != 2 {
		t.Fatalf("expected 2 models (only generateContent), got %d: %v",
			len(models.Data), func() []string {
				ids := make([]string, len(models.Data))
				for i, m := range models.Data {
					ids[i] = m.ID
				}
				return ids
			}())
	}

	byID := make(map[string]interface{})
	for _, m := range models.Data {
		byID[m.ID] = struct{}{}
	}
	if _, ok := byID["gemini-2.5-flash"]; !ok {
		t.Fatal("expected gemini-2.5-flash in results")
	}
	if _, ok := byID["gemini-2.5-pro"]; !ok {
		t.Fatal("expected gemini-2.5-pro in results")
	}
}

func TestFetchGoogleModelsStripsModelsPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-2.0-flash",
					"displayName":                "Gemini 2.0 Flash",
					"outputTokenLimit":           8192,
					"supportedGenerationMethods": []string{"generateContent"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGenericProvider("google", "google-1", "Google")
	provider.token = "test-key"
	provider.baseURL = server.URL

	models, err := provider.fetchGoogleModels()
	if err != nil {
		t.Fatalf("fetchGoogleModels() error = %v", err)
	}
	if len(models.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models.Data))
	}
	// ID must NOT have the "models/" prefix
	if models.Data[0].ID != "gemini-2.0-flash" {
		t.Fatalf("expected ID 'gemini-2.0-flash', got %q", models.Data[0].ID)
	}
	// Provider must be the instance ID
	if models.Data[0].Provider != "google-1" {
		t.Fatalf("expected provider 'google-1', got %q", models.Data[0].Provider)
	}
	// MaxTokens from outputTokenLimit
	if models.Data[0].MaxTokens != 8192 {
		t.Fatalf("expected maxTokens 8192, got %d", models.Data[0].MaxTokens)
	}
}

func TestFetchGoogleModelsDefaultsMaxTokensWhenZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name":                       "models/gemini-1.5-flash",
					"displayName":                "Gemini 1.5 Flash",
					"outputTokenLimit":           0, // missing / zero
					"supportedGenerationMethods": []string{"generateContent"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGenericProvider("google", "google-1", "Google")
	provider.token = "test-key"
	provider.baseURL = server.URL

	models, err := provider.fetchGoogleModels()
	if err != nil {
		t.Fatalf("fetchGoogleModels() error = %v", err)
	}
	if models.Data[0].MaxTokens != 8192 {
		t.Fatalf("expected default maxTokens=8192 when outputTokenLimit=0, got %d", models.Data[0].MaxTokens)
	}
}

func TestFetchGoogleModelsRequiresToken(t *testing.T) {
	provider := NewGenericProvider("google", "google-1", "Google")
	// No token set
	_, err := provider.fetchGoogleModels()
	if err == nil {
		t.Fatal("expected error when token is empty")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchGoogleModelsHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"API key not valid"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewGenericProvider("google", "google-1", "Google")
	provider.token = "bad-key"
	provider.baseURL = server.URL

	_, err := provider.fetchGoogleModels()
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status 400 in error, got: %v", err)
	}
}

// ─── Helpers ───

func ptr[T any](v T) *T { return &v }
