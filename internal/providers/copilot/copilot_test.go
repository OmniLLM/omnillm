package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"omnillm/internal/cif"
)

func TestGetModels_PopulatesShapeCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[
						{"id":"gpt-5.4","name":"GPT-5.4","capabilities":{},"supported_endpoints":["/responses","ws:/responses"]},
						{"id":"grok-4.5","name":"Grok 4.5","capabilities":{"limits":{"max_context_window_tokens":328000,"max_output_tokens":128000},"supports":{"tool_calls":true,"parallel_tool_calls":true}},"supported_endpoints":["/responses"]},
						{"id":"grok-4.6","name":"Grok 4.6","capabilities":{"limits":{"max_context_window_tokens":328000,"max_output_tokens":128000},"supports":{"tool_calls":true,"parallel_tool_calls":true}},"supported_endpoints":["/responses"]},
						{"id":"claude-opus-4.7","name":"Claude Opus 4.7","capabilities":{},"supported_endpoints":["/v1/messages","/chat/completions"]},
						{"id":"gpt-4o","name":"GPT-4o","capabilities":{},"supported_endpoints":["/chat/completions"]}
					]}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"

	_, err := provider.GetModels()
	if err != nil {
		t.Fatalf("GetModels returned error: %v", err)
	}

	if provider.shapeCache == nil {
		t.Fatal("expected shapeCache to be populated after GetModels")
	}
	if got := provider.shapeCache["gpt-5.4"]; got != shapeResponses {
		t.Errorf("gpt-5.4: expected shapeResponses, got %q", got)
	}
	for _, model := range []string{"grok-4.5", "grok-4.6"} {
		if got := provider.shapeCache[model]; got != shapeResponses {
			t.Errorf("%s: expected shapeResponses, got %q", model, got)
		}
	}
	if got := provider.shapeCache["claude-opus-4.7"]; got != shapeChat {
		t.Errorf("claude-opus-4.7: expected shapeChat, got %q", got)
	}
	if got := provider.shapeCache["gpt-4o"]; got != shapeChat {
		t.Errorf("gpt-4o: expected shapeChat, got %q", got)
	}
}

func TestGetModels_ShapeCacheRemainsNilOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"

	_, _ = provider.GetModels() // error is expected; ignore it

	if provider.shapeCache != nil {
		t.Errorf("expected shapeCache to remain nil on server error, got %v", provider.shapeCache)
	}
}

func TestSelectShape_CacheHit(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	provider.shapeCache = modelShapeCache{
		"gpt-5.4":         shapeResponses,
		"claude-opus-4.7": shapeChat,
	}
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if got := adapter.selectShape("gpt-5.4", nil); got != shapeResponses {
		t.Errorf("expected shapeResponses for gpt-5.4, got %q", got)
	}
	if got := adapter.selectShape("claude-opus-4.7", nil); got != shapeChat {
		t.Errorf("expected shapeChat for claude-opus-4.7, got %q", got)
	}
}

func TestSelectShape_ForceChatCompletionsOverridesCache(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	provider.shapeCache = modelShapeCache{
		"gpt-5.4":  shapeResponses,
		"grok-4.5": shapeResponses,
	}
	adapter := provider.GetAdapter().(*CopilotAdapter)

	force := true
	req := &cif.CanonicalRequest{Extensions: &cif.Extensions{ForceChatCompletions: &force}}
	if got := adapter.selectShape("gpt-5.4", req); got != shapeChat {
		t.Errorf("expected shapeChat when ForceChatCompletions=true, got %q", got)
	}
	if got := adapter.selectShape("grok-4.5", req); got != shapeResponses {
		t.Errorf("expected known Responses-only Grok model to ignore ForceChatCompletions, got %q", got)
	}
}

func TestSelectShape_CacheMissFallsBackToHeuristic(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	// shapeCache is nil — simulates pre-GetModels state
	adapter := provider.GetAdapter().(*CopilotAdapter)

	for _, model := range []string{"grok-4.5", "grok-4.6", "gpt-5.5"} {
		if got := adapter.selectShape(model, nil); got != shapeResponses {
			t.Errorf("expected shapeResponses for %s heuristic fallback, got %q", model, got)
		}
	}
	if got := adapter.selectShape("claude-opus-4.7", nil); got != shapeChat {
		t.Errorf("expected shapeChat for claude fallback, got %q", got)
	}
}

