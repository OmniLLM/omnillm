package shared

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"sort"
	"strings"
)

// CollectStream assembles a CanonicalResponse from a CIF stream channel.
func CollectStream(ch <-chan cif.CIFStreamEvent) (*cif.CanonicalResponse, error) {
	response := &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	textParts := make(map[int]*strings.Builder)
	thinkingParts := make(map[int]*strings.Builder)
	thinkingSignatures := make(map[int]*string)
	toolCalls := make(map[int]*cif.CIFToolCallPart)
	toolArgBufs := make(map[int]*strings.Builder)

	for event := range ch {
		switch e := event.(type) {
		case cif.CIFStreamStart:
			response.ID = e.ID
			response.Model = e.Model
		case cif.CIFContentDelta:
			if e.ContentBlock != nil {
				switch cb := e.ContentBlock.(type) {
				case cif.CIFThinkingPart:
					if cb.Signature != nil {
						thinkingSignatures[e.Index] = cb.Signature
					}
				case cif.CIFToolCallPart:
					toolCopy := cb
					toolCalls[e.Index] = &toolCopy
				}
			}
			switch d := e.Delta.(type) {
			case cif.TextDelta:
				if textParts[e.Index] == nil {
					textParts[e.Index] = &strings.Builder{}
				}
				textParts[e.Index].WriteString(d.Text)
			case cif.ThinkingDelta:
				if thinkingParts[e.Index] == nil {
					thinkingParts[e.Index] = &strings.Builder{}
				}
				thinkingParts[e.Index].WriteString(d.Thinking)
			case cif.ToolArgumentsDelta:
				if toolArgBufs[e.Index] == nil {
					toolArgBufs[e.Index] = &strings.Builder{}
				}
				toolArgBufs[e.Index].WriteString(d.PartialJSON)
			}
		case cif.CIFStreamEnd:
			response.StopReason = e.StopReason
			response.Usage = e.Usage
		case cif.CIFStreamError:
			return nil, fmt.Errorf("stream error: %s", e.Error.Message)
		}
	}

	indicesSet := make(map[int]struct{})
	for idx := range thinkingParts {
		indicesSet[idx] = struct{}{}
	}
	for idx := range textParts {
		indicesSet[idx] = struct{}{}
	}
	for idx := range toolCalls {
		indicesSet[idx] = struct{}{}
	}
	indices := make([]int, 0, len(indicesSet))
	for idx := range indicesSet {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		if buf, ok := thinkingParts[idx]; ok && buf.Len() > 0 {
			response.Content = append(response.Content, cif.CIFThinkingPart{
				Type:      "thinking",
				Thinking:  buf.String(),
				Signature: thinkingSignatures[idx],
			})
		}
		if buf, ok := textParts[idx]; ok && buf.Len() > 0 {
			response.Content = append(response.Content, cif.CIFTextPart{Type: "text", Text: buf.String()})
		}
		if tc, ok := toolCalls[idx]; ok {
			finalTC := *tc
			if buf, ok := toolArgBufs[idx]; ok {
				json.Unmarshal([]byte(buf.String()), &finalTC.ToolArguments) //nolint:errcheck
			}
			response.Content = append(response.Content, finalTC)
		}
	}

	return response, nil
}
