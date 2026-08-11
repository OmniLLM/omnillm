package serialization

import (
	"encoding/json"
	"strings"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCompatibilityFixtureNonStreamSerializationMatrix(t *testing.T) {
	for _, scenario := range testcompat.SemanticScenarios() {
		if scenario.Name == "cancellation" {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			response := testcompat.Response(scenario)
			chat, err := SerializeToOpenAI(response)
			if err != nil {
				t.Fatalf("chat serialization: %v", err)
			}
			anthropic, err := SerializeToAnthropic(response)
			if err != nil {
				t.Fatalf("anthropic serialization: %v", err)
			}
			responses, err := SerializeToResponses(response)
			if err != nil {
				t.Fatalf("responses serialization: %v", err)
			}
			for name, payload := range map[string]interface{}{"chat": chat, "anthropic": anthropic, "responses": responses} {
				data, marshalErr := json.Marshal(payload)
				if marshalErr != nil || !json.Valid(data) {
					t.Fatalf("%s invalid JSON: %v", name, marshalErr)
				}
				if !strings.Contains(string(data), testcompat.Model) {
					t.Errorf("%s omitted model", name)
				}
				for _, exchange := range scenario.Exchanges {
					if !strings.Contains(string(data), exchange.ID) || !strings.Contains(string(data), exchange.Name) {
						t.Errorf("%s omitted tool identity %s/%s", name, exchange.ID, exchange.Name)
					}
				}
			}
		})
	}
}

func TestCompatibilityFixtureStreamSerializationMatrix(t *testing.T) {
	scenarios := []testcompat.Scenario{
		findCompatibilityScenario(t, "parallel-interleaved-calls"),
		findCompatibilityScenario(t, "mixed-text-and-calls"),
		findCompatibilityScenario(t, "abrupt-failure"),
	}
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			openAIState := CreateOpenAIStreamState()
			anthropicState := CreateAnthropicStreamState()
			responsesState := CreateResponsesStreamState()
			var chat strings.Builder
			var anthropicEvents, responseEvents []map[string]interface{}
			for _, event := range testcompat.StreamEvents(scenario) {
				chunk, err := ConvertCIFEventToOpenAISSE(event, openAIState)
				if err != nil {
					t.Fatalf("chat stream: %v", err)
				}
				chat.WriteString(chunk)
				anthropicChunk, err := ConvertCIFEventToAnthropicSSE(event, anthropicState)
				if err != nil {
					t.Fatalf("anthropic stream: %v", err)
				}
				anthropicEvents = append(anthropicEvents, anthropicChunk...)
				events, err := ConvertCIFEventToResponsesSSE(event, responsesState)
				if err != nil {
					t.Fatalf("responses stream: %v", err)
				}
				responseEvents = append(responseEvents, events...)
			}
			anthropicEncoded, _ := json.Marshal(anthropicEvents)
			if scenario.Name != "abrupt-failure" {
				if strings.Count(chat.String(), "[DONE]") != 1 {
					t.Errorf("chat terminal count = %d", strings.Count(chat.String(), "[DONE]"))
				}
				messageStops := 0
				for _, event := range anthropicEvents {
					if event["type"] == "message_stop" {
						messageStops++
					}
				}
				if messageStops != 1 {
					t.Errorf("anthropic terminal count = %d", messageStops)
				}
				completed := 0
				for _, event := range responseEvents {
					if event["type"] == "response.completed" {
						completed++
					}
				}
				if completed != 1 {
					t.Errorf("responses terminal count = %d", completed)
				}
			}
			encoded, _ := json.Marshal(responseEvents)
			for _, exchange := range scenario.Exchanges {
				for name, data := range map[string]string{"chat": chat.String(), "anthropic": string(anthropicEncoded), "responses": string(encoded)} {
					if strings.Count(data, exchange.ID) == 0 {
						t.Errorf("%s omitted tool id %s", name, exchange.ID)
					}
				}
			}
		})
	}
}

func findCompatibilityScenario(t *testing.T, name string) testcompat.Scenario {
	t.Helper()
	for _, scenario := range testcompat.SemanticScenarios() {
		if scenario.Name == name {
			return scenario
		}
	}
	t.Fatalf("missing scenario %q", name)
	return testcompat.Scenario{}
}

var _ cif.CIFStreamEvent
