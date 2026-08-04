package routes

import (
	"bytes"
	"encoding/json"
	"omnillm/internal/cif"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	if entry.ResultBytes != len("[Tool result missing due to internal error]") {
		t.Fatalf("expected raw result byte length %d, got %d", len("[Tool result missing due to internal error]"), entry.ResultBytes)
	}
}

func TestExtractLatestRawAnthropicToolResultEntriesFallsBackToID(t *testing.T) {
	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":    "tool_result",
						"id":      "call_fallback",
						"name":    "Read",
						"content": "fallback result",
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
	if entry.ToolCallID != "call_fallback" {
		t.Fatalf("expected fallback tool call id call_fallback, got %q", entry.ToolCallID)
	}
	if entry.ToolName != "Read" {
		t.Fatalf("expected tool name Read, got %q", entry.ToolName)
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
	if entry.ResultBytes != len(longResult) {
		t.Fatalf("expected result byte length %d, got %d", len(longResult), entry.ResultBytes)
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
	if entries[0].ArgumentBytes != len(`{"dir":"/tmp"}`) {
		t.Fatalf("expected argument byte length %d, got %d", len(`{"dir":"/tmp"}`), entries[0].ArgumentBytes)
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
	if entry.ArgumentBytes != len(`{"pattern":"TODO","path":"src"}`) {
		t.Fatalf("expected accumulated argument byte length %d, got %d", len(`{"pattern":"TODO","path":"src"}`), entry.ArgumentBytes)
	}
}

func TestToolLoopCallTrackerSeedsArgumentsOnlyBeforeDeltas(t *testing.T) {
	seedArguments := map[string]interface{}{"seed": "value"}
	seedBytes := len(`{"seed":"value"}`)
	deltaJSON := `{"delta":true}`

	tests := []struct {
		name       string
		events     []cif.CIFContentDelta
		wantBytes  int
		wantCallID string
	}{
		{
			name: "repeated content block does not replace accumulated bytes",
			events: []cif.CIFContentDelta{
				{Index: 2, ContentBlock: cif.CIFToolCallPart{ToolCallID: "call_repeat", ToolName: "Search", ToolArguments: seedArguments}},
				{Index: 2, Delta: cif.ToolArgumentsDelta{PartialJSON: deltaJSON}},
				{Index: 2, ContentBlock: cif.CIFToolCallPart{ToolCallID: "call_repeat", ToolName: "Search", ToolArguments: seedArguments}},
			},
			wantBytes:  seedBytes + len(deltaJSON),
			wantCallID: "call_repeat",
		},
		{
			name: "content block after delta updates identity without adding seed",
			events: []cif.CIFContentDelta{
				{Index: 2, Delta: cif.ToolArgumentsDelta{PartialJSON: deltaJSON}},
				{Index: 2, ContentBlock: cif.CIFToolCallPart{ToolCallID: "call_late", ToolName: "Search", ToolArguments: seedArguments}},
			},
			wantBytes:  len(deltaJSON),
			wantCallID: "call_late",
		},
		{
			name: "content block and equivalent delta count one representation",
			events: []cif.CIFContentDelta{
				{
					Index:        2,
					ContentBlock: cif.CIFToolCallPart{ToolCallID: "call_combined", ToolName: "Search", ToolArguments: seedArguments},
					Delta:        cif.ToolArgumentsDelta{PartialJSON: `{"seed":"value"}`},
				},
			},
			wantBytes:  seedBytes,
			wantCallID: "call_combined",
		},
		{
			name: "whitespace delta contributes to payload bytes",
			events: []cif.CIFContentDelta{
				{Index: 2, ContentBlock: cif.CIFToolCallPart{ToolCallID: "call_space", ToolName: "Search"}},
				{Index: 2, Delta: cif.ToolArgumentsDelta{PartialJSON: "   "}},
			},
			wantBytes:  3,
			wantCallID: "call_space",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker := newToolLoopCallTracker()
			for _, event := range test.events {
				tracker.Observe(event)
			}

			entries := tracker.Entries()
			if len(entries) != 1 {
				t.Fatalf("expected 1 tracked tool call, got %d", len(entries))
			}
			entry := entries[0]
			if entry.ArgumentBytes != test.wantBytes {
				t.Fatalf("argument byte length = %d, want %d", entry.ArgumentBytes, test.wantBytes)
			}
			if entry.ToolCallID != test.wantCallID || entry.ToolName != "Search" || entry.BlockIndex != 2 {
				t.Fatalf("tracked identity changed: %#v", entry)
			}
		})
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

func captureToolLoopLogs(t *testing.T, level zerolog.Level, emit func()) []map[string]interface{} {
	t.Helper()

	var output bytes.Buffer
	previousLogger := log.Logger
	log.Logger = zerolog.New(&output).Level(level)
	t.Cleanup(func() { log.Logger = previousLogger })

	emit()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	records := make([]map[string]interface{}, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode captured log %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func assertToolLoopLogsExclude(t *testing.T, records []map[string]interface{}, sentinel string) {
	t.Helper()

	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal captured logs: %v", err)
	}
	if bytes.Contains(encoded, []byte(sentinel)) {
		t.Fatalf("captured logs contain secret sentinel %q", sentinel)
	}
	for _, record := range records {
		for _, field := range []string{"tool_arguments", "tool_result", "raw_inbound_payload"} {
			if _, ok := record[field]; ok {
				t.Fatalf("captured log contains payload field %q: %s", field, encoded)
			}
		}
	}
}

func findToolLoopLog(t *testing.T, records []map[string]interface{}, message string) map[string]interface{} {
	t.Helper()

	for _, record := range records {
		if record["message"] == message {
			return record
		}
	}
	t.Fatalf("missing log message %q in %#v", message, records)
	return nil
}

func TestToolLoopLogsExcludePayloadContent(t *testing.T) {
	const sentinel = "SECRET_SENTINEL_TOOL_PAYLOAD_9f31"
	isError := true
	rawPayload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_raw",
						"name":        "Read",
						"content": []interface{}{
							map[string]interface{}{"type": "text", "text": sentinel},
						},
						"is_error": true,
					},
				},
			},
		},
	}
	request := &cif.CanonicalRequest{
		Model: "test-model",
		Tools: []cif.CIFTool{{Name: anthropicAgentToolName}},
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "call_agent",
						ToolName:   anthropicAgentToolName,
						Content:    sentinel,
						IsError:    &isError,
					},
				},
			},
		},
	}
	response := &cif.CanonicalResponse{
		Content: []cif.CIFContentPart{
			cif.CIFToolCallPart{
				Type:       "tool_call",
				ToolCallID: "call_out",
				ToolName:   "Write",
				ToolArguments: map[string]interface{}{
					"nested": map[string]interface{}{"secret": sentinel},
				},
			},
		},
	}

	records := captureToolLoopLogs(t, zerolog.DebugLevel, func() {
		logRawAnthropicToolLoopPayload("request-secret", rawPayload)
		logAnthropicToolLoopRequest("request-secret", request)
		logAnthropicToolLoopResponse("request-secret", "test-model", "used-model", "provider-1", false, extractToolCallLogEntriesFromResponse(response))
	})

	assertToolLoopLogsExclude(t, records, sentinel)
	rawRecord := findToolLoopLog(t, records, "TOOL LOOP raw inbound tool_result")
	if rawRecord["request_id"] != "request-secret" || rawRecord["tool_call_id"] != "call_raw" {
		t.Fatalf("raw metadata missing: %#v", rawRecord)
	}
	if rawRecord["tool_result_bytes"] != float64(len(sentinel)) {
		t.Fatalf("raw result byte length = %#v, want %d", rawRecord["tool_result_bytes"], len(sentinel))
	}
	canonicalRecord := findToolLoopLog(t, records, "TOOL LOOP inbound tool_result")
	if canonicalRecord["tool_result_bytes"] != float64(len(sentinel)) {
		t.Fatalf("canonical result byte length = %#v, want %d", canonicalRecord["tool_result_bytes"], len(sentinel))
	}
	outboundRecord := findToolLoopLog(t, records, "TOOL LOOP outbound tool_call")
	if outboundRecord["tool_argument_bytes"] == nil || outboundRecord["tool_name"] != "Write" {
		t.Fatalf("outbound metadata missing: %#v", outboundRecord)
	}
	warningRecord := findToolLoopLog(t, records, "AGENT TOOL inbound tool_result indicates local client execution failure")
	if warningRecord["level"] != "warn" || warningRecord["tool_result_bytes"] != float64(len(sentinel)) {
		t.Fatalf("Agent warning metadata missing: %#v", warningRecord)
	}
}

