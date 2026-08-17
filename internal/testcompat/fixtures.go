package testcompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"omnillm/internal/cif"
)

const (
	Model                      = "compatibility-model"
	SystemPrompt               = "Preserve tool relationships across every dialect."
	InitialPrompt              = "Inspect README.md and report the result."
	AssistantPrelude           = "I will inspect it."
	FinalPrompt                = "Give the final answer."
	FinalAnswer                = "The compatibility fixture completed."
	LargeResultSize            = 128 * 1024
	ToolErrorText              = "synthetic tool failure sentinel"
	MinimumSequentialToolCalls = 5
	DroidCallID                = "call_apply_patch"
	DroidToolName              = "ApplyPatch"
	DroidRawInput              = "*** Update File: settings.json\n@@\n- old\n+ new"
	DroidToolResult            = "Error: synthetic patch failure"
)

type ToolExchange struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
	Result    string
	IsError   bool
	ToolKind  cif.CIFToolKind
	RawInput  *string
	Namespace string
}

type ClientCacheFixture struct {
	Name      string
	Endpoint  string
	Headers   map[string]string
	Request   json.RawMessage
	Exchanges []ToolExchange
}

type Scenario struct {
	Name      string
	Thinking  string
	Prelude   string
	Exchanges []ToolExchange
	FinalText string
}

func SemanticScenarios() []Scenario {
	return []Scenario{
		{Name: "plain", FinalText: FinalAnswer},
		{Name: "one-tool-cycle", Prelude: AssistantPrelude, Exchanges: []ToolExchange{exchange(1, `README content`)}},
		{Name: "five-sequential-tool-cycles", Prelude: AssistantPrelude, Exchanges: []ToolExchange{exchange(1, "first"), exchange(2, "second"), exchange(3, "third"), exchange(4, "fourth"), exchange(5, "fifth")}},
		{Name: "parallel-interleaved-calls", Prelude: "Checking both files.", Exchanges: []ToolExchange{exchange(1, "README"), exchange(2, "go.mod")}},
		{Name: "mixed-text-and-calls", Prelude: AssistantPrelude, Exchanges: []ToolExchange{exchange(1, "mixed")}, FinalText: FinalAnswer},
		{Name: "empty-arguments", Exchanges: []ToolExchange{{ID: "call_empty", Name: "List", Arguments: map[string]interface{}{}, Result: "[]"}}},
		{Name: "large-result", Exchanges: []ToolExchange{{ID: "call_large", Name: "Read", Arguments: map[string]interface{}{"file_path": "large.txt"}, Result: strings.Repeat("x", LargeResultSize)}}},
		{Name: "tool-error", Exchanges: []ToolExchange{{ID: "call_error", Name: "Read", Arguments: map[string]interface{}{"file_path": "missing.txt"}, Result: ToolErrorText, IsError: true}}},
		{Name: "thinking-before-tool", Thinking: "I should inspect the file first.", Exchanges: []ToolExchange{exchange(1, "thinking")}},
		{Name: "normal-completion", FinalText: FinalAnswer},
		{Name: "abrupt-failure"},
		{Name: "cancellation"},
	}
}

func AgenticScenario() Scenario {
	for _, scenario := range SemanticScenarios() {
		if scenario.Name == "five-sequential-tool-cycles" {
			return scenario
		}
	}
	panic("five-sequential-tool-cycles scenario is missing")
}

// ClientCacheFixtures models the native API histories submitted by each
// maintained coding-agent client. Every fixture includes five observed calls,
// five associated results, and the terminal continuation prompt; a provider
// response to the request must therefore contain no sixth call.
func ClientCacheFixtures() []ClientCacheFixture {
	scenario := AgenticScenario()
	codexExchanges := customToolExchanges("exec", "functions", "command")
	droidExchanges := customToolExchanges(DroidToolName, "", "patch")
	return []ClientCacheFixture{
		{
			Name:      "claude-code",
			Endpoint:  "/v1/messages",
			Headers:   map[string]string{"anthropic-version": "2023-06-01"},
			Request:   AnthropicRequest(scenario, false),
			Exchanges: scenario.Exchanges,
		},
		{
			Name:      "github-copilot-cli-custom-provider",
			Endpoint:  "/v1/chat/completions",
			Request:   ChatCompletionsRequest(scenario, false),
			Exchanges: scenario.Exchanges,
		},
		{
			Name:      "codex-cli",
			Endpoint:  "/v1/responses",
			Request:   customToolResponsesRequest(codexExchanges, false),
			Exchanges: codexExchanges,
		},
		{
			Name:      "droid",
			Endpoint:  "/v1/responses",
			Request:   customToolResponsesRequest(droidExchanges, false),
			Exchanges: droidExchanges,
		},
	}
}

