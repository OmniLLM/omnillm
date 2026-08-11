package ingestion

import (
	"encoding/json"
	"reflect"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCompatibilityFixtureIngestionMatrix(t *testing.T) {
	parsers := []struct {
		name  string
		build func(testcompat.Scenario, bool) json.RawMessage
		parse func(json.RawMessage) (*cif.CanonicalRequest, error)
	}{
		{"chat-completions", testcompat.ChatCompletionsRequest, ParseOpenAIChatCompletions},
		{"anthropic-messages", testcompat.AnthropicRequest, ParseAnthropicMessages},
		{"responses", testcompat.ResponsesRequest, ParseResponsesPayload},
	}

	for _, scenario := range testcompat.SemanticScenarios() {
		if scenario.Name == "abrupt-failure" || scenario.Name == "cancellation" || scenario.Name == "parallel-interleaved-calls" {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			want := normalizeCompatibilityMessages(testcompat.CanonicalMessages(scenario))
			for _, parser := range parsers {
				t.Run(parser.name, func(t *testing.T) {
					request, err := parser.parse(parser.build(scenario, true))
					if err != nil {
						t.Fatalf("parse fixture: %v", err)
					}
					if request.Model != testcompat.Model || !request.Stream {
						t.Fatalf("request metadata = model %q stream %v", request.Model, request.Stream)
					}
					got := normalizeCompatibilityMessages(request.Messages)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("canonical messages differ\ngot:  %#v\nwant: %#v", got, want)
					}
					if scenario.Name == "tool-error" && parser.name == "anthropic-messages" {
						assertCompatibilityToolError(t, request.Messages)
					}
				})
			}
		})
	}
}

type compatibilityPart struct {
	Type      string
	Text      string
	CallID    string
	Name      string
	Arguments map[string]interface{}
	Result    string
	IsError   bool
}

type compatibilityMessage struct {
	Role  string
	Parts []compatibilityPart
}

func assertCompatibilityToolError(t *testing.T, messages []cif.CIFMessage) {
	t.Helper()
	for _, message := range messages {
		user, ok := message.(cif.CIFUserMessage)
		if !ok {
			continue
		}
		for _, part := range user.Content {
			result, ok := part.(cif.CIFToolResultPart)
			if ok && result.ToolCallID == "call_error" {
				if result.IsError == nil || !*result.IsError {
					t.Fatalf("Anthropic tool error metadata was not preserved: %#v", result)
				}
				return
			}
		}
	}
	t.Fatal("Anthropic tool error result not found")
}

func normalizeCompatibilityMessages(messages []cif.CIFMessage) []compatibilityMessage {
	result := make([]compatibilityMessage, 0, len(messages))
	for _, message := range messages {
		var parts []cif.CIFContentPart
		switch typed := message.(type) {
		case cif.CIFUserMessage:
			parts = typed.Content
		case cif.CIFAssistantMessage:
			parts = typed.Content
		case cif.CIFSystemMessage:
			continue
		}
		normalized := compatibilityMessage{Role: message.GetRole()}
		for _, part := range parts {
			switch typed := part.(type) {
			case cif.CIFTextPart:
				normalized.Parts = append(normalized.Parts, compatibilityPart{Type: "text", Text: typed.Text})
			case cif.CIFThinkingPart:
				// Chat Completions and Responses have no historical reasoning item.
				continue
			case cif.CIFToolCallPart:
				normalized.Parts = append(normalized.Parts, compatibilityPart{Type: "tool_call", CallID: typed.ToolCallID, Name: typed.ToolName, Arguments: typed.ToolArguments})
			case cif.CIFToolResultPart:
				normalized.Parts = append(normalized.Parts, compatibilityPart{Type: "tool_result", CallID: typed.ToolCallID, Result: typed.Content, IsError: typed.IsError != nil && *typed.IsError})
			}
		}
		// Error markers are dialect-specific: Anthropic preserves is_error while
		// Chat Completions and Responses carry only the associated result.
		for index := range normalized.Parts {
			normalized.Parts[index].IsError = false
		}
		result = append(result, normalized)
	}
	return result
}
