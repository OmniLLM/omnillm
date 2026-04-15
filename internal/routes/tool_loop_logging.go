package routes

import (
	"encoding/json"
	"strings"

	"omnimodel/internal/cif"

	"github.com/rs/zerolog/log"
)

const toolLoopLogValueLimit = 400
const anthropicAgentToolName = "Agent"

type toolLoopResultLogEntry struct {
	MessageIndex  int
	ItemIndex     int
	ToolCallID    string
	ToolName      string
	ResultPreview string
	IsError       *bool
}

type toolLoopCallLogEntry struct {
	BlockIndex       int
	ToolCallID       string
	ToolName         string
	ArgumentsPreview string
}

type agentToolTranscriptGap struct {
	AssistantMessageIndex int
	NextMessageIndex      int
	NextMessageRole       string
	ToolCallID            string
}

type toolLoopCallTracker struct {
	callsByIndex map[int]*toolLoopCallState
	order        []int
}

type toolLoopCallState struct {
	blockIndex int
	toolCallID string
	toolName   string
	rawArgs    strings.Builder
}

func newToolLoopCallTracker() *toolLoopCallTracker {
	return &toolLoopCallTracker{
		callsByIndex: make(map[int]*toolLoopCallState),
	}
}

func (t *toolLoopCallTracker) Observe(event cif.CIFStreamEvent) {
	if t == nil {
		return
	}

	delta, ok := event.(cif.CIFContentDelta)
	if !ok {
		return
	}

	if contentBlock, ok := delta.ContentBlock.(cif.CIFToolCallPart); ok {
		state := t.ensure(delta.Index)
		if contentBlock.ToolCallID != "" {
			state.toolCallID = contentBlock.ToolCallID
		}
		if contentBlock.ToolName != "" {
			state.toolName = contentBlock.ToolName
		}
		if state.rawArgs.Len() == 0 && len(contentBlock.ToolArguments) > 0 {
			state.rawArgs.WriteString(mustMarshalCompactJSON(contentBlock.ToolArguments))
		}
	}

	argsDelta, ok := delta.Delta.(cif.ToolArgumentsDelta)
	if !ok || strings.TrimSpace(argsDelta.PartialJSON) == "" {
		return
	}

	state := t.ensure(delta.Index)
	state.rawArgs.WriteString(argsDelta.PartialJSON)
}

func (t *toolLoopCallTracker) Entries() []toolLoopCallLogEntry {
	if t == nil {
		return nil
	}

	entries := make([]toolLoopCallLogEntry, 0, len(t.order))
	for _, idx := range t.order {
		state := t.callsByIndex[idx]
		if state == nil {
			continue
		}
		entries = append(entries, toolLoopCallLogEntry{
			BlockIndex:       state.blockIndex,
			ToolCallID:       state.toolCallID,
			ToolName:         state.toolName,
			ArgumentsPreview: truncateToolLoopValue(state.rawArgs.String()),
		})
	}
	return entries
}

func (t *toolLoopCallTracker) ensure(index int) *toolLoopCallState {
	if state, ok := t.callsByIndex[index]; ok {
		return state
	}

	state := &toolLoopCallState{blockIndex: index}
	t.callsByIndex[index] = state
	t.order = append(t.order, index)
	return state
}

func extractLatestToolResultLogEntries(request *cif.CanonicalRequest) []toolLoopResultLogEntry {
	if request == nil {
		return nil
	}

	for messageIndex := len(request.Messages) - 1; messageIndex >= 0; messageIndex-- {
		userMessage, ok := request.Messages[messageIndex].(cif.CIFUserMessage)
		if !ok {
			continue
		}

		entries := make([]toolLoopResultLogEntry, 0, len(userMessage.Content))
		for itemIndex, part := range userMessage.Content {
			toolResult, ok := part.(cif.CIFToolResultPart)
			if !ok {
				continue
			}
			entries = append(entries, toolLoopResultLogEntry{
				MessageIndex:  messageIndex,
				ItemIndex:     itemIndex,
				ToolCallID:    toolResult.ToolCallID,
				ToolName:      toolResult.ToolName,
				ResultPreview: truncateToolLoopValue(toolResult.Content),
				IsError:       toolResult.IsError,
			})
		}
		if len(entries) > 0 {
			return entries
		}
	}

	return nil
}

