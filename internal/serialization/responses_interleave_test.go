package serialization

import (
	"testing"

	"omnillm/internal/cif"
)

func TestResponsesSSEInterleavedToolArgumentsUseCIFIndex(t *testing.T) {
	state := CreateResponsesStreamState()
	events := []cif.CIFStreamEvent{
		cif.CIFStreamStart{Type: "stream_start", ID: "resp", Model: "model"},
		responsesToolAnnouncement(3, "call-a", "first"),
		responsesToolAnnouncement(7, "call-b", "second"),
		// Repeated announcement must not create a duplicate output item.
		responsesToolAnnouncement(3, "call-a", "first"),
		responsesToolDelta(3, `{"a":1}`),
		responsesToolDelta(7, `{"b":2}`),
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	}

	var output []map[string]interface{}
	for _, event := range events {
		converted, err := ConvertCIFEventToResponsesSSE(event, state)
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, converted...)
	}

	added := 0
	deltaIndexes := map[int]string{}
	doneIndexes := map[string]int{}
	for _, event := range output {
		switch event["type"] {
		case "response.output_item.added":
			item := event["item"].(map[string]interface{})
			if item["type"] == "function_call" {
				added++
			}
		case "response.function_call_arguments.delta":
			deltaIndexes[event["output_index"].(int)] = event["delta"].(string)
		case "response.function_call_arguments.done":
			doneIndexes[event["call_id"].(string)] = event["output_index"].(int)
		case "response.output_item.done":
			item := event["item"].(map[string]interface{})
			if item["type"] == "function_call" {
				doneIndexes[item["call_id"].(string)] = event["output_index"].(int)
			}
		}
	}
	if added != 2 {
		t.Fatalf("added %d tool output items, want 2", added)
	}
	if deltaIndexes[0] != `{"a":1}` || deltaIndexes[1] != `{"b":2}` {
		t.Fatalf("argument deltas routed incorrectly: %#v", deltaIndexes)
	}
	if doneIndexes["call-a"] != 0 || doneIndexes["call-b"] != 1 {
		t.Fatalf("terminal indices incorrect: %#v", doneIndexes)
	}
	if state.ToolItemsByIndex[3]["arguments"] != `{"a":1}` || state.ToolItemsByIndex[7]["arguments"] != `{"b":2}` {
		t.Fatalf("accumulated arguments incorrect: %#v", state.ToolItemsByIndex)
	}
}

func TestResponsesSSETextAfterToolKeepsAssignedOutputOrder(t *testing.T) {
	state := CreateResponsesStreamState()
	events := []cif.CIFStreamEvent{
		cif.CIFStreamStart{Type: "stream_start", ID: "resp", Model: "model"},
		responsesToolAnnouncement(3, "call-a", "first"),
		responsesToolDelta(3, `{"a":1}`),
		cif.CIFContentDelta{Type: "content_delta", Index: 7, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "done"}},
		cif.CIFContentBlockStop{Type: "content_block_stop", Index: 7},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	}

	var converted []map[string]interface{}
	for _, event := range events {
		output, err := ConvertCIFEventToResponsesSSE(event, state)
		if err != nil {
			t.Fatal(err)
		}
		converted = append(converted, output...)
	}

	for _, event := range converted {
		switch event["type"] {
		case "response.function_call_arguments.delta":
			if event["output_index"] != 0 {
				t.Fatalf("tool delta output_index = %v, want 0", event["output_index"])
			}
		case "response.output_text.delta", "response.output_text.done":
			if event["output_index"] != 1 {
				t.Fatalf("text event output_index = %v, want 1", event["output_index"])
			}
		case "response.completed":
			response := event["response"].(map[string]interface{})
			output := response["output"].([]interface{})
			if len(output) != 2 || output[0].(map[string]interface{})["type"] != "function_call" || output[1].(map[string]interface{})["type"] != "message" {
				t.Fatalf("completed output order = %#v", output)
			}
		}
	}
}

func TestResponsesSSETextToolTextUsesStableMessageAndContentIndexes(t *testing.T) {
	state := CreateResponsesStreamState()
	events := []cif.CIFStreamEvent{
		cif.CIFStreamStart{Type: "stream_start", ID: "resp", Model: "model"},
		cif.CIFContentDelta{Type: "content_delta", Index: 2, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: "first"}},
		cif.CIFContentBlockStop{Type: "content_block_stop", Index: 2},
		responsesToolAnnouncement(5, "call", "tool"),
		responsesToolDelta(5, `{}`),
		cif.CIFContentDelta{Type: "content_delta", Index: 9, ContentBlock: cif.CIFThinkingPart{Type: "thinking"}, Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: "second"}},
		cif.CIFContentBlockStop{Type: "content_block_stop", Index: 9},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	}

	var converted []map[string]interface{}
	for _, event := range events {
		output, err := ConvertCIFEventToResponsesSSE(event, state)
		if err != nil {
			t.Fatal(err)
		}
		converted = append(converted, output...)
	}

	var textIndexes []int
	for _, event := range converted {
		if event["type"] == "response.output_text.delta" {
			if event["output_index"] != 0 {
				t.Fatalf("text output_index = %v, want stable message index 0", event["output_index"])
			}
			textIndexes = append(textIndexes, event["content_index"].(int))
		}
		if event["type"] == "response.completed" {
			response := event["response"].(map[string]interface{})
			output := response["output"].([]interface{})
			if len(output) != 2 || output[0].(map[string]interface{})["type"] != "message" || output[1].(map[string]interface{})["type"] != "function_call" {
				t.Fatalf("completed output order = %#v", output)
			}
			content := output[0].(map[string]interface{})["content"].([]map[string]interface{})
			if len(content) != 2 || content[0]["text"] != "first" || content[1]["text"] != "second" {
				t.Fatalf("message content = %#v", content)
			}
		}
	}
	if len(textIndexes) != 2 || textIndexes[0] != 0 || textIndexes[1] != 1 {
		t.Fatalf("content indices = %#v, want [0 1]", textIndexes)
	}
}

func TestOpenAISSESuppressesRepeatedToolAnnouncements(t *testing.T) {
	state := CreateOpenAIStreamState()
	events := []cif.CIFStreamEvent{
		cif.CIFStreamStart{Type: "stream_start", ID: "resp", Model: "model"},
		responsesToolAnnouncement(4, "call", "tool"),
		responsesToolAnnouncement(4, "call", "tool"),
		responsesToolDelta(4, `{}`),
	}
	announcements := 0
	for index, event := range events {
		chunk, err := ConvertCIFEventToOpenAISSE(event, state)
		if err != nil {
			t.Fatal(err)
		}
		if chunk != "" && (index == 1 || index == 2) {
			announcements++
		}
	}
	if announcements != 1 {
		t.Fatalf("emitted %d OpenAI tool announcements, want 1", announcements)
	}
}

func responsesToolAnnouncement(index int, id, name string) cif.CIFContentDelta {
	return cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        index,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: id, ToolName: name},
		Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta"},
	}
}

func responsesToolDelta(index int, partial string) cif.CIFContentDelta {
	return cif.CIFContentDelta{
		Type:  "content_delta",
		Index: index,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: partial},
	}
}
