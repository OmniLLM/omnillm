package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMessagesCountTokensRejectsInvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages/count_tokens",
		`{"model":`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestMessagesCountTokensReturnsEstimatedInputTokens(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages/count_tokens",
		`{
			"model":"claude-3-5-sonnet",
			"system":"Be terse.",
			"messages":[
				{"role":"user","content":"Hello there"},
				{"role":"assistant","content":[{"type":"text","text":"Hi!"}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"Sunny"}]}
			],
			"tools":[
				{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}
			]
		}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.InputTokens <= 0 {
		t.Fatalf("expected positive token estimate, got %#v", payload)
	}
}

func TestMessagesCountTokensRejectsMalformedAnthropicPayload(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages/count_tokens",
		`{
			"model":"claude-3-5-sonnet",
			"messages":[
				{"role":"user","content":[{"type":"image"}]}
			]
		}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}
