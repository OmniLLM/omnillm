package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"

	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

type copilotResponsesResponse struct {
	ID                string                       `json:"id"`
	Model             string                       `json:"model"`
	Status            string                       `json:"status"`
	Output            []copilotResponsesOutputItem `json:"output"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
	Usage *copilotResponsesUsage `json:"usage,omitempty"`
}

type copilotResponsesOutputItem struct {
	Type      string                         `json:"type"`
	ID        string                         `json:"id"`
	CallID    string                         `json:"call_id,omitempty"`
	Role      string                         `json:"role,omitempty"`
	Name      string                         `json:"name,omitempty"`
	Arguments string                         `json:"arguments,omitempty"`
	Input     string                         `json:"input,omitempty"`
	Namespace string                         `json:"namespace,omitempty"`
	Content   []copilotResponsesContentBlock `json:"content,omitempty"`
	Status    string                         `json:"status,omitempty"`
}

type copilotResponsesContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type copilotResponsesInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type copilotResponsesUsage struct {
	InputTokens        int                                 `json:"input_tokens"`
	OutputTokens       int                                 `json:"output_tokens"`
	InputTokensDetails *copilotResponsesInputTokensDetails `json:"input_tokens_details,omitempty"`
}

// ---------------------------------------------------------------------------
// Payload builder
// ---------------------------------------------------------------------------

// buildResponsesPayload converts a CIF request into the Responses API payload map.
func (a *CopilotAdapter) buildResponsesPayload(request *cif.CanonicalRequest, stream bool) map[string]any {
	if request == nil {
		request = &cif.CanonicalRequest{}
	}
	model := a.RemapModel(request.Model)

	payload := map[string]any{
		"model":  model,
		"input":  cifMessagesToResponsesInput(request),
		"stream": stream,
	}

	if system := cif.PlainSystemText(request.System); strings.TrimSpace(system) != "" {
		payload["instructions"] = system
	}

	if !shared.IsReasoningModel(model) {
		if request.Temperature != nil {
			payload["temperature"] = *request.Temperature
		}
		if request.TopP != nil {
			payload["top_p"] = *request.TopP
		}
	}

	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		if copilotModelUsesMaxCompletionTokens(model) {
			payload["max_completion_tokens"] = *request.MaxTokens
		} else {
			maxOut := *request.MaxTokens
			if maxOut < 16 {
				maxOut = 16
			}
			payload["max_output_tokens"] = maxOut
		}
	}

	if request.UserID != nil {
		payload["user"] = shared.TruncateOpenAIUserID(*request.UserID)
	}

	if len(request.Tools) > 0 {
		payload["parallel_tool_calls"] = true
		tools := make([]map[string]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			item := map[string]any{
				"name": tool.Name,
			}
			if tool.ToolKind == cif.CIFToolKindCustom {
				item["type"] = "custom"
				if tool.Format != nil {
					item["format"] = tool.Format
				}
			} else {
				item["type"] = "function"
				item["parameters"] = shared.NormalizeToolParameters(tool.ParametersSchema)
			}
			if tool.Description != nil {
				item["description"] = *tool.Description
			}
			tools = append(tools, item)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		if tc := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); tc != nil {
			model := payload["model"].(string)
			// Reasoning models reject tool_choice with specific function names.
			if shared.IsReasoningModel(model) {
				if _, isString := tc.(string); isString {
					payload["tool_choice"] = tc
				}
			} else {
				payload["tool_choice"] = normalizeResponsesToolChoice(tc)
			}
		}
	}

	return payload
}

func normalizeResponsesToolChoice(toolChoice any) any {
	choice, ok := toolChoice.(map[string]interface{})
	if !ok || choice["type"] != "function" {
		return toolChoice
	}
	function, ok := choice["function"].(map[string]interface{})
	if !ok {
		return toolChoice
	}
	name, _ := function["name"].(string)
	if name == "" {
		return toolChoice
	}
	return map[string]interface{}{
		"type": "function",
		"name": name,
	}
}

// cifMessagesToResponsesInput converts CIF messages to the Responses API input array.
func cifMessagesToResponsesInput(request *cif.CanonicalRequest) []map[string]any {
	if request == nil {
		return nil
	}
	var input []map[string]any
	for _, message := range request.Messages {
		switch m := message.(type) {
		case cif.CIFSystemMessage:
			input = append(input, map[string]any{
				"type": "message",
				"role": "system",
				"content": []map[string]any{
					{"type": "input_text", "text": cif.PlainSystemText(m.Content)},
				},
			})
		case cif.CIFUserMessage:
			input = append(input, responsesUserItems(m)...)
		case cif.CIFAssistantMessage:
			input = append(input, responsesAssistantItems(m)...)
		}
	}
	return input
}

func responsesUserItems(message cif.CIFUserMessage) []map[string]any {
	var items []map[string]any
	var content []map[string]any

	flush := func() {
		if len(content) == 0 {
			return
		}
		items = append(items, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": content,
		})
		content = nil
	}

	for _, part := range message.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			content = append(content, map[string]any{
				"type": "input_text",
				"text": p.Text,
			})
		case cif.CIFImagePart:
			imgURL := responsesImageURL(p)
			if imgURL == "" {
				continue
			}
			content = append(content, map[string]any{
				"type":      "input_image",
				"image_url": imgURL,
			})
		case cif.CIFToolResultPart:
			flush()
			out := p.Content
			if p.IsError != nil && *p.IsError && out == "" {
				out = "Error: tool call failed"
			}
			if p.ToolKind == cif.CIFToolKindCustom {
				customOutput := p.CustomOutput
				if customOutput == nil {
					customOutput = out
				}
				items = append(items, map[string]any{
					"type":    "custom_tool_call_output",
					"call_id": p.ToolCallID,
					"output":  customOutput,
				})
				continue
			}
			output := p.RawOutput
			if output == nil {
				output = out
			}
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": p.ToolCallID,
				"output":  output,
			})
		}
	}
	flush()
	return items
}

func responsesAssistantItems(message cif.CIFAssistantMessage) []map[string]any {
	var items []map[string]any
	var content []map[string]any

	flush := func() {
		if len(content) == 0 {
			return
		}
		items = append(items, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": content,
		})
		content = nil
	}

	for _, part := range message.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			content = append(content, map[string]any{
				"type": "output_text",
				"text": p.Text,
			})
		case cif.CIFThinkingPart:
			content = append(content, map[string]any{
				"type": "output_text",
				"text": fmt.Sprintf("<thinking>\n%s\n</thinking>", p.Thinking),
			})
		case cif.CIFToolCallPart:
			flush()
			if p.ToolKind == cif.CIFToolKindCustom {
				rawInput := ""
				if p.RawInput != nil {
					rawInput = *p.RawInput
				}
				item := map[string]any{
					"type":    "custom_tool_call",
					"call_id": p.ToolCallID,
					"name":    p.ToolName,
					"input":   rawInput,
				}
				if p.Namespace != "" {
					item["namespace"] = p.Namespace
				}
				items = append(items, item)
				continue
			}
			argsBytes, _ := json.Marshal(p.ToolArguments)
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   p.ToolCallID,
				"name":      p.ToolName,
				"arguments": string(argsBytes),
			})
		}
	}
	flush()
	return items
}

func responsesImageURL(part cif.CIFImagePart) string {
	if part.Data != nil {
		return fmt.Sprintf("data:%s;base64,%s", part.MediaType, *part.Data)
	}
	if part.URL != nil {
		return *part.URL
	}
	return ""
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

func parseResponsesResponse(resp *copilotResponsesResponse) *cif.CanonicalResponse {
	if resp == nil {
		return &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	}
	result := &cif.CanonicalResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		StopReason: responsesStopReason(resp),
		Usage:      responsesUsageToCIF(resp.Usage),
	}
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, block := range item.Content {
				if block.Type != "output_text" && block.Type != "text" {
					continue
				}
				if block.Text == "" {
					continue
				}
				result.Content = append(result.Content, cif.CIFTextPart{Type: "text", Text: block.Text})
			}
		case "custom_tool_call":
			rawInput := item.Input
			result.Content = append(result.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(item.CallID, item.ID),
				ToolName:      item.Name,
				ToolArguments: map[string]any{"input": rawInput},
				ToolKind:      cif.CIFToolKindCustom,
				RawInput:      &rawInput,
				Namespace:     item.Namespace,
			})
		case "function_call":
			var args map[string]any
			json.Unmarshal([]byte(item.Arguments), &args) //nolint:errcheck
			if args == nil {
				args = map[string]any{}
			}
			result.Content = append(result.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(item.CallID, item.ID),
				ToolName:      item.Name,
				ToolArguments: args,
			})
		}
	}
	return result
}

func responsesStopReason(resp *copilotResponsesResponse) cif.CIFStopReason {
	if resp == nil {
		return cif.StopReasonEndTurn
	}
	for _, item := range resp.Output {
		if item.Type == "function_call" || item.Type == "custom_tool_call" {
			return cif.StopReasonToolUse
		}
	}
	if resp.IncompleteDetails != nil {
		switch resp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return cif.StopReasonMaxTokens
		case "content_filter":
			return cif.StopReasonContentFilter
		}
	}
	if strings.EqualFold(resp.Status, "incomplete") {
		return cif.StopReasonMaxTokens
	}
	return cif.StopReasonEndTurn
}

func responsesUsageToCIF(usage *copilotResponsesUsage) *cif.CIFUsage {
	if usage == nil {
		return nil
	}
	var cached *int
	if usage.InputTokensDetails != nil {
		value := usage.InputTokensDetails.CachedTokens
		cached = &value
	}
	return cif.UsageFromTotal(usage.InputTokens, usage.OutputTokens, cached)
}

// ---------------------------------------------------------------------------
// HTTP execution
// ---------------------------------------------------------------------------

func (a *CopilotAdapter) executeResponses(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeResponsesWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeResponsesWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (*cif.CanonicalResponse, error) {
	payload := a.buildResponsesPayload(request, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("copilot: marshal responses request: %w", err)
	}

	url := fmt.Sprintf("%s/responses", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound copilot responses request")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("copilot: create responses request: %w", err)
	}
	// Provide GetBody so the request can be retried on timeout.
	snapshot := body
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewBuffer(snapshot)), nil
	}
	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}

	started := time.Now()
	resp, err := copilotResponsesClient.Do(req)
	elapsed := time.Since(started)
	if debugEnabled() {
		logCopilotElapsed(a.provider.GetInstanceID(), "responses", request.Model, started, "Copilot upstream request completed")
	}
	if err != nil {
		a.logUpstreamTimeout(ctx, request, "responses", copilotResponsesBudget(), elapsed, err)
		// Retry once on timeout — but only for *early* timeouts (TLS/connect/
		// transient transport failures). If we burned the full client budget
		// the upstream is simply slow or stalled; retrying just doubles the
		// wall-clock before failover without improving the odds.
		if shouldRetryCopilotResponsesTimeout(err) {
			switch {
			case ctx.Err() != nil:
				// The caller gave up (client disconnect, upstream deadline).
				// A retry would reuse the same dead context and fail
				// instantly — pure noise.
				log.Debug().Err(err).Str("url", url).Dur("elapsed", elapsed).
					Msg("copilot: /responses aborted by caller context; not retrying")
			case worthRetryingAfter(elapsed, copilotResponsesBudget()):
				retryReq, rerr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(snapshot))
				if rerr == nil {
					for k, v := range a.requestHeaders(request) {
						retryReq.Header.Set(k, v)
					}
					log.Warn().Err(err).Str("url", url).Dur("elapsed", elapsed).Msg("copilot: retrying timed-out /responses request once")
					resp, err = copilotResponsesClient.Do(retryReq)
				}
			default:
				log.Warn().Err(err).Str("url", url).Dur("elapsed", elapsed).
					Msg("copilot: /responses exhausted its timeout budget upstream; failing over without retry")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("copilot: responses request failed: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: b}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("responses") {
			return a.executeResponsesWithRetry(ctx, request, false)
		}
		return nil, apiErr
	}

	var responsesResp copilotResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		return nil, fmt.Errorf("copilot: decode responses response: %w", err)
	}
	return parseResponsesResponse(&responsesResp), nil
}

func (a *CopilotAdapter) executeResponsesStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeResponsesStreamWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeResponsesStreamWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildResponsesPayload(request, true)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("copilot: marshal responses stream request: %w", err)
	}

	url := fmt.Sprintf("%s/responses", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound copilot responses stream request")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("copilot: create responses stream request: %w", err)
	}
	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := copilotStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot: responses stream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: b}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("responses-stream") {
			return a.executeResponsesStreamWithRetry(ctx, request, false)
		}
		return nil, apiErr
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseResponsesSSE(ctx, resp.Body, eventCh)
	return eventCh, nil
}

// worthRetryingAfter reports whether a timed-out request failed early enough
// that a retry is worth the latency.
//
// A timeout well inside the budget suggests a transient transport fault
// (connect/TLS/reset) that a second attempt may clear. A timeout at or near
// the full budget means the upstream accepted the connection and then stalled
// — retrying there just doubles wall-clock before failover, which is exactly
// how a 120s cap produced 240s of dead time before failing over.
func worthRetryingAfter(elapsed, budget time.Duration) bool {
	if budget <= 0 {
		return true // no budget configured; preserve retry-once behavior
	}
	return elapsed < budget/2
}

// shouldRetryCopilotResponsesTimeout returns true when a /responses request
// timed out and a single retry is warranted.
func shouldRetryCopilotResponsesTimeout(err error) bool {
	return isTimeoutError(err)
}

// ---------------------------------------------------------------------------
// Responses SSE parser
// ---------------------------------------------------------------------------

type responsesSSEState struct {
	streamStarted     bool
	nextContentIndex  int
	textBlockIndices  map[string]int
	textBlockHasDelta map[string]bool
	toolCallIndices   map[int]int
	toolCallHasDelta  map[int]bool
}

func parseResponsesSSE(ctx context.Context, body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)
	stop := context.AfterFunc(ctx, func() { body.Close() })
	defer stop()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)

	state := &responsesSSEState{
		textBlockIndices:  make(map[string]int),
		textBlockHasDelta: make(map[string]bool),
		toolCallIndices:   make(map[int]int),
		toolCallHasDelta:  make(map[int]bool),
	}

	var eventType string
	var dataLines []string

	flush := func() {
		if eventType == "" || len(dataLines) == 0 {
			eventType = ""
			dataLines = nil
			return
		}
		handleResponsesSSEEvent(eventType, strings.Join(dataLines, "\n"), state, eventCh)
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if val, ok := strings.CutPrefix(line, "event:"); ok {
			eventType = strings.TrimSpace(val)
			continue
		}
		if val, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimSpace(val))
		}
	}
	flush()

	if err := scanner.Err(); err != nil && !shared.IsBenignStreamEndError(ctx, err) {
		log.Error().Err(err).Str("provider", "copilot").Msg("responses SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
	}
}

//nolint:gocyclo,funlen
func handleResponsesSSEEvent(eventType, data string, state *responsesSSEState, eventCh chan cif.CIFStreamEvent) {
	if data == "" {
		return
	}
	switch eventType {
	case "response.created":
		var payload struct {
			Response *copilotResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			return
		}
		state.streamStarted = true
		eventCh <- cif.CIFStreamStart{
			Type:  "stream_start",
			ID:    payload.Response.ID,
			Model: payload.Response.Model,
		}

	case "response.output_item.added", "response.output_item.done":
		var payload struct {
			Item        copilotResponsesOutputItem `json:"item"`
			OutputIndex int                        `json:"output_index"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return
		}
		if payload.Item.Type != "function_call" && payload.Item.Type != "custom_tool_call" {
			return
		}
		isCustom := payload.Item.Type == "custom_tool_call"
		index, exists := state.toolCallIndices[payload.OutputIndex]
		if !exists {
			index = state.nextContentIndex
			state.nextContentIndex++
			state.toolCallIndices[payload.OutputIndex] = index
			contentBlock := cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(payload.Item.CallID, payload.Item.ID),
				ToolName:      payload.Item.Name,
				ToolArguments: map[string]any{},
			}
			var delta cif.DeltaContent = cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""}
			if isCustom {
				rawInput := payload.Item.Input
				contentBlock.ToolKind = cif.CIFToolKindCustom
				contentBlock.RawInput = &rawInput
				contentBlock.Namespace = payload.Item.Namespace
				contentBlock.ToolArguments = map[string]any{"input": rawInput}
				delta = cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: ""}
			}
			eventCh <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        index,
				ContentBlock: contentBlock,
				Delta:        delta,
			}
		}
		partial := payload.Item.Arguments
		if isCustom {
			partial = payload.Item.Input
		}
		if partial == "" || state.toolCallHasDelta[payload.OutputIndex] {
			return
		}
		var delta cif.DeltaContent = cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: partial}
		if isCustom {
			delta = cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: partial}
		}
		eventCh <- cif.CIFContentDelta{Type: "content_delta", Index: index, Delta: delta}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done":
		var payload struct {
			OutputIndex int    `json:"output_index"`
			Delta       string `json:"delta"`
			Input       string `json:"input"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return
		}
		index, ok := state.toolCallIndices[payload.OutputIndex]
		if !ok {
			return
		}
		partial := payload.Delta
		if partial == "" {
			partial = payload.Input
		}
		if partial == "" {
			return
		}
		if eventType == "response.custom_tool_call_input.done" && state.toolCallHasDelta[payload.OutputIndex] {
			return
		}
		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.CustomToolInputDelta{Type: "custom_tool_input_delta", Delta: partial},
		}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		var payload struct {
			OutputIndex int    `json:"output_index"`
			Delta       string `json:"delta"`
			Arguments   string `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return
		}
		index, ok := state.toolCallIndices[payload.OutputIndex]
		if !ok {
			return
		}
		partial := payload.Delta
		if partial == "" {
			partial = payload.Arguments
		}
		if partial == "" {
			return
		}
		if eventType == "response.function_call_arguments.done" && state.toolCallHasDelta[payload.OutputIndex] {
			return
		}
		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: partial},
		}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.output_text.delta", "response.output_text.done":
		var payload struct {
			OutputIndex  int    `json:"output_index"`
			ContentIndex int    `json:"content_index"`
			Delta        string `json:"delta"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return
		}
		index, isNew := ensureTextBlockIndex(state, payload.OutputIndex, payload.ContentIndex)
		key := textBlockKey(payload.OutputIndex, payload.ContentIndex)
		text := payload.Delta
		if text == "" {
			text = payload.Text
		}
		if text == "" {
			return
		}
		if eventType == "response.output_text.done" && state.textBlockHasDelta[key] {
			return
		}
		delta := cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.TextDelta{Type: "text_delta", Text: text},
		}
		if isNew {
			delta.ContentBlock = cif.CIFTextPart{Type: "text", Text: ""}
		}
		eventCh <- delta
		state.textBlockHasDelta[key] = true

	case "response.completed":
		var payload struct {
			Response *copilotResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			return
		}
		eventCh <- cif.CIFStreamEnd{
			Type:       "stream_end",
			StopReason: responsesStopReason(payload.Response),
			Usage:      responsesUsageToCIF(payload.Response.Usage),
		}

	case "error", "response.failed":
		var payload struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
			Response *struct {
				Error *struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return
		}
		errInfo := payload.Error
		if errInfo == nil && payload.Response != nil {
			errInfo = payload.Response.Error
		}
		if errInfo == nil {
			return
		}
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: firstNonEmpty(errInfo.Type, "stream_error"), Message: errInfo.Message},
		}
	}
}

func ensureTextBlockIndex(state *responsesSSEState, outputIndex, contentIndex int) (int, bool) {
	key := textBlockKey(outputIndex, contentIndex)
	if index, ok := state.textBlockIndices[key]; ok {
		return index, false
	}
	index := state.nextContentIndex
	state.nextContentIndex++
	state.textBlockIndices[key] = index
	return index, true
}

func textBlockKey(outputIndex, contentIndex int) string {
	return fmt.Sprintf("%d:%d", outputIndex, contentIndex)
}
