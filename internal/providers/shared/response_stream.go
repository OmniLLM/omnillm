package shared

import (
	"encoding/json"

	"omnillm/internal/cif"
)

// StreamResponse replays a completed CIF response as a synthetic CIF stream.
// This is useful when the upstream transport must stay non-streaming but the
// downstream client still expects SSE semantics.
func StreamResponse(response *cif.CanonicalResponse) <-chan cif.CIFStreamEvent {
	if response == nil {
		response = &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	}

	responseID := response.ID
	if responseID == "" {
		responseID = RandomID()
	}

	stopReason := response.StopReason
	if stopReason == "" {
		stopReason = cif.StopReasonEndTurn
	}

	eventCh := make(chan cif.CIFStreamEvent, len(response.Content)+2)
	eventCh <- cif.CIFStreamStart{
		Type:  "stream_start",
		ID:    responseID,
		Model: response.Model,
	}

	for index, part := range response.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				ContentBlock: cif.CIFTextPart{
					Type: "text",
					Text: "",
				},
				Delta: cif.TextDelta{
					Type: "text_delta",
					Text: p.Text,
				},
			}
		case cif.CIFThinkingPart:
			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				ContentBlock: cif.CIFThinkingPart{
					Type:      "thinking",
					Thinking:  "",
					Signature: p.Signature,
				},
				Delta: cif.ThinkingDelta{
					Type:     "thinking_delta",
					Thinking: p.Thinking,
				},
			}
		case cif.CIFToolCallPart:
			toolCallID := p.ToolCallID
			if toolCallID == "" {
				toolCallID = "call_" + RandomID()
			}

			partialJSON := "{}"
			if len(p.ToolArguments) > 0 {
				if argsBytes, err := json.Marshal(p.ToolArguments); err == nil {
					partialJSON = string(argsBytes)
				}
			}

			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				ContentBlock: cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    toolCallID,
					ToolName:      p.ToolName,
					ToolArguments: map[string]interface{}{},
				},
				Delta: cif.ToolArgumentsDelta{
					Type:        "tool_arguments_delta",
					PartialJSON: partialJSON,
				},
			}
		}
	}

	eventCh <- cif.CIFStreamEnd{
		Type:         "stream_end",
		StopReason:   stopReason,
		StopSequence: response.StopSequence,
		Usage:        response.Usage,
	}
	close(eventCh)

	return eventCh
}
