package ingestion

import (
	"encoding/json"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCompatibilityParallelCallsIngestion(t *testing.T) {
	scenario := findCompatibilityIngestionScenario(t, "parallel-interleaved-calls")
	requests := []struct {
		name  string
		raw   json.RawMessage
		parse func(json.RawMessage) (*cif.CanonicalRequest, error)
	}{
		{"chat", parallelChatRequest(scenario), ParseOpenAIChatCompletions},
		{"anthropic", parallelAnthropicRequest(scenario), ParseAnthropicMessages},
		{"responses", parallelResponsesRequest(scenario), ParseResponsesPayload},
	}
	for _, row := range requests {
		t.Run(row.name, func(t *testing.T) {
			request, err := row.parse(row.raw)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			results := 0
			for _, message := range request.Messages {
				switch typed := message.(type) {
				case cif.CIFAssistantMessage:
					for _, part := range typed.Content {
						if _, ok := part.(cif.CIFToolCallPart); ok {
							calls++
						}
					}
				case cif.CIFUserMessage:
					for _, part := range typed.Content {
						if _, ok := part.(cif.CIFToolResultPart); ok {
							results++
						}
					}
				}
			}
			if calls != 2 || results != 2 {
				t.Fatalf("calls=%d results=%d", calls, results)
			}
		})
	}
}

func findCompatibilityIngestionScenario(t *testing.T, name string) testcompat.Scenario {
	t.Helper()
	for _, scenario := range testcompat.SemanticScenarios() {
		if scenario.Name == name {
			return scenario
		}
	}
	t.Fatalf("missing scenario %q", name)
	return testcompat.Scenario{}
}

func parallelChatRequest(s testcompat.Scenario) json.RawMessage {
	calls := []interface{}{}
	results := []interface{}{map[string]interface{}{"role": "user", "content": testcompat.InitialPrompt}}
	for _, exchange := range s.Exchanges {
		args, _ := json.Marshal(exchange.Arguments)
		calls = append(calls, map[string]interface{}{"id": exchange.ID, "type": "function", "function": map[string]interface{}{"name": exchange.Name, "arguments": string(args)}})
	}
	results = append(results, map[string]interface{}{"role": "assistant", "content": s.Prelude, "tool_calls": calls})
	for _, exchange := range s.Exchanges {
		results = append(results, map[string]interface{}{"role": "tool", "tool_call_id": exchange.ID, "content": exchange.Result})
	}
	return compatibilityMarshal(results, "messages")
}

func parallelAnthropicRequest(s testcompat.Scenario) json.RawMessage {
	calls := []interface{}{map[string]interface{}{"type": "text", "text": s.Prelude}}
	results := []interface{}{}
	for _, exchange := range s.Exchanges {
		calls = append(calls, map[string]interface{}{"type": "tool_use", "id": exchange.ID, "name": exchange.Name, "input": exchange.Arguments})
		results = append(results, map[string]interface{}{"type": "tool_result", "tool_use_id": exchange.ID, "content": exchange.Result})
	}
	messages := []interface{}{map[string]interface{}{"role": "user", "content": testcompat.InitialPrompt}, map[string]interface{}{"role": "assistant", "content": calls}, map[string]interface{}{"role": "user", "content": results}}
	payload := map[string]interface{}{"model": testcompat.Model, "max_tokens": 64, "messages": messages}
	data, _ := json.Marshal(payload)
	return data
}

func parallelResponsesRequest(s testcompat.Scenario) json.RawMessage {
	input := []interface{}{map[string]interface{}{"type": "message", "role": "user", "content": testcompat.InitialPrompt}, map[string]interface{}{"type": "message", "role": "assistant", "content": s.Prelude}}
	for _, exchange := range s.Exchanges {
		args, _ := json.Marshal(exchange.Arguments)
		input = append(input, map[string]interface{}{"type": "function_call", "call_id": exchange.ID, "name": exchange.Name, "arguments": string(args)})
	}
	for _, exchange := range s.Exchanges {
		input = append(input, map[string]interface{}{"type": "function_call_output", "call_id": exchange.ID, "output": exchange.Result})
	}
	payload := map[string]interface{}{"model": testcompat.Model, "input": input}
	data, _ := json.Marshal(payload)
	return data
}

func compatibilityMarshal(messages []interface{}, key string) json.RawMessage {
	payload := map[string]interface{}{"model": testcompat.Model, key: messages}
	data, _ := json.Marshal(payload)
	return data
}
