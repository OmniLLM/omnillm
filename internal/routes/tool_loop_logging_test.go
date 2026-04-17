package routes

import (
	"strings"
	"testing"

	"omnimodel/internal/cif"
)

func TestExtractLatestRawAnthropicToolResultEntriesUsesMostRecentUserToolResults(t *testing.T) {
	isError := true
	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "older_call",
						"name":        "Read",
						"content":     "old result",
					},
				},
			},
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "call_fs",
						"name": "Read",
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "tool output follows"},
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_fs",
						"name":        "Read",
						"content":     "[Tool result missing due to internal error]",
						"is_error":    isError,
					},
				},
			},
		},
	}

	entries := extractLatestRawAnthropicToolResultEntries(payload)
	if len(entries) != 1 {
		t.Fatalf("expected 1 latest raw tool result entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.MessageIndex != 2 {
		t.Fatalf("expected latest message index 2, got %d", entry.MessageIndex)
	}
	if entry.ItemIndex != 1 {
		t.Fatalf("expected latest item index 1, got %d", entry.ItemIndex)
	}
	if entry.ToolCallID != "call_fs" {
		t.Fatalf("expected tool call id call_fs, got %q", entry.ToolCallID)
	}
	if entry.ToolName != "Read" {
		t.Fatalf("expected tool name Read, got %q", entry.ToolName)
	}
	if entry.IsError == nil || !*entry.IsError {
		t.Fatalf("expected is_error=true, got %#v", entry.IsError)
	}
	if entry.ResultPreview != "[Tool result missing due to internal error]" {
		t.Fatalf("unexpected raw result preview: %q", entry.ResultPreview)
	}
}

func TestExtractLatestToolResultLogEntriesUsesMostRecentUserToolResults(t *testing.T) {
	isError := true
	longResult := strings.Repeat("result-", 80)
	request := &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "older_call",
						ToolName:   "old_tool",
						Content:    "old result",
					},
				},
			},
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_fs",
						ToolName:      "read_file",
						ToolArguments: map[string]interface{}{"path": "/tmp/demo"},
					},
				},
			},
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "tool output follows"},
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "call_fs",
						ToolName:   "read_file",
						Content:    longResult,
						IsError:    &isError,
					},
				},
			},
		},
	}

	entries := extractLatestToolResultLogEntries(request)
	if len(entries) != 1 {
		t.Fatalf("expected 1 latest tool result entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.MessageIndex != 2 {
		t.Fatalf("expected latest message index 2, got %d", entry.MessageIndex)
	}
	if entry.ItemIndex != 1 {
		t.Fatalf("expected latest item index 1, got %d", entry.ItemIndex)
	}
	if entry.ToolCallID != "call_fs" {
		t.Fatalf("expected tool call id call_fs, got %q", entry.ToolCallID)
	}
	if entry.ToolName != "read_file" {
		t.Fatalf("expected tool name read_file, got %q", entry.ToolName)
	}
	if entry.IsError == nil || !*entry.IsError {
		t.Fatalf("expected is_error=true, got %#v", entry.IsError)
	}
	if !strings.HasSuffix(entry.ResultPreview, "...(truncated)") {
		t.Fatalf("expected truncated result preview, got %q", entry.ResultPreview)
	}
}

func TestExtractToolCallLogEntriesFromResponseCapturesArguments(t *testing.T) {
	response := &cif.CanonicalResponse{
		ID:    "resp_1",
		Model: "qwen3.6-plus",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "working"},
			cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    "call_1",
				ToolName:      "list_files",
				ToolArguments: map[string]interface{}{"dir": "/tmp"},
			},
		},
		StopReason: cif.StopReasonToolUse,
	}

	entries := extractToolCallLogEntriesFromResponse(response)
	if len(entries) != 1 {
		t.Fatalf("expected 1 tool call entry, got %d", len(entries))
	}
	if entries[0].BlockIndex != 1 {
		t.Fatalf("expected block index 1, got %d", entries[0].BlockIndex)
	}
	if entries[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %q", entries[0].ToolCallID)
	}
	if entries[0].ToolName != "list_files" {
		t.Fatalf("expected tool name list_files, got %q", entries[0].ToolName)
	}
	if entries[0].ArgumentsPreview != `{"dir":"/tmp"}` {
		t.Fatalf("unexpected arguments preview: %q", entries[0].ArgumentsPreview)
	}
}

