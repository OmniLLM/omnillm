package shared

import (
	"encoding/json"
	"reflect"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/testcompat"
)

func TestAntigravityHistoryPreservesFiveClientToolExchanges(t *testing.T) {
	for _, fixture := range testcompat.ClientCacheFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			parse := ingestion.ParseAnthropicMessages
			switch fixture.Endpoint {
			case "/v1/chat/completions":
				parse = ingestion.ParseOpenAIChatCompletions
			case "/v1/responses":
				parse = ingestion.ParseResponsesPayload
			}
			request, err := parse(fixture.Request)
			if err != nil {
				t.Fatal(err)
			}
			// Responses clients can omit result names; only the call ID is authoritative.
			for _, message := range request.Messages {
				user, ok := message.(cif.CIFUserMessage)
				if !ok {
					continue
				}
				for i, part := range user.Content {
					if result, ok := part.(cif.CIFToolResultPart); ok {
						result.ToolName = ""
						user.Content[i] = result
					}
				}
			}
			before, err := json.Marshal(request.Messages)
			if err != nil {
				t.Fatal(err)
			}
			contents := CIFMessagesToGemini(request.Messages)
			calls := []map[string]interface{}{}
			results := []map[string]interface{}{}
			for _, content := range contents {
				for _, part := range content["parts"].([]map[string]interface{}) {
					if call, ok := part["functionCall"].(map[string]interface{}); ok {
						calls = append(calls, call)
					}
					if result, ok := part["functionResponse"].(map[string]interface{}); ok {
						results = append(results, result)
					}
				}
			}
			if len(calls) != 5 || len(results) != 5 {
				t.Fatalf("calls=%d results=%d, want five of each", len(calls), len(results))
			}
			for i, exchange := range fixture.Exchanges {
				if calls[i]["id"] != exchange.ID || results[i]["id"] != exchange.ID {
					t.Errorf("exchange %d lost call/result identity", i)
				}
				if calls[i]["name"] != exchange.Name || results[i]["name"] != exchange.Name {
					t.Errorf("exchange %d lost call/result name", i)
				}
				if !reflect.DeepEqual(calls[i]["args"], exchange.Arguments) {
					t.Errorf("exchange %d changed arguments", i)
				}
				if results[i]["response"].(map[string]interface{})["output"] != exchange.Result {
					t.Errorf("exchange %d changed result", i)
				}
			}
			after, err := json.Marshal(request.Messages)
			if err != nil || string(before) != string(after) {
				t.Fatal("canonical history mutated")
			}
		})
	}
}
