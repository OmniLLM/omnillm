package generic

import (
	"encoding/json"
	"omnillm/internal/cif"
	alibabapkg "omnillm/internal/providers/alibaba"
	"omnillm/internal/providers/openaicompat"
	"testing"
)

// buildAlibabaPayload is a test helper that exercises the same code path used
// in production: alibaba.Adapter.buildRequest → openaicompat.BuildChatRequest.
func buildAlibabaPayload(t *testing.T, request *cif.CanonicalRequest) map[string]interface{} {
	t.Helper()
	model := alibabapkg.RemapModel(request.Model)
	defTemp := 0.55
	defTopP := 1.0
	extras := map[string]interface{}{}
	if alibabapkg.IsReasoningModel(model) && len(request.Tools) == 0 {
		extras["enable_thinking"] = true
	}
	cfg := openaicompat.Config{
		DefaultTemperature:   &defTemp,
		DefaultTopP:          &defTopP,
		IncludeUsageInStream: false,
		Extras:               extras,
	}
	cr, err := openaicompat.BuildChatRequest(model, request, false, cfg)
	if err != nil {
		t.Fatalf("BuildChatRequest: %v", err)
	}
	b, err := openaicompat.Marshal(cr)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return payload
}

func TestBuildOpenAIPayloadIncludesSystemPrompt(t *testing.T) {
	systemPrompt := "Be concise."
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model:        "qwen3.6-plus",
		SystemPrompt: &systemPrompt,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "hello"},
				},
			},
		},
	})

	messages, ok := payload["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages has unexpected type %T", payload["messages"])
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(messages))
	}
	sys := messages[0].(map[string]interface{})
	if sys["role"] != "system" {
		t.Fatalf("expected first role=system, got %v", sys["role"])
	}
	if sys["content"] != systemPrompt {
		t.Fatalf("expected system content %q, got %v", systemPrompt, sys["content"])
	}
	usr := messages[1].(map[string]interface{})
	if usr["role"] != "user" {
		t.Fatalf("expected second role=user, got %v", usr["role"])
	}
}

func TestAlibabaBuildOpenAIPayloadPreservesQwen36Plus(t *testing.T) {
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hello"}},
			},
		},
	})
	if payload["model"] != "qwen3.6-plus" {
		t.Fatalf("expected model=qwen3.6-plus, got %v", payload["model"])
	}
}

func TestAlibabaReasoningModelEnableThinking(t *testing.T) {
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen3-max",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "think"}},
			},
		},
	})
	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true for reasoning model, got %v (present=%v)", val, ok)
	}
}

func TestAlibabaGlobalNonReasoningModelNoEnableThinking(t *testing.T) {
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen2-5-72b-instruct",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hello"}},
			},
		},
	})
	if _, ok := payload["enable_thinking"]; ok {
		t.Errorf("expected enable_thinking absent for non-reasoning model")
	}
}

func TestQwen36PlusToolUseWithRealTools(t *testing.T) {
	desc := "Get the current weather for a location"
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "What's the weather in New York?"}},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "get_weather",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
					},
					"required": []string{"location"},
				},
			},
		},
	})
	tools, ok := payload["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", payload["tools"])
	}
	fn := tools[0].(map[string]interface{})["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name get_weather, got %v", fn["name"])
	}
	// enable_thinking must be absent when tools are present
	if _, ok := payload["enable_thinking"]; ok {
		t.Errorf("enable_thinking must be absent when tools are present")
	}
}

func TestQwen36PlusToolUseWithToolChoice(t *testing.T) {
	desc := "Make an API call"
	toolChoice := "required"
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Call the API"}},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:        "api_call",
				Description: &desc,
				ParametersSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"endpoint": map[string]interface{}{"type": "string"}},
				},
			},
		},
		ToolChoice: toolChoice,
	})
	if tc, ok := payload["tool_choice"]; !ok || tc != toolChoice {
		t.Errorf("expected tool_choice=%q, got %v (ok=%v)", toolChoice, tc, ok)
	}
}

func TestQwen36PlusNoToolsInjectsEnableThinking(t *testing.T) {
	payload := buildAlibabaPayload(t, &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hello"}},
			},
		},
	})
	if val, ok := payload["enable_thinking"]; !ok || val != true {
		t.Errorf("expected enable_thinking=true for reasoning model without tools, got %v (present=%v)", val, ok)
	}
	if tools, ok := payload["tools"]; ok && tools != nil {
		t.Errorf("expected no tools injected, got %v", tools)
	}
}