func TestSelectShape_GPT5MiniDoesNotUseResponses(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if got := adapter.selectShape("gpt-5-mini", nil); got != shapeChat {
		t.Errorf("expected shapeChat for gpt-5-mini, got %q", got)
	}
}

func TestConvertCIFToOpenAI_PrependsSystemPrompt(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)
	systemPrompt := "Follow the project instructions."

	payload := adapter.convertCIFToOpenAI(&cif.CanonicalRequest{
		Model:        "grok-4.5",
		SystemPrompt: &systemPrompt,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ping"}}},
		},
	}, newCopilotToolNameMapper(nil))

	messages := payload["messages"].([]map[string]interface{})
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want system and user messages", messages)
	}
	if messages[0]["role"] != "system" || messages[0]["content"] != systemPrompt {
		t.Fatalf("system message = %#v", messages[0])
	}
}

func TestBuildResponsesPayload_FlattensNamedToolChoice(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	payload := adapter.buildResponsesPayload(&cif.CanonicalRequest{
		Model: "grok-4.5",
		Tools: []cif.CIFTool{{Name: "matrix_echo", ParametersSchema: map[string]interface{}{"type": "object"}}},
		ToolChoice: map[string]interface{}{
			"type":         "function",
			"functionName": "matrix_echo",
		},
	}, false)

	choice, ok := payload["tool_choice"].(map[string]interface{})
	if !ok || choice["type"] != "function" || choice["name"] != "matrix_echo" {
		t.Fatalf("tool_choice = %#v, want flat Responses function choice", payload["tool_choice"])
	}
	if payload["parallel_tool_calls"] != true {
		t.Fatalf("parallel_tool_calls = %#v, want true", payload["parallel_tool_calls"])
	}
	if _, nested := choice["function"]; nested {
		t.Fatalf("tool_choice unexpectedly retained Chat Completions nesting: %#v", choice)
	}
}

func TestUnsupportedChatCompletionsModel(t *testing.T) {
	adapter := &CopilotAdapter{}
	for _, tc := range []struct {
		name string
		err  *copilotAPIError
		want bool
	}{
		{
			name: "explicit unsupported API",
			err:  &copilotAPIError{statusCode: http.StatusBadRequest, body: []byte(`{"error":{"code":"unsupported_api_for_model"}}`)},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := adapter.isUnsupportedChatCompletionsModel(tc.err); got != tc.want {
				t.Fatalf("isUnsupportedChatCompletionsModel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCopilotAdapter_ShapeCacheDrivesRouting(t *testing.T) {
	// Verify that a model explicitly listed as responses-only in the cache
	// routes to /responses, and a model listed as chat routes to /chat/completions,
	// regardless of the model name's GPT-5 heuristic.

	cases := []struct {
		name         string
		model        string
		shape        copilotAPIShape
		expectedPath string
		serverResp   string
	}{
		{
			name:         "cache says responses",
			model:        "some-future-model",
			shape:        shapeResponses,
			expectedPath: "/responses",
			serverResp:   `{"id":"resp_cache","model":"some-future-model","status":"completed","output":[{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`,
		},
		{
			name:         "grok cache says responses",
			model:        "grok-4.5",
			shape:        shapeResponses,
			expectedPath: "/responses",
			serverResp:   `{"id":"resp_grok","model":"grok-4.5","status":"completed","output":[{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`,
		},
		{
			name:         "cache says chat",
			model:        "gpt-5.5", // would normally route to /responses by heuristic
			shape:        shapeChat,
			expectedPath: "/chat/completions",
			serverResp: func() string {
				b, _ := json.Marshal(map[string]interface{}{
					"id":    "chatcmpl_cache",
					"model": "gpt-5.5",
					"choices": []map[string]interface{}{{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "ok"},
						"finish_reason": "stop",
					}},
				})
				return string(b)
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.serverResp))
			}))
			defer server.Close()

			provider := NewGitHubCopilotProvider("test", "")
			provider.baseURL = server.URL
			provider.token = "test-token"
			provider.shapeCache = modelShapeCache{tc.model: tc.shape}
			adapter := provider.GetAdapter().(*CopilotAdapter)

			_, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
				Model: tc.model,
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ping"}}},
				},
			})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if capturedPath != tc.expectedPath {
				t.Errorf("expected path %q, got %q", tc.expectedPath, capturedPath)
			}
		})
	}
}
