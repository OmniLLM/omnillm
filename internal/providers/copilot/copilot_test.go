package copilot

import (
	"context"
	"encoding/json"
	"fmt"
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

	provider := NewGitHubCopilotProvider("github-copilot-test")
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

	provider := NewGitHubCopilotProvider("github-copilot-test")
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

	provider := NewGitHubCopilotProvider("github-copilot-test")
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


func TestCopilotAdapterExecute_ConvertsResponsesStyleHistoryToChatCompletions(t *testing.T) {
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}

		messages, ok := capturedPayload["messages"].([]interface{})
		if !ok {
			t.Fatalf("expected messages array, got %#v", capturedPayload["messages"])
		}
		if len(messages) != 3 {
			t.Fatalf("expected 3 chat-completions messages, got %d: %#v", len(messages), messages)
		}

		assistant, ok := messages[1].(map[string]interface{})
		if !ok || assistant["role"] != "assistant" {
			t.Fatalf("expected assistant message at index 1, got %#v", messages[1])
		}
		toolCalls, ok := assistant["tool_calls"].([]interface{})
		if !ok || len(toolCalls) != 1 {
			t.Fatalf("expected one assistant tool_call, got %#v", assistant["tool_calls"])
		}

		toolMsg, ok := messages[2].(map[string]interface{})
		if !ok || toolMsg["role"] != "tool" {
			t.Fatalf("expected tool message at index 2, got %#v", messages[2])
		}
		if toolMsg["tool_call_id"] != "call_123" || toolMsg["content"] != "README summary" {
			t.Fatalf("unexpected tool message payload: %#v", toolMsg)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "chatcmpl_responses_history",
			"model": "gpt-5-mini",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "done",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	_, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "gpt-5-mini",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "explain codebase"}}},
			cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Plan updated"},
				cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_123", ToolName: "Glob", ToolArguments: map[string]interface{}{"pattern": "src/**"}},
			}},
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
				cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call_123", ToolName: "Glob", Content: "src/a.ts"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}

func TestCopilotAdapterExecute_AliasesLongToolNamesForChatCompletions(t *testing.T) {
	var capturedPayload map[string]interface{}
	var upstreamToolName string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}

		tools, ok := capturedPayload["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Fatalf("unexpected tools payload: %#v", capturedPayload["tools"])
		}
		tool, ok := tools[0].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected tool payload: %#v", tools[0])
		}
		function, ok := tool["function"].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected function payload: %#v", tool["function"])
		}
		upstreamToolName, _ = function["name"].(string)
		if len(upstreamToolName) > copilotMaxToolNameLength {
			t.Fatalf("expected aliased tool name to fit Copilot limit, got %d chars (%q)", len(upstreamToolName), upstreamToolName)
		}

		toolChoice, ok := capturedPayload["tool_choice"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected object tool_choice, got %#v", capturedPayload["tool_choice"])
		}
		choiceFunction, ok := toolChoice["function"].(map[string]interface{})
		if !ok || choiceFunction["name"] != upstreamToolName {
			t.Fatalf("expected aliased tool_choice name, got %#v", toolChoice)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "chatcmpl_long_tool",
			"model": "claude-haiku-4.5",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_long_tool",
								"type": "function",
								"function": map[string]interface{}{
									"name":      upstreamToolName,
									"arguments": `{"query":"ping"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	originalToolName := "mcp__extremely_long_server_name_that_keeps_going__tool_name_that_is_also_long"
	response, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Use the MCP tool."},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name: originalToolName,
				ParametersSchema: map[string]interface{}{
					"type": "object",
				},
			},
		},
		ToolChoice: map[string]interface{}{
			"type":         "function",
			"functionName": originalToolName,
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if upstreamToolName == "" {
		t.Fatal("expected upstream tool name to be captured")
	}
	if upstreamToolName == originalToolName {
		t.Fatalf("expected long tool name to be aliased before upstream request, got %q", upstreamToolName)
	}

	if len(response.Content) != 1 {
		t.Fatalf("expected one content part, got %d", len(response.Content))
	}
	toolCall, ok := response.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected tool call response, got %#v", response.Content[0])
	}
	if toolCall.ToolName != originalToolName {
		t.Fatalf("expected tool name to be restored to original %q, got %q", originalToolName, toolCall.ToolName)
	}
}

func TestCopilotAdapterExecuteStream_RestoresAliasedToolNamesForChatCompletions(t *testing.T) {
	originalToolName := "mcp__extremely_long_server_name_that_keeps_going__tool_name_that_is_also_long"
	var upstreamToolName string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}

		tools, ok := payload["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Fatalf("unexpected tools payload: %#v", payload["tools"])
		}
		tool, ok := tools[0].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected tool payload: %#v", tools[0])
		}
		function, ok := tool["function"].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected function payload: %#v", tool["function"])
		}
		upstreamToolName, _ = function["name"].(string)

		w.Header().Set("Content-Type", "text/event-stream")
		sseBody := fmt.Sprintf(strings.TrimSpace(`
data: {"id":"chatcmpl_stream_long_tool","model":"claude-haiku-4.5","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl_stream_long_tool","model":"claude-haiku-4.5","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_stream_long_tool","type":"function","function":{"name":"%s"}}]}}]}

data: {"id":"chatcmpl_stream_long_tool","model":"claude-haiku-4.5","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":\"ping\"}"}}]}}]}

data: {"id":"chatcmpl_stream_long_tool","model":"claude-haiku-4.5","choices":[{"index":0,"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":3}}
`), upstreamToolName)
		_, _ = w.Write([]byte(sseBody + "\n\n"))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("github-copilot-test")
	provider.baseURL = server.URL
	provider.token = "test-token"
	adapter := provider.GetAdapter().(*CopilotAdapter)

	eventCh, err := adapter.ExecuteStream(context.Background(), &cif.CanonicalRequest{
		Model: "claude-haiku-4.5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Use the MCP tool."},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name: originalToolName,
				ParametersSchema: map[string]interface{}{
					"type": "object",
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}

	var restoredToolName string
	for event := range eventCh {
		contentDelta, ok := event.(cif.CIFContentDelta)
		if !ok || contentDelta.ContentBlock == nil {
			continue
		}
		toolCall, ok := contentDelta.ContentBlock.(cif.CIFToolCallPart)
		if !ok {
			continue
		}
		restoredToolName = toolCall.ToolName
		break
	}

	if upstreamToolName == "" {
		t.Fatal("expected upstream tool name to be captured")
	}
	if upstreamToolName == originalToolName {
		t.Fatalf("expected long tool name to be aliased before upstream request, got %q", upstreamToolName)
	}
	if restoredToolName != originalToolName {
		t.Fatalf("expected streamed tool name to be restored to %q, got %q", originalToolName, restoredToolName)
	}
}