func customToolExchanges(name, namespace, inputLabel string) []ToolExchange {
	exchanges := make([]ToolExchange, 0, MinimumSequentialToolCalls)
	for index := 1; index <= MinimumSequentialToolCalls; index++ {
		rawInput := fmt.Sprintf("%s-%d\nline-%d", inputLabel, index, index)
		exchanges = append(exchanges, ToolExchange{
			ID:        fmt.Sprintf("call_custom_%d", index),
			Name:      name,
			Arguments: map[string]interface{}{"input": rawInput},
			Result:    fmt.Sprintf("result-%d", index),
			ToolKind:  cif.CIFToolKindCustom,
			RawInput:  &rawInput,
			Namespace: namespace,
		})
	}
	return exchanges
}

func customToolResponsesRequest(exchanges []ToolExchange, stream bool) json.RawMessage {
	first := exchanges[0]
	input := []interface{}{map[string]interface{}{"type": "message", "role": "user", "content": InitialPrompt}}
	for _, exchange := range exchanges {
		call := map[string]interface{}{
			"type":    "custom_tool_call",
			"call_id": exchange.ID,
			"name":    exchange.Name,
			"input":   *exchange.RawInput,
		}
		if exchange.Namespace != "" {
			call["namespace"] = exchange.Namespace
		}
		input = append(input,
			call,
			map[string]interface{}{"type": "custom_tool_call_output", "call_id": exchange.ID, "output": exchange.Result},
		)
	}
	input = append(input, map[string]interface{}{"type": "message", "role": "user", "content": FinalPrompt})
	tool := map[string]interface{}{
		"type":        "custom",
		"name":        first.Name,
		"description": "Execute a native custom tool",
		"format":      map[string]interface{}{"type": "text"},
	}
	if first.Namespace != "" {
		tool["namespace"] = first.Namespace
	}
	return marshal(map[string]interface{}{
		"model":  Model,
		"stream": stream,
		"input":  input,
		"tools":  []interface{}{tool},
	})
}

func exchange(index int, result string) ToolExchange {
	return ToolExchange{
		ID:        "call_" + string(rune('0'+index)),
		Name:      "Read",
		Arguments: map[string]interface{}{"file_path": "file" + string(rune('0'+index)) + ".txt"},
		Result:    result,
	}
}

func CanonicalMessages(s Scenario) []cif.CIFMessage {
	messages := []cif.CIFMessage{cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: InitialPrompt}}}}
	if s.Name == "parallel-interleaved-calls" {
		parts := make([]cif.CIFContentPart, 0, len(s.Exchanges)+1)
		if s.Prelude != "" {
			parts = append(parts, cif.CIFTextPart{Type: "text", Text: s.Prelude})
		}
		results := make([]cif.CIFContentPart, 0, len(s.Exchanges))
		for _, exchange := range s.Exchanges {
			parts = append(parts, cif.CIFToolCallPart{Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name, ToolArguments: exchange.Arguments})
			results = append(results, cif.CIFToolResultPart{Type: "tool_result", ToolCallID: exchange.ID, ToolName: exchange.Name, Content: exchange.Result, IsError: boolPointer(exchange.IsError)})
		}
		messages = append(messages, cif.CIFAssistantMessage{Role: "assistant", Content: parts}, cif.CIFUserMessage{Role: "user", Content: results})
	} else {
		for _, exchange := range s.Exchanges {
			parts := make([]cif.CIFContentPart, 0, 3)
			if s.Thinking != "" {
				parts = append(parts, cif.CIFThinkingPart{Type: "thinking", Thinking: s.Thinking})
			}
			if s.Prelude != "" {
				parts = append(parts, cif.CIFTextPart{Type: "text", Text: s.Prelude})
			}
			parts = append(parts, cif.CIFToolCallPart{Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name, ToolArguments: exchange.Arguments})
			messages = append(messages,
				cif.CIFAssistantMessage{Role: "assistant", Content: parts},
				cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFToolResultPart{Type: "tool_result", ToolCallID: exchange.ID, ToolName: exchange.Name, Content: exchange.Result, IsError: boolPointer(exchange.IsError)}}},
			)
		}
	}
	messages = append(messages, cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: FinalPrompt}}})
	return messages
}

