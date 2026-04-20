// Package google — Gemini SSE stream parser.
package google

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"strings"

	"github.com/rs/zerolog/log"
)

// ParseGeminiSSE parses a Google Gemini SSE stream into CIF events.
func ParseGeminiSSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 4*1024*1024)

	var streamStartSent bool
	var textIndex int
	toolCallIndex := 1000

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var envelope struct {
			Candidates []struct {
				Content struct {
					Parts []map[string]interface{} `json:"parts"`
					Role  string                   `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}

		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			log.Warn().Err(err).Msg("Failed to parse Google Gemini SSE line")
			continue
		}

		if len(envelope.Candidates) == 0 {
			continue
		}

		if !streamStartSent {
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: shared.RandomID(), Model: "google"}
			streamStartSent = true
		}

		candidate := envelope.Candidates[0]

		for _, part := range candidate.Content.Parts {
			if text, ok := part["text"].(string); ok && text != "" {
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        textIndex,
					ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
					Delta:        cif.TextDelta{Type: "text_delta", Text: text},
				}
			} else if fcMap, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fcMap["name"].(string)
				args := shared.NormalizeToolArguments(fcMap["args"])
				argsJSON, _ := json.Marshal(args)
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: toolCallIndex,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    fmt.Sprintf("call_%s", shared.RandomID()),
						ToolName:      name,
						ToolArguments: args,
					},
					Delta: cif.ToolArgumentsDelta{
						Type:        "tool_arguments_delta",
						PartialJSON: string(argsJSON),
					},
				}
				toolCallIndex++
			}
		}

		if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" {
			usage := envelope.UsageMetadata
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: StopReason(candidate.FinishReason),
				Usage: &cif.CIFUsage{
					InputTokens:  usage.PromptTokenCount,
					OutputTokens: usage.CandidatesTokenCount,
				},
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
		return
	}

	if streamStartSent {
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
	}
}

// StopReason converts a Google Gemini finishReason to a CIF stop reason.
func StopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "STOP":
		return cif.StopReasonEndTurn
	case "MAX_TOKENS":
		return cif.StopReasonMaxTokens
	case "FUNCTION_CALL":
		return cif.StopReasonToolUse
	case "SAFETY", "RECITATION", "LANGUAGE":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}
