package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCompatibilityRouteReplaysParallelCallsAndLargeResult(t *testing.T) {
	parallel := findServerCompatibilityScenario(t, "parallel-interleaved-calls")
	large := findServerCompatibilityScenario(t, "large-result")
	var captured []*cif.CanonicalRequest
	registerStubProvider(t, "compat-replay-model", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		captured = append(captured, request)
		return &cif.CanonicalResponse{ID: "replay-final", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	rows := []struct {
		name      string
		messages  []interface{}
		exchanges []testcompat.ToolExchange
	}{
		{name: "parallel", messages: parallelChatHistory(parallel), exchanges: parallel.Exchanges},
		{name: "large-result", messages: singleExchangeChatHistory(large.Exchanges[0]), exchanges: large.Exchanges},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]interface{}{"model": "compat-replay-model", "messages": row.messages})
			resp := postJSON(t, srv.URL+"/v1/chat/completions", string(payload), nil)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			request := captured[len(captured)-1]
			assertCompatibilityExchanges(t, request.Messages, row.exchanges)
			if row.name == "parallel" {
				assistantTurns := 0
				for _, message := range request.Messages {
					if assistant, ok := message.(cif.CIFAssistantMessage); ok {
						toolCalls := 0
						for _, part := range assistant.Content {
							if _, ok := part.(cif.CIFToolCallPart); ok {
								toolCalls++
							}
						}
						if toolCalls == len(parallel.Exchanges) {
							assistantTurns++
						}
					}
				}
				if assistantTurns != 1 {
					t.Fatalf("parallel calls were not preserved in one assistant turn: %#v", request.Messages)
				}
			}
			if row.name == "large-result" {
				result := compatibilityResultForCall(t, request.Messages, large.Exchanges[0].ID)
				if len(result.Content) != testcompat.LargeResultSize || result.Content != large.Exchanges[0].Result {
					t.Fatalf("large result length=%d, want %d", len(result.Content), testcompat.LargeResultSize)
				}
			}
		})
	}
}

func parallelChatHistory(scenario testcompat.Scenario) []interface{} {
	toolCalls := make([]interface{}, 0, len(scenario.Exchanges))
	messages := []interface{}{map[string]interface{}{"role": "user", "content": testcompat.InitialPrompt}}
	for _, exchange := range scenario.Exchanges {
		arguments, _ := json.Marshal(exchange.Arguments)
		toolCalls = append(toolCalls, map[string]interface{}{"id": exchange.ID, "type": "function", "function": map[string]interface{}{"name": exchange.Name, "arguments": string(arguments)}})
	}
	messages = append(messages, map[string]interface{}{"role": "assistant", "content": scenario.Prelude, "tool_calls": toolCalls})
	for _, exchange := range scenario.Exchanges {
		messages = append(messages, map[string]interface{}{"role": "tool", "tool_call_id": exchange.ID, "content": exchange.Result})
	}
	return messages
}

func singleExchangeChatHistory(exchange testcompat.ToolExchange) []interface{} {
	arguments, _ := json.Marshal(exchange.Arguments)
	return []interface{}{
		map[string]interface{}{"role": "user", "content": testcompat.InitialPrompt},
		map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": exchange.ID, "type": "function", "function": map[string]interface{}{"name": exchange.Name, "arguments": string(arguments)}}}},
		map[string]interface{}{"role": "tool", "tool_call_id": exchange.ID, "content": exchange.Result},
	}
}

func compatibilityResultForCall(t *testing.T, messages []cif.CIFMessage, callID string) cif.CIFToolResultPart {
	t.Helper()
	for _, message := range messages {
		user, ok := message.(cif.CIFUserMessage)
		if !ok {
			continue
		}
		for _, part := range user.Content {
			if result, ok := part.(cif.CIFToolResultPart); ok && result.ToolCallID == callID {
				return result
			}
		}
	}
	t.Fatalf("tool result %q not found", callID)
	return cif.CIFToolResultPart{}
}

func findServerCompatibilityScenario(t *testing.T, name string) testcompat.Scenario {
	t.Helper()
	for _, scenario := range testcompat.SemanticScenarios() {
		if scenario.Name == name {
			return scenario
		}
	}
	t.Fatalf("compatibility scenario %q not found", name)
	return testcompat.Scenario{}
}

func TestCompatibilityRouteExecutesSuccessiveToolCycles(t *testing.T) {
	scenario := testcompat.AgenticScenario()
	turn := 0
	registerStubProvider(t, "compat-loop-model", func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		if turn < len(scenario.Exchanges) {
			exchange := scenario.Exchanges[turn]
			turn++
			return &cif.CanonicalResponse{ID: fmt.Sprintf("loop-%d", turn), Model: request.Model, Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name, ToolArguments: exchange.Arguments}}, StopReason: cif.StopReasonToolUse}, nil
		}
		return &cif.CanonicalResponse{ID: "loop-final", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	messages := []interface{}{map[string]interface{}{"role": "user", "content": testcompat.InitialPrompt}}
	for index, exchange := range scenario.Exchanges {
		payload, _ := json.Marshal(map[string]interface{}{"model": "compat-loop-model", "messages": messages, "tools": []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "Read", "parameters": map[string]interface{}{"type": "object"}}}}})
		resp := postJSON(t, srv.URL+"/v1/chat/completions", string(payload), nil)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("cycle %d status %d: %s", index, resp.StatusCode, body)
		}
		var result struct {
			Choices []struct {
				Message      map[string]interface{} `json:"message"`
				FinishReason string                 `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Choices) != 1 || result.Choices[0].FinishReason != "tool_calls" {
			t.Fatalf("cycle %d did not request tool: %s", index, body)
		}
		messages = append(messages, result.Choices[0].Message, map[string]interface{}{"role": "tool", "tool_call_id": exchange.ID, "content": exchange.Result})
	}
	payload, _ := json.Marshal(map[string]interface{}{"model": "compat-loop-model", "messages": messages})
	resp := postJSON(t, srv.URL+"/v1/chat/completions", string(payload), nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("final status %d: %s", resp.StatusCode, body)
	}
	var final struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string        `json:"content"`
				ToolCalls []interface{} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Choices) != 1 || final.Choices[0].FinishReason != "stop" || final.Choices[0].Message.Content != testcompat.FinalAnswer || len(final.Choices[0].Message.ToolCalls) != 0 {
		t.Fatalf("unexpected final response: %s", body)
	}
}