func TestToolLoopCallTrackerAccumulatesStreamedArguments(t *testing.T) {
	tracker := newToolLoopCallTracker()

	tracker.Observe(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 3,
		ContentBlock: cif.CIFToolCallPart{
			Type:          "tool_call",
			ToolCallID:    "call_loop",
			ToolName:      "search_files",
			ToolArguments: map[string]interface{}{},
		},
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
	})
	tracker.Observe(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 3,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"pattern":"TODO",`},
	})
	tracker.Observe(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 3,
		Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"path":"src"}`},
	})

	entries := tracker.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 tracked tool call, got %d", len(entries))
	}
	entry := entries[0]
	if entry.BlockIndex != 3 {
		t.Fatalf("expected block index 3, got %d", entry.BlockIndex)
	}
	if entry.ToolCallID != "call_loop" {
		t.Fatalf("expected tool call id call_loop, got %q", entry.ToolCallID)
	}
	if entry.ToolName != "search_files" {
		t.Fatalf("expected tool name search_files, got %q", entry.ToolName)
	}
	if entry.ArgumentsPreview != `{"pattern":"TODO","path":"src"}` {
		t.Fatalf("unexpected accumulated arguments: %q", entry.ArgumentsPreview)
	}
}

func TestExtractAgentToolTranscriptGapsDetectsMissingImmediateToolResult(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "explain codebase"}},
			},
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "I'll explore the repository."},
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_agent_1",
						ToolName:      anthropicAgentToolName,
						ToolArguments: map[string]interface{}{"subagent_type": "Explore"},
					},
				},
			},
			cif.CIFAssistantMessage{
				Role:    "assistant",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "stalled"}},
			},
		},
	}

	gaps := extractAgentToolTranscriptGaps(request)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 Agent pairing gap, got %d", len(gaps))
	}
	if gaps[0].AssistantMessageIndex != 1 {
		t.Fatalf("expected assistant message index 1, got %d", gaps[0].AssistantMessageIndex)
	}
	if gaps[0].NextMessageIndex != 2 {
		t.Fatalf("expected next message index 2, got %d", gaps[0].NextMessageIndex)
	}
	if gaps[0].NextMessageRole != "assistant" {
		t.Fatalf("expected next message role assistant, got %q", gaps[0].NextMessageRole)
	}
	if gaps[0].ToolCallID != "call_agent_1" {
		t.Fatalf("expected tool call id call_agent_1, got %q", gaps[0].ToolCallID)
	}
}

func TestExtractAgentToolTranscriptGapsIgnoresSatisfiedPair(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model: "qwen3.6-plus",
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_agent_ok",
						ToolName:      anthropicAgentToolName,
						ToolArguments: map[string]interface{}{"subagent_type": "Explore"},
					},
				},
			},
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "call_agent_ok",
						ToolName:   anthropicAgentToolName,
						Content:    "subagent finished",
					},
				},
			},
		},
	}

	if gaps := extractAgentToolTranscriptGaps(request); len(gaps) != 0 {
		t.Fatalf("expected no Agent pairing gaps, got %d", len(gaps))
	}
}

func TestFilterErroredToolResultEntriesReturnsOnlyErroredEntries(t *testing.T) {
	isError := true
	isNotError := false
	entries := []toolLoopResultLogEntry{
		{ToolCallID: "call_err", ToolName: anthropicAgentToolName, IsError: &isError},
		{ToolCallID: "call_ok", ToolName: anthropicAgentToolName, IsError: &isNotError},
		{ToolCallID: "call_nil", ToolName: anthropicAgentToolName, IsError: nil},
	}

	filtered := filterErroredToolResultEntries(entries)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 errored tool result, got %d", len(filtered))
	}
	if filtered[0].ToolCallID != "call_err" {
		t.Fatalf("expected errored tool call id call_err, got %q", filtered[0].ToolCallID)
	}
}

func TestFilterToolCallEntriesByNameReturnsOnlyAgentCalls(t *testing.T) {
	entries := []toolLoopCallLogEntry{
		{ToolCallID: "call_agent", ToolName: anthropicAgentToolName},
		{ToolCallID: "call_read", ToolName: "Read"},
	}

	filtered := filterToolCallEntriesByName(entries, anthropicAgentToolName)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 Agent tool call, got %d", len(filtered))
	}
	if filtered[0].ToolCallID != "call_agent" {
		t.Fatalf("expected Agent call id call_agent, got %q", filtered[0].ToolCallID)
	}
}
