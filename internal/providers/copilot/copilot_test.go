package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"strings"
	"testing"
	"time"

	ghservice "omnillm/internal/services/github"
)

func TestCopilotAdapterExecute_TranslatesSpecificToolChoiceForChatCompletions(t *testing.T) {
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "chatcmpl_tool_choice",
			"model": "claude-haiku-4.5",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "pong",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	_, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Use the weather tool."},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name: "get_weather",
				ParametersSchema: map[string]interface{}{
					"type": "object",
				},
			},
		},
		ToolChoice: map[string]interface{}{
			"type":         "function",
			"functionName": "get_weather",
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	toolChoice, ok := capturedPayload["tool_choice"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object tool_choice, got %#v", capturedPayload["tool_choice"])
	}
	if toolChoice["type"] != "function" {
		t.Fatalf("unexpected tool_choice type: %#v", toolChoice)
	}
	function, ok := toolChoice["function"].(map[string]interface{})
	if !ok || function["name"] != "get_weather" {
		t.Fatalf("unexpected tool_choice function payload: %#v", toolChoice["function"])
	}
}

func TestCopilotAdapterExecute_StripsThinkingPartsFromPayload(t *testing.T) {
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "chatcmpl_strip_thinking",
			"model": "claude-haiku-4.5",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	_, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
					cif.CIFThinkingPart{Type: "thinking", Thinking: "internal chain of thought"},
				},
			},
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFThinkingPart{Type: "thinking", Thinking: "more hidden reasoning"},
					cif.CIFTextPart{Type: "text", Text: "visible reply"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	messages, ok := capturedPayload["messages"].([]interface{})
	if !ok {
		t.Fatalf("expected messages array, got %#v", capturedPayload["messages"])
	}
	serialized, _ := json.Marshal(messages)
	if strings.Contains(string(serialized), "internal chain of thought") || strings.Contains(string(serialized), "more hidden reasoning") {
		t.Fatalf("expected thinking content to be stripped, got payload: %s", string(serialized))
	}
}

func TestCopilotAdapterExecuteStream_RefreshesTokenAndRetriesOnUnauthorized(t *testing.T) {
	var requestCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		requestCalls++
		switch r.Header.Get("Authorization") {
		case "Bearer stale-token":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "IDE token expired: unauthorized: token expired",
					"type":    "authentication_error",
				},
			})
		case "Bearer fresh-token":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(strings.TrimSpace(`
data: {"id":"chatcmpl_refresh","model":"claude-haiku-4.5","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl_refresh","model":"claude-haiku-4.5","choices":[{"index":0,"delta":{"content":"pong"}}]}

data: {"id":"chatcmpl_refresh","model":"claude-haiku-4.5","choices":[{"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":1}}
	`) + "\n\n"))
		default:
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.githubToken = "github-token"
	provider.token = "stale-token"
	provider.expiresAt = time.Now().Add(30 * time.Minute).Unix()

	var refreshCalls int
	provider.tokenFetcher = func(githubToken string) (*ghservice.CopilotTokenResponse, error) {
		refreshCalls++
		if githubToken != "github-token" {
			t.Fatalf("expected github token to be passed to refresh, got %q", githubToken)
		}
		return &ghservice.CopilotTokenResponse{
			Token:     "fresh-token",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}, nil
	}

	adapter := provider.GetAdapter().(*CopilotAdapter)
	eventCh, err := adapter.ExecuteStream(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}

	var sawTextDelta bool
	var sawEnd bool
	for event := range eventCh {
		switch e := event.(type) {
		case cif.CIFContentDelta:
			if delta, ok := e.Delta.(cif.TextDelta); ok && delta.Text == "pong" {
				sawTextDelta = true
			}
		case cif.CIFStreamEnd:
			sawEnd = true
		}
	}

	if !sawTextDelta {
		t.Fatal("expected a streamed text delta after token refresh")
	}
	if !sawEnd {
		t.Fatal("expected stream end after token refresh")
	}
	if refreshCalls != 1 {
		t.Fatalf("expected one token refresh after 401, got %d", refreshCalls)
	}
	if requestCalls != 2 {
		t.Fatalf("expected two upstream requests (401 then retry), got %d", requestCalls)
	}
}

func TestCopilotAdapterExecuteStream_DisableAuthRetryMakesSingleAttempt(t *testing.T) {
	var requestCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		requestCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "IDE token expired: unauthorized: token expired",
				"type":    "authentication_error",
			},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.githubToken = "github-token"
	provider.token = "stale-token"
	provider.expiresAt = time.Now().Add(30 * time.Minute).Unix()

	var refreshCalls int
	provider.tokenFetcher = func(githubToken string) (*ghservice.CopilotTokenResponse, error) {
		refreshCalls++
		return &ghservice.CopilotTokenResponse{
			Token:     "fresh-token",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}, nil
	}

	adapter := provider.GetAdapter().(*CopilotAdapter)
	forceChatCompletions := true
	disableAuthRetry := true
	_, err := adapter.ExecuteStream(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
				},
			},
		},
		Stream: true,
		Extensions: &cif.Extensions{
			ForceChatCompletions: &forceChatCompletions,
			DisableAuthRetry:     &disableAuthRetry,
		},
	})
	if err == nil {
		t.Fatal("expected ExecuteStream to return upstream auth error without retry")
	}

	if refreshCalls != 0 {
		t.Fatalf("expected zero token refresh attempts, got %d", refreshCalls)
	}
	if requestCalls != 1 {
		t.Fatalf("expected one upstream request, got %d", requestCalls)
	}
}

func TestCopilotAdapterExecute_GPT5FamilyRoutesToResponsesEndpoint(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&capturedPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_gpt5","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	resp, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "gpt-5.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if capturedPath != "/responses" {
		t.Fatalf("expected /responses for gpt-5 family, got %q", capturedPath)
	}
	if model, _ := capturedPayload["model"].(string); model != "gpt-5.5" {
		t.Fatalf("expected upstream model gpt-5.5, got %#v", capturedPayload["model"])
	}
	if resp == nil || resp.ID != "resp_gpt5" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestCopilotAdapterExecute_RoutesResponsesUsingRemappedModel(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_remap","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	resp, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "  gpt-5.5  ",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ping"}}},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if capturedPath != "/responses" {
		t.Fatalf("expected /responses for remapped gpt-5.5, got %q", capturedPath)
	}
	if resp == nil || resp.ID != "resp_remap" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestCopilotAdapterExecute_NonGPT5StaysOnChatCompletions(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "chatcmpl_nongpt5",
			"model": "claude-haiku-4.5",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if _, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if capturedPath != "/chat/completions" {
		t.Fatalf("expected /chat/completions for non-gpt-5 model, got %q", capturedPath)
	}
}

func TestCopilotAdapterExecute_FallsBackToResponsesOnUnsupportedAPIError(t *testing.T) {
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": `model "future-responses-only" is not accessible via the /chat/completions endpoint`,
					"code":    "unsupported_api_for_model",
				},
			})
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_fallback","model":"future-responses-only","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	resp, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		// Note: model name does NOT match the gpt-5 prefix — exercises the
		// upstream-error fallback path rather than shouldUseResponsesAPI.
		Model: "future-responses-only",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "ping"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp == nil || resp.ID != "resp_fallback" {
		t.Fatalf("unexpected fallback response: %#v", resp)
	}
	if len(paths) != 2 || paths[0] != "/chat/completions" || paths[1] != "/responses" {
		t.Fatalf("expected chat→responses fallback, got %v", paths)
	}
}
