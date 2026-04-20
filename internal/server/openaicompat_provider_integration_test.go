package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/registry"
	"strings"
	"sync"
	"testing"
	"time"

	providertypes "omnillm/internal/providers/types"
)

type capturedOpenAICompatRequest struct {
	Authorization string
	Accept        string
	Payload       map[string]interface{}
}

type fakeOpenAICompatUpstream struct {
	server *httptest.Server
	model  string

	mu           sync.Mutex
	chatRequests []capturedOpenAICompatRequest
}

func newFakeOpenAICompatUpstream(t *testing.T, model string) *fakeOpenAICompatUpstream {
	t.Helper()

	upstream := &fakeOpenAICompatUpstream{model: model}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": model},
				},
			})
			return

		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, fmt.Sprintf("decode payload: %v", err), http.StatusBadRequest)
				return
			}

			upstream.mu.Lock()
			upstream.chatRequests = append(upstream.chatRequests, capturedOpenAICompatRequest{
				Authorization: r.Header.Get("Authorization"),
				Accept:        r.Header.Get("Accept"),
				Payload:       payload,
			})
			upstream.mu.Unlock()

			if stream, _ := payload["stream"].(bool); stream {
				http.Error(w, "unexpected upstream streaming request", http.StatusTeapot)
				return
			}
			if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
				http.Error(w, "unexpected upstream SSE accept header", http.StatusTeapot)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if requestContainsToolResult(payload) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"id":    "chat_final",
					"model": model,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": "This codebase exposes a proxy that normalizes requests into CIF and adapts multiple upstream APIs.",
							},
							"finish_reason": "stop",
						},
					},
					"usage": map[string]interface{}{
						"prompt_tokens":     61,
						"completion_tokens": 18,
						"total_tokens":      79,
					},
				})
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "chat_tool",
				"model": model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant",
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call_readme",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "Read",
										"arguments": `{"file_path":"README.md"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     37,
					"completion_tokens": 9,
					"total_tokens":      46,
				},
			})
			return
		}

		http.NotFound(w, r)
	}))

	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *fakeOpenAICompatUpstream) baseURL() string {
	return u.server.URL + "/v1"
}

func (u *fakeOpenAICompatUpstream) lastChatRequest(t *testing.T) capturedOpenAICompatRequest {
	t.Helper()

	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.chatRequests) == 0 {
		t.Fatal("expected at least one captured openai-compatible chat request")
	}
	return u.chatRequests[len(u.chatRequests)-1]
}

func (u *fakeOpenAICompatUpstream) chatRequestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.chatRequests)
}

func registerOpenAICompatibleProvider(t *testing.T, upstreamBaseURL, apiKey, modelID string) string {
	t.Helper()

	instanceID := fmt.Sprintf("openai-compatible-test-%d", time.Now().UnixNano())
	provider := openaicompatprovider.NewProvider(instanceID, "OpenAI-Compatible Test")
	if err := provider.SetupAuth(&providertypes.AuthOptions{
		Endpoint:            upstreamBaseURL,
		APIKey:              apiKey,
		Models:              fmt.Sprintf("[\"%s\"]", modelID),
		AllowLocalEndpoints: true,
	}); err != nil {
		t.Fatalf("setup openai-compatible provider: %v", err)
	}

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register openai-compatible provider: %v", err)
	}
	if _, err := reg.AddActive(instanceID); err != nil {
		t.Fatalf("activate openai-compatible provider: %v", err)
	}

	t.Cleanup(func() {
		_ = reg.Remove(instanceID)
	})

	return instanceID
}

func TestOpenAICompatibleAnthropicStreamingIsBufferedDownstream(t *testing.T) {
	modelID := fmt.Sprintf("compat-buffered-%d", time.Now().UnixNano())
	upstream := newFakeOpenAICompatUpstream(t, modelID)
	apiKey := fmt.Sprintf("sk-compat-%d", time.Now().UnixNano())
	registerOpenAICompatibleProvider(t, upstream.baseURL(), apiKey, modelID)

	backend := newTestServer(t)
	defer backend.Close()

	firstResp := postJSON(
		t,
		backend.URL+"/v1/messages",
		fmt.Sprintf(`{"model":"%s","stream":true,"max_tokens":256,"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],"messages":[{"role":"user","content":"Explain codebase in detail"}]}`, modelID),
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	firstBody := readBody(t, firstResp)
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected first turn 200, got %d: %s", firstResp.StatusCode, firstBody)
	}
	if !strings.Contains(firstResp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected Anthropic SSE response, got %q", firstResp.Header.Get("Content-Type"))
	}

	firstEvents := parseSSEBody(t, firstBody)
	var firstToolUse bool
	var firstStopReason string
	var firstArgs strings.Builder
	for _, evt := range firstEvents {
		if contentBlock, ok := evt.Data["content_block"].(map[string]interface{}); ok {
			if contentBlock["type"] == "tool_use" && contentBlock["name"] == "Read" && contentBlock["id"] == "call_readme" {
				firstToolUse = true
			}
		}
		if delta, ok := evt.Data["delta"].(map[string]interface{}); ok {
			switch delta["type"] {
			case "input_json_delta":
				if text, ok := delta["partial_json"].(string); ok {
					firstArgs.WriteString(text)
				}
			case "text_delta":
				t.Fatalf("did not expect text deltas in tool-use turn: %s", firstBody)
			}
			if evt.Event == "message_delta" {
				if reason, ok := delta["stop_reason"].(string); ok {
					firstStopReason = reason
				}
			}
		}
	}
	if !firstToolUse {
		t.Fatalf("expected buffered Anthropic stream to include tool_use block: %s", firstBody)
	}
	if firstStopReason != "tool_use" {
		t.Fatalf("expected first turn stop_reason=tool_use, got %q", firstStopReason)
	}
	if firstArgs.String() != `{"file_path":"README.md"}` {
		t.Fatalf("unexpected buffered tool arguments: %q", firstArgs.String())
	}
	if upstream.chatRequestCount() != 1 {
		t.Fatalf("expected one upstream request after first turn, got %d", upstream.chatRequestCount())
	}

	firstUpstreamReq := upstream.lastChatRequest(t)
	if firstUpstreamReq.Authorization != "Bearer "+apiKey {
		t.Fatalf("expected bearer auth header, got %q", firstUpstreamReq.Authorization)
	}
	if firstUpstreamReq.Accept != "application/json" {
		t.Fatalf("expected buffered upstream Accept header, got %q", firstUpstreamReq.Accept)
	}
	if stream, _ := firstUpstreamReq.Payload["stream"].(bool); stream {
		t.Fatalf("expected buffered upstream request to disable stream, got payload %#v", firstUpstreamReq.Payload)
	}

	secondResp := postJSON(
		t,
		backend.URL+"/v1/messages",
		fmt.Sprintf(`{"model":"%s","stream":true,"max_tokens":512,"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],"messages":[{"role":"user","content":"Explain codebase in detail"},{"role":"assistant","content":[{"type":"tool_use","id":"call_readme","name":"Read","input":{"file_path":"README.md"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_readme","content":"README.md says this proxy normalizes requests into CIF before routing upstream."}]}]}`, modelID),
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	secondBody := readBody(t, secondResp)
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("expected second turn 200, got %d: %s", secondResp.StatusCode, secondBody)
	}
	if !strings.Contains(secondResp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected Anthropic SSE response on second turn, got %q", secondResp.Header.Get("Content-Type"))
	}

	secondEvents := parseSSEBody(t, secondBody)
	var finalText strings.Builder
	var secondStopReason string
	var sawSecondToolUse bool
	for _, evt := range secondEvents {
		if contentBlock, ok := evt.Data["content_block"].(map[string]interface{}); ok && contentBlock["type"] == "tool_use" {
			sawSecondToolUse = true
		}
		if delta, ok := evt.Data["delta"].(map[string]interface{}); ok {
			if delta["type"] == "text_delta" {
				if text, ok := delta["text"].(string); ok {
					finalText.WriteString(text)
				}
			}
			if evt.Event == "message_delta" {
				if reason, ok := delta["stop_reason"].(string); ok {
					secondStopReason = reason
				}
			}
		}
	}
	if sawSecondToolUse {
		t.Fatalf("did not expect second tool_use block in buffered final turn: %s", secondBody)
	}
	if secondStopReason != "end_turn" {
		t.Fatalf("expected second turn stop_reason=end_turn, got %q", secondStopReason)
	}
	wantFinalText := "This codebase exposes a proxy that normalizes requests into CIF and adapts multiple upstream APIs."
	if finalText.String() != wantFinalText {
		t.Fatalf("unexpected buffered final answer:\nwant: %q\ngot:  %q", wantFinalText, finalText.String())
	}
	if upstream.chatRequestCount() != 2 {
		t.Fatalf("expected exactly two buffered upstream requests across the tool loop, got %d", upstream.chatRequestCount())
	}

	secondUpstreamReq := upstream.lastChatRequest(t)
	if secondUpstreamReq.Accept != "application/json" {
		t.Fatalf("expected buffered upstream Accept header on second turn, got %q", secondUpstreamReq.Accept)
	}
	if stream, _ := secondUpstreamReq.Payload["stream"].(bool); stream {
		t.Fatalf("expected second buffered upstream request to disable stream, got payload %#v", secondUpstreamReq.Payload)
	}
}