func TestAgentWarningExcludesStructuredResultAtDefaultLogLevel(t *testing.T) {
	const sentinel = "SECRET_SENTINEL_STRUCTURED_RESULT_2c17"
	isError := true
	structuredResult := `{"credentials":{"api_key":"` + sentinel + `"}}`
	request := &cif.CanonicalRequest{
		Model: "test-model",
		Tools: []cif.CIFTool{{Name: anthropicAgentToolName}},
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "call_agent_structured",
						ToolName:   anthropicAgentToolName,
						Content:    structuredResult,
						IsError:    &isError,
					},
				},
			},
		},
	}

	records := captureToolLoopLogs(t, zerolog.InfoLevel, func() {
		logAnthropicToolLoopRequest("request-warning", request)
	})

	assertToolLoopLogsExclude(t, records, sentinel)
	if len(records) != 1 {
		t.Fatalf("expected one warning at info level, got %#v", records)
	}
	warning := findToolLoopLog(t, records, "AGENT TOOL inbound tool_result indicates local client execution failure")
	if warning["level"] != "warn" || warning["tool_result_bytes"] != float64(len(structuredResult)) {
		t.Fatalf("Agent warning metadata missing: %#v", warning)
	}
}

func TestStreamedToolLoopLogsExcludeArgumentContent(t *testing.T) {
	const sentinel = "SECRET_SENTINEL_STREAMED_ARGUMENT_a83d"
	tracker := newToolLoopCallTracker()
	tracker.Observe(cif.CIFContentDelta{
		Type:  "content_delta",
		Index: 4,
		ContentBlock: cif.CIFToolCallPart{
			Type:       "tool_call",
			ToolCallID: "call_stream",
			ToolName:   anthropicAgentToolName,
		},
	})
	first := `{"token":"` + sentinel[:18]
	second := sentinel[18:] + `"}`
	tracker.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 4, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: first}})
	tracker.Observe(cif.CIFContentDelta{Type: "content_delta", Index: 4, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: second}})

	records := captureToolLoopLogs(t, zerolog.DebugLevel, func() {
		logAnthropicToolLoopResponse("request-stream", "test-model", "used-model", "provider-1", true, tracker.Entries())
	})

	assertToolLoopLogsExclude(t, records, sentinel)
	record := findToolLoopLog(t, records, "TOOL LOOP outbound tool_call")
	if record["tool_argument_bytes"] != float64(len(first)+len(second)) {
		t.Fatalf("streamed argument byte length = %#v, want %d", record["tool_argument_bytes"], len(first)+len(second))
	}
	if record["loop_block_index"] != float64(4) || record["tool_call_id"] != "call_stream" || record["stream"] != true {
		t.Fatalf("stream metadata missing: %#v", record)
	}
}