func ChatCompletionsRequest(s Scenario, stream bool) json.RawMessage {
	messages := []interface{}{map[string]interface{}{"role": "user", "content": InitialPrompt}}
	for _, x := range s.Exchanges {
		args, _ := json.Marshal(x.Arguments)
		assistant := map[string]interface{}{"role": "assistant", "content": s.Prelude, "tool_calls": []interface{}{map[string]interface{}{"id": x.ID, "type": "function", "function": map[string]interface{}{"name": x.Name, "arguments": string(args)}}}}
		messages = append(messages, assistant, map[string]interface{}{"role": "tool", "tool_call_id": x.ID, "content": x.Result})
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": FinalPrompt})
	return marshal(map[string]interface{}{"model": Model, "stream": stream, "messages": messages, "tools": chatTools()})
}

func AnthropicRequest(s Scenario, stream bool) json.RawMessage {
	messages := []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "text", "text": InitialPrompt}}}}
	for _, x := range s.Exchanges {
		assistantContent := []interface{}{}
		if s.Thinking != "" {
			assistantContent = append(assistantContent, map[string]interface{}{"type": "thinking", "thinking": s.Thinking})
		}
		if s.Prelude != "" {
			assistantContent = append(assistantContent, map[string]interface{}{"type": "text", "text": s.Prelude})
		}
		assistantContent = append(assistantContent, map[string]interface{}{"type": "tool_use", "id": x.ID, "name": x.Name, "input": x.Arguments})
		messages = append(messages,
			map[string]interface{}{"role": "assistant", "content": assistantContent},
			map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "tool_result", "tool_use_id": x.ID, "content": x.Result, "is_error": x.IsError}}},
		)
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "text", "text": FinalPrompt}}})
	return marshal(map[string]interface{}{"model": Model, "max_tokens": 256, "stream": stream, "system": SystemPrompt, "messages": messages, "tools": []interface{}{map[string]interface{}{"name": "Read", "description": "Read a file", "input_schema": schema()}}})
}

func StructuredResponsesOutput(index int) []interface{} {
	return []interface{}{
		map[string]interface{}{"type": "input_text", "text": fmt.Sprintf("result-%d", index)},
	}
}

func ResponsesRequest(s Scenario, stream bool) json.RawMessage {
	return responsesRequest(s, stream, false)
}

func StructuredResponsesRequest(s Scenario, stream bool) json.RawMessage {
	return responsesRequest(s, stream, true)
}

