package server

import (
	"context"
	"encoding/json"
	"net/http"
	"omnillm/internal/cif"
	"testing"
)

func TestRequestedModelsChatAndToolUse(t *testing.T) {
	models := []string{
		"gpt-5-mini",
		"claude-haiku-4.5",
		"claude-sonnet-4.6",
		"gpt-5.4",
		"qwen3.6-plus",
		"deepseek-v4-flash",
	}

	for _, modelID := range models {
		t.Run(modelID, func(t *testing.T) {
			registerStubProvider(
				t,
				modelID,
				func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
					if req.Model != modelID {
						t.Fatalf("expected model %q, got %q", modelID, req.Model)
					}
					if len(req.Tools) != 1 {
						t.Fatalf("expected one tool definition for %s, got %#v", modelID, req.Tools)
					}
					if req.Tools[0].Name != "get_weather" {
						t.Fatalf("expected get_weather tool for %s, got %#v", modelID, req.Tools[0])
					}

					return &cif.CanonicalResponse{
						ID:    "resp_tool_" + modelID,
						Model: req.Model,
						Content: []cif.CIFContentPart{
							cif.CIFToolCallPart{
								Type:          "tool_call",
								ToolCallID:    "call_weather",
								ToolName:      "get_weather",
								ToolArguments: map[string]interface{}{"location": "Shanghai"},
							},
						},
						StopReason: cif.StopReasonToolUse,
					}, nil
				},
				nil,
			)

			srv := newTestServer(t)
			defer srv.Close()

			assertRequestedModelChatToolUse(t, srv.URL, modelID)
			assertRequestedModelAnthropicToolUse(t, srv.URL, modelID)
			assertRequestedModelResponsesToolUse(t, srv.URL, modelID)
		})
	}
}

func assertRequestedModelChatToolUse(t *testing.T, serverURL, modelID string) {
	t.Helper()

	resp := postJSON(
		t,
		serverURL+"/v1/chat/completions",
		`{"model":"`+modelID+`","messages":[{"role":"user","content":"Check the weather"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}}]}`,
		nil,
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat completions: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
			Message      struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("chat completions: invalid JSON for %s: %v", modelID, err)
	}
	if len(payload.Choices) != 1 || payload.Choices[0].FinishReason == nil || *payload.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("chat completions: unexpected tool response for %s: %#v", modelID, payload)
	}
	toolCalls := payload.Choices[0].Message.ToolCalls
	if len(toolCalls) != 1 || toolCalls[0].ID != "call_weather" || toolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("chat completions: unexpected tool call for %s: %#v", modelID, toolCalls)
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("chat completions: invalid tool arguments for %s: %v", modelID, err)
	}
	if args["location"] != "Shanghai" {
		t.Fatalf("chat completions: unexpected tool arguments for %s: %#v", modelID, args)
	}
}

func assertRequestedModelAnthropicToolUse(t *testing.T, serverURL, modelID string) {
	t.Helper()

	resp := postJSON(
		t,
		serverURL+"/v1/messages",
		`{"model":"`+modelID+`","max_tokens":100,"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],"messages":[{"role":"user","content":"Check the weather"}]}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anthropic messages: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		StopReason *string `json:"stop_reason"`
		Content    []struct {
			Type  string                 `json:"type"`
			ID    string                 `json:"id"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("anthropic messages: invalid JSON for %s: %v", modelID, err)
	}
	if payload.StopReason == nil || *payload.StopReason != "tool_use" {
		t.Fatalf("anthropic messages: unexpected stop reason for %s: %#v", modelID, payload.StopReason)
	}
	if len(payload.Content) != 1 || payload.Content[0].Type != "tool_use" || payload.Content[0].ID != "toolu_call_weather" || payload.Content[0].Name != "get_weather" {
		t.Fatalf("anthropic messages: unexpected tool payload for %s: %#v", modelID, payload.Content)
	}
	if payload.Content[0].Input["location"] != "Shanghai" {
		t.Fatalf("anthropic messages: unexpected tool input for %s: %#v", modelID, payload.Content[0].Input)
	}
}

func assertRequestedModelResponsesToolUse(t *testing.T, serverURL, modelID string) {
	t.Helper()

	resp := postJSON(
		t,
		serverURL+"/v1/responses",
		`{"model":"`+modelID+`","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Check the weather"}]}],"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}]}`,
		nil,
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Output []struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("responses: invalid JSON for %s: %v", modelID, err)
	}
	if len(payload.Output) != 1 || payload.Output[0].Type != "function_call" || payload.Output[0].CallID != "call_weather" || payload.Output[0].Name != "get_weather" {
		t.Fatalf("responses: unexpected function call output for %s: %#v", modelID, payload.Output)
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(payload.Output[0].Arguments), &args); err != nil {
		t.Fatalf("responses: invalid tool arguments for %s: %v", modelID, err)
	}
	if args["location"] != "Shanghai" {
		t.Fatalf("responses: unexpected tool arguments for %s: %#v", modelID, args)
	}
}