func extractToolCallLogEntriesFromResponse(response *cif.CanonicalResponse) []toolLoopCallLogEntry {
	if response == nil {
		return nil
	}

	entries := make([]toolLoopCallLogEntry, 0, len(response.Content))
	for blockIndex, part := range response.Content {
		toolCall, ok := part.(cif.CIFToolCallPart)
		if !ok {
			continue
		}
		entries = append(entries, toolLoopCallLogEntry{
			BlockIndex:       blockIndex,
			ToolCallID:       toolCall.ToolCallID,
			ToolName:         toolCall.ToolName,
			ArgumentsPreview: truncateToolLoopValue(mustMarshalCompactJSON(toolCall.ToolArguments)),
		})
	}
	return entries
}

func hasToolNamed(request *cif.CanonicalRequest, toolName string) bool {
	if request == nil {
		return false
	}

	for _, tool := range request.Tools {
		if tool.Name == toolName {
			return true
		}
	}

	return false
}

func filterToolResultEntriesByName(entries []toolLoopResultLogEntry, toolName string) []toolLoopResultLogEntry {
	filtered := make([]toolLoopResultLogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ToolName == toolName {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func filterToolCallEntriesByName(entries []toolLoopCallLogEntry, toolName string) []toolLoopCallLogEntry {
	filtered := make([]toolLoopCallLogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ToolName == toolName {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func extractAgentToolTranscriptGaps(request *cif.CanonicalRequest) []agentToolTranscriptGap {
	if request == nil {
		return nil
	}

	var gaps []agentToolTranscriptGap
	for messageIndex, message := range request.Messages {
		assistantMessage, ok := message.(cif.CIFAssistantMessage)
		if !ok {
			continue
		}

		agentCallIDs := make(map[string]struct{})
		for _, part := range assistantMessage.Content {
			toolCall, ok := part.(cif.CIFToolCallPart)
			if !ok || toolCall.ToolName != anthropicAgentToolName {
				continue
			}
			agentCallIDs[toolCall.ToolCallID] = struct{}{}
		}
		if len(agentCallIDs) == 0 {
			continue
		}

		nextMessageIndex := messageIndex + 1
		if nextMessageIndex >= len(request.Messages) {
			for toolCallID := range agentCallIDs {
				gaps = append(gaps, agentToolTranscriptGap{
					AssistantMessageIndex: messageIndex,
					NextMessageIndex:      -1,
					ToolCallID:            toolCallID,
				})
			}
			continue
		}

		nextMessage := request.Messages[nextMessageIndex]
		userMessage, ok := nextMessage.(cif.CIFUserMessage)
		if !ok {
			for toolCallID := range agentCallIDs {
				gaps = append(gaps, agentToolTranscriptGap{
					AssistantMessageIndex: messageIndex,
					NextMessageIndex:      nextMessageIndex,
					NextMessageRole:       nextMessage.GetRole(),
					ToolCallID:            toolCallID,
				})
			}
			continue
		}

		matchedCallIDs := make(map[string]struct{})
		for _, part := range userMessage.Content {
			toolResult, ok := part.(cif.CIFToolResultPart)
			if !ok {
				continue
			}
			if _, exists := agentCallIDs[toolResult.ToolCallID]; exists {
				matchedCallIDs[toolResult.ToolCallID] = struct{}{}
			}
		}

		for toolCallID := range agentCallIDs {
			if _, matched := matchedCallIDs[toolCallID]; matched {
				continue
			}
			gaps = append(gaps, agentToolTranscriptGap{
				AssistantMessageIndex: messageIndex,
				NextMessageIndex:      nextMessageIndex,
				NextMessageRole:       userMessage.GetRole(),
				ToolCallID:            toolCallID,
			})
		}
	}

	return gaps
}

func logAnthropicToolLoopRequest(requestID string, request *cif.CanonicalRequest) {
	for _, entry := range extractLatestToolResultLogEntries(request) {
		event := log.Info().
			Str("request_id", requestID).
			Str("api_shape", "anthropic").
			Str("model_requested", request.Model).
			Int("loop_message_index", entry.MessageIndex).
			Int("loop_item_index", entry.ItemIndex).
			Str("tool_call_id", entry.ToolCallID).
			Str("tool_name", entry.ToolName).
			Str("tool_result", entry.ResultPreview)
		if entry.IsError != nil {
			event = event.Bool("tool_is_error", *entry.IsError)
		}
		event.Msg("TOOL LOOP inbound tool_result")
	}

	logAnthropicAgentGuardrailRequest(requestID, request)
}

func logAnthropicToolLoopResponse(requestID string, originalModel string, modelUsed string, providerID string, stream bool, entries []toolLoopCallLogEntry) {
	for _, entry := range entries {
		log.Info().
			Str("request_id", requestID).
			Str("api_shape", "anthropic").
			Str("model_requested", originalModel).
			Str("model_used", modelUsed).
			Str("provider", providerID).
			Bool("stream", stream).
			Int("loop_block_index", entry.BlockIndex).
			Str("tool_call_id", entry.ToolCallID).
			Str("tool_name", entry.ToolName).
			Str("tool_arguments", entry.ArgumentsPreview).
			Msg("TOOL LOOP outbound tool_call")
	}

	logAnthropicAgentGuardrailResponse(requestID, originalModel, modelUsed, providerID, stream, entries)
}

func logAnthropicAgentGuardrailRequest(requestID string, request *cif.CanonicalRequest) {
	if request == nil {
		return
	}

	hasAgentTool := hasToolNamed(request, anthropicAgentToolName)
	agentResults := filterToolResultEntriesByName(extractLatestToolResultLogEntries(request), anthropicAgentToolName)
	agentGaps := extractAgentToolTranscriptGaps(request)
	if !hasAgentTool && len(agentResults) == 0 && len(agentGaps) == 0 {
		return
	}

	log.Info().
		Str("request_id", requestID).
		Str("api_shape", "anthropic").
		Str("model_requested", request.Model).
		Bool("agent_tool_available", hasAgentTool).
		Int("latest_agent_tool_results", len(agentResults)).
		Int("agent_tool_pairing_gaps", len(agentGaps)).
		Msg("AGENT TOOL inbound guardrail")

	for _, gap := range agentGaps {
		event := log.Warn().
			Str("request_id", requestID).
			Str("api_shape", "anthropic").
			Str("model_requested", request.Model).
			Int("assistant_message_index", gap.AssistantMessageIndex).
			Str("tool_call_id", gap.ToolCallID).
			Bool("likely_client_tool_result_drop", true)
		if gap.NextMessageIndex >= 0 {
			event = event.
				Int("next_message_index", gap.NextMessageIndex).
				Str("next_message_role", gap.NextMessageRole)
		} else {
			event = event.Str("next_message_role", "missing")
		}
		event.Msg("AGENT TOOL inbound transcript is missing the immediate tool_result for a prior Agent tool_call")
	}
}

func logAnthropicAgentGuardrailResponse(requestID string, originalModel string, modelUsed string, providerID string, stream bool, entries []toolLoopCallLogEntry) {
	for _, entry := range filterToolCallEntriesByName(entries, anthropicAgentToolName) {
		log.Info().
			Str("request_id", requestID).
			Str("api_shape", "anthropic").
			Str("model_requested", originalModel).
			Str("model_used", modelUsed).
			Str("provider", providerID).
			Bool("stream", stream).
			Str("tool_call_id", entry.ToolCallID).
			Str("tool_name", entry.ToolName).
			Bool("expected_client_tool_result", true).
			Str("failure_boundary", "local_claude_client_after_outbound_tool_call").
			Msg("AGENT TOOL outbound tool_call requires a local client tool_result on the next Anthropic turn")
	}
}

func truncateToolLoopValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= toolLoopLogValueLimit {
		return trimmed
	}
	return trimmed[:toolLoopLogValueLimit] + "...(truncated)"
}

func mustMarshalCompactJSON(value interface{}) string {
	if value == nil {
		return ""
	}
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}