func responsesRequest(s Scenario, stream, structuredOutputs bool) json.RawMessage {
	input := []interface{}{map[string]interface{}{"type": "message", "role": "user", "content": []interface{}{map[string]interface{}{"type": "input_text", "text": InitialPrompt}}}}
	for index, x := range s.Exchanges {
		args, _ := json.Marshal(x.Arguments)
		if s.Prelude != "" {
			input = append(input, map[string]interface{}{"type": "message", "role": "assistant", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": s.Prelude}}})
		}
		output := interface{}(x.Result)
		if structuredOutputs {
			output = StructuredResponsesOutput(index + 1)
		}
		input = append(input,
			map[string]interface{}{"type": "function_call", "id": x.ID, "call_id": x.ID, "name": x.Name, "arguments": string(args)},
			map[string]interface{}{"type": "function_call_output", "call_id": x.ID, "output": output},
		)
	}
	input = append(input, map[string]interface{}{"type": "message", "role": "user", "content": []interface{}{map[string]interface{}{"type": "input_text", "text": FinalPrompt}}})
	return marshal(map[string]interface{}{"model": Model, "stream": stream, "instructions": SystemPrompt, "input": input, "tools": []interface{}{map[string]interface{}{"type": "function", "name": "Read", "description": "Read a file", "parameters": schema()}}})
}

func DroidCustomToolResponsesRequest(stream bool) json.RawMessage {
	input := []interface{}{
		map[string]interface{}{"type": "message", "role": "user", "content": InitialPrompt},
		map[string]interface{}{"type": "message", "role": "assistant", "content": AssistantPrelude},
		map[string]interface{}{"type": "custom_tool_call", "call_id": DroidCallID, "name": DroidToolName, "input": DroidRawInput},
		map[string]interface{}{"type": "custom_tool_call_output", "call_id": DroidCallID, "output": DroidToolResult},
		map[string]interface{}{"type": "message", "role": "user", "content": FinalPrompt},
	}
	tool := map[string]interface{}{
		"type":        "custom",
		"name":        DroidToolName,
		"description": "Apply a patch",
		"format":      map[string]interface{}{"type": "text"},
	}
	return marshal(map[string]interface{}{
		"model":  Model,
		"stream": stream,
		"input":  input,
		"tools":  []interface{}{tool},
	})
}

func Response(s Scenario) *cif.CanonicalResponse {
	content := []cif.CIFContentPart{}
	if s.Thinking != "" {
		content = append(content, cif.CIFThinkingPart{Type: "thinking", Thinking: s.Thinking})
	}
	if s.Prelude != "" {
		content = append(content, cif.CIFTextPart{Type: "text", Text: s.Prelude})
	}
	for _, x := range s.Exchanges {
		content = append(content, cif.CIFToolCallPart{Type: "tool_call", ToolCallID: x.ID, ToolName: x.Name, ToolArguments: x.Arguments})
	}
	if s.FinalText != "" {
		content = append(content, cif.CIFTextPart{Type: "text", Text: s.FinalText})
	}
	stop := cif.StopReasonEndTurn
	if len(s.Exchanges) > 0 {
		stop = cif.StopReasonToolUse
	}
	if s.Name == "abrupt-failure" {
		stop = cif.StopReasonError
	}
	return &cif.CanonicalResponse{ID: "compat_response", Model: Model, Content: content, StopReason: stop, Usage: &cif.CIFUsage{InputTokens: 21, OutputTokens: 13}}
}

func StreamEvents(s Scenario) []cif.CIFStreamEvent {
	events := []cif.CIFStreamEvent{cif.CIFStreamStart{Type: "stream_start", ID: "compat_stream", Model: Model}}
	index := 0
	if s.Thinking != "" {
		events = append(events, cif.CIFContentDelta{Type: "content_delta", Index: index, ContentBlock: cif.CIFThinkingPart{Type: "thinking"}, Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: s.Thinking}}, cif.CIFContentBlockStop{Type: "content_block_stop", Index: index})
		index++
	}
	if s.Prelude != "" {
		events = append(events, cif.CIFContentDelta{Type: "content_delta", Index: index, ContentBlock: cif.CIFTextPart{Type: "text"}, Delta: cif.TextDelta{Type: "text_delta", Text: s.Prelude}}, cif.CIFContentBlockStop{Type: "content_block_stop", Index: index})
		index++
	}

	if s.Name == "parallel-interleaved-calls" {
		arguments := make([][]byte, len(s.Exchanges))
		cuts := make([]int, len(s.Exchanges))
		for offset, exchange := range s.Exchanges {
			arguments[offset], _ = json.Marshal(exchange.Arguments)
			cuts[offset] = len(arguments[offset]) / 2
			events = append(events, cif.CIFContentDelta{
				Type: "content_delta", Index: index + offset,
				ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name, ToolArguments: map[string]interface{}{}},
				Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(arguments[offset][:cuts[offset]])},
			})
		}
		for offset := range s.Exchanges {
			events = append(events, cif.CIFContentDelta{Type: "content_delta", Index: index + offset, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(arguments[offset][cuts[offset]:])}})
		}
		for offset := range s.Exchanges {
			events = append(events, cif.CIFContentBlockStop{Type: "content_block_stop", Index: index + offset})
		}
	} else {
		for offset, exchange := range s.Exchanges {
			args, _ := json.Marshal(exchange.Arguments)
			cut := len(args) / 2
			events = append(events,
				cif.CIFContentDelta{Type: "content_delta", Index: index + offset, ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name, ToolArguments: map[string]interface{}{}}, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(args[:cut])}},
				cif.CIFContentDelta{Type: "content_delta", Index: index + offset, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(args[cut:])}},
				cif.CIFContentBlockStop{Type: "content_block_stop", Index: index + offset},
			)
		}
	}
	if s.Name == "abrupt-failure" {
		return append(events, cif.CIFStreamError{Type: "stream_error", Error: cif.ErrorInfo{Type: "upstream_error", Message: "synthetic abrupt failure"}})
	}
	return append(events, cif.CIFStreamEnd{Type: "stream_end", StopReason: Response(s).StopReason, Usage: &cif.CIFUsage{InputTokens: 21, OutputTokens: 13}})
}

func schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{"file_path": map[string]interface{}{"type": "string"}}}
}
func chatTools() []interface{} {
	return []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "Read", "description": "Read a file", "parameters": schema()}}}
}
func marshal(value interface{}) json.RawMessage { data, _ := json.Marshal(value); return data }
func boolPointer(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}
