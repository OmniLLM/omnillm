package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/lib/responsecache"
	"omnillm/internal/testcompat"
)

func TestCompatibilityDroidCustomToolRoute(t *testing.T) {
	var captured *cif.CanonicalRequest
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		captured = request
		return &cif.CanonicalResponse{
			ID:         "droid-custom-route",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/v1/responses", string(testcompat.DroidCustomToolResponsesRequest(false)), nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if captured == nil {
		t.Fatal("provider did not receive custom-tool request")
	}
	rawInput := testcompat.DroidRawInput
	want := []testcompat.ToolExchange{{
		ID:        testcompat.DroidCallID,
		Name:      testcompat.DroidToolName,
		Arguments: map[string]interface{}{"input": testcompat.DroidRawInput},
		Result:    testcompat.DroidToolResult,
		ToolKind:  cif.CIFToolKindCustom,
		RawInput:  &rawInput,
	}}
	assertCompatibilityExchanges(t, captured.Messages, want)
	if got := captured.Tools[0].ParametersSchema["required"]; !reflect.DeepEqual(got, []interface{}{"input"}) {
		t.Fatalf("custom tool required fields = %#v", got)
	}
}

func TestCompatibilityDroidCustomToolRouteRejectsMalformedItem(t *testing.T) {
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		t.Fatalf("provider received malformed request: %#v", request)
		return nil, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	body := `{"model":"compatibility-model","input":[{"type":"custom_tool_call","call_id":"call_bad","name":"ApplyPatch"}]}`
	resp := postJSON(t, srv.URL+"/v1/responses", body, nil)
	responseBody := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d: %s", resp.StatusCode, responseBody)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	errorObject, ok := envelope["error"].(map[string]interface{})
	if !ok || errorObject["type"] != "invalid_request_error" {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

// TestCompatibilityClientCacheReplayMatrix extends the maintained client-shape
// suite rather than duplicating route-specific cache tests. Each native history
// has already observed five calls/results. The first request populates the exact
// response cache; the byte-identical second request must replay the terminal
// response without invoking the provider and without returning a sixth call.
func TestCompatibilityClientCacheReplayMatrix(t *testing.T) {
	enableResponseCacheForTest(t)
	store := newMemoryResponseCacheStore()
	restore := responsecache.ConfigureStore(store)
	defer restore()

	upstreamCalls := 0
	var expectedExchanges []testcompat.ToolExchange
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		upstreamCalls++
		assertCompatibilityExchanges(t, request.Messages, expectedExchanges)
		return &cif.CanonicalResponse{
			ID:         "compat-cache-terminal",
			Model:      request.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	for _, fixture := range testcompat.ClientCacheFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			expectedExchanges = fixture.Exchanges
			before := upstreamCalls
			for requestNumber, wantCache := range []string{"miss", "hit"} {
				response := postJSON(t, srv.URL+fixture.Endpoint, string(fixture.Request), fixture.Headers)
				body := readBody(t, response)
				if response.StatusCode != http.StatusOK {
					t.Fatalf("request %d status %d: %s", requestNumber+1, response.StatusCode, body)
				}
				if got := response.Header.Get("X-OmniLLM-Cache"); got != wantCache {
					t.Fatalf("request %d cache = %q, want %q", requestNumber+1, got, wantCache)
				}
				if !strings.Contains(body, testcompat.FinalAnswer) {
					t.Fatalf("request %d terminal continuation missing: %s", requestNumber+1, body)
				}
				assertNoCompatibilityToolCall(t, fixture.Endpoint, body)
			}
			if got := upstreamCalls - before; got != 1 {
				t.Fatalf("upstream calls = %d, want 1", got)
			}
		})
	}
}

func assertNoCompatibilityToolCall(t *testing.T, endpoint, body string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if endpoint == "/v1/messages" {
		content, ok := payload["content"].([]interface{})
		if !ok {
			t.Fatalf("messages response missing content: %s", body)
		}
		for _, raw := range content {
			if block, ok := raw.(map[string]interface{}); ok && block["type"] == "tool_use" {
				t.Fatalf("unexpected sixth tool call: %s", body)
			}
		}
		return
	}
	if endpoint == "/v1/responses" {
		output, ok := payload["output"].([]interface{})
		if !ok {
			t.Fatalf("Responses response missing output: %s", body)
		}
		for _, raw := range output {
			if item, ok := raw.(map[string]interface{}); ok && (item["type"] == "function_call" || item["type"] == "custom_tool_call") {
				t.Fatalf("unexpected sixth tool call: %s", body)
			}
		}
		return
	}
	choices, ok := payload["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatalf("chat response missing choices: %s", body)
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		t.Fatalf("chat response malformed choice: %s", body)
	}
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("chat response missing message: %s", body)
	}
	if calls, ok := message["tool_calls"].([]interface{}); ok && len(calls) > 0 {
		t.Fatalf("unexpected sixth tool call: %s", body)
	}
}

func TestCompatibilityCachedResponsesToolFidelity(t *testing.T) {
	enableResponseCacheForTest(t)
	store := newMemoryResponseCacheStore()
	restore := responsecache.ConfigureStore(store)
	defer restore()

	customRawInput := "printf 'cached custom'\nprintf 'second line'"
	structuredOutput := testcompat.StructuredResponsesOutput(1)
	responses := []*cif.CanonicalResponse{
		{
			ID:    "cached-custom-response",
			Model: testcompat.Model,
			Content: []cif.CIFContentPart{
				cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    "call_cached_custom",
					ToolName:      "exec",
					ToolArguments: map[string]interface{}{"input": customRawInput},
					ToolKind:      cif.CIFToolKindCustom,
					RawInput:      &customRawInput,
					Namespace:     "functions",
				},
				cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    "call_cached_function",
					ToolName:      "Read",
					ToolArguments: map[string]interface{}{"file_path": "README.md"},
				},
			},
			StopReason: cif.StopReasonToolUse,
		},
		{
			ID:         "cached-terminal-response",
			Model:      testcompat.Model,
			Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}},
			StopReason: cif.StopReasonEndTurn,
		},
	}
	upstreamCalls := 0
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		response := responses[upstreamCalls]
		upstreamCalls++
		if upstreamCalls == 2 {
			assertCachedResponsesContinuation(t, request.Messages, structuredOutput, customRawInput)
		}
		return response, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	initialRequest := `{"model":"compatibility-model","input":"run both tools","tools":[{"type":"custom","name":"exec","namespace":"functions","format":{"type":"text"}},{"type":"function","name":"Read","parameters":{"type":"object"}}]}`
	var replayed map[string]interface{}
	for index, wantCache := range []string{"miss", "hit"} {
		response := postJSON(t, srv.URL+"/v1/responses", initialRequest, nil)
		body := readBody(t, response)
		if response.StatusCode != http.StatusOK || response.Header.Get("X-OmniLLM-Cache") != wantCache {
			t.Fatalf("request %d status=%d cache=%q body=%s", index+1, response.StatusCode, response.Header.Get("X-OmniLLM-Cache"), body)
		}
		if err := json.Unmarshal([]byte(body), &replayed); err != nil {
			t.Fatalf("decode cached Responses replay: %v", err)
		}
	}
	assertCachedResponsesOutput(t, replayed, customRawInput)
	if upstreamCalls != 1 {
		t.Fatalf("initial cached replay upstream calls = %d, want 1", upstreamCalls)
	}

	continuation := fmt.Sprintf(`{"model":"compatibility-model","input":[{"type":"custom_tool_call","call_id":"call_cached_custom","name":"exec","namespace":"functions","input":%q},{"type":"custom_tool_call_output","call_id":"call_cached_custom","output":"custom-ok"},{"type":"function_call","call_id":"call_cached_function","name":"Read","arguments":"{\"file_path\":\"README.md\"}"},{"type":"function_call_output","call_id":"call_cached_function","output":%s},{"type":"message","role":"user","content":"continue"}]}`, customRawInput, mustMarshalJSON(t, structuredOutput))
	for index, wantCache := range []string{"miss", "hit"} {
		response := postJSON(t, srv.URL+"/v1/responses", continuation, nil)
		body := readBody(t, response)
		if response.StatusCode != http.StatusOK || response.Header.Get("X-OmniLLM-Cache") != wantCache || !strings.Contains(body, testcompat.FinalAnswer) {
			t.Fatalf("continuation %d status=%d cache=%q body=%s", index+1, response.StatusCode, response.Header.Get("X-OmniLLM-Cache"), body)
		}
	}
	if upstreamCalls != 2 {
		t.Fatalf("continuation cached replay upstream calls = %d, want 2", upstreamCalls)
	}

	streamRequest := strings.Replace(initialRequest, `"input":"run both tools"`, `"stream":true,"input":"run both tools"`, 1)
	stream := postJSON(t, srv.URL+"/v1/responses", streamRequest, nil)
	streamBody := readBody(t, stream)
	if stream.StatusCode != http.StatusOK || stream.Header.Get("X-OmniLLM-Cache") != "hit" {
		t.Fatalf("stream status=%d cache=%q body=%s", stream.StatusCode, stream.Header.Get("X-OmniLLM-Cache"), streamBody)
	}
	for _, vocabulary := range []string{
		"response.custom_tool_call_input.delta",
		"response.custom_tool_call_input.done",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
	} {
		if !strings.Contains(streamBody, vocabulary) {
			t.Errorf("cached stream missing %q: %s", vocabulary, streamBody)
		}
	}
	if got := strings.Count(streamBody, `"type":"response.completed"`); got != 1 {
		t.Fatalf("cached stream terminal count = %d, want 1: %s", got, streamBody)
	}
}

func assertCachedResponsesOutput(t *testing.T, payload map[string]interface{}, rawInput string) {
	t.Helper()
	output, ok := payload["output"].([]interface{})
	if !ok || len(output) != 2 {
		t.Fatalf("cached Responses output = %#v", payload["output"])
	}
	custom, customOK := output[0].(map[string]interface{})
	function, functionOK := output[1].(map[string]interface{})
	if !customOK || !functionOK {
		t.Fatalf("cached Responses items malformed: %#v", output)
	}
	if custom["type"] != "custom_tool_call" || custom["call_id"] != "call_cached_custom" || custom["name"] != "exec" || custom["namespace"] != "functions" || custom["input"] != rawInput {
		t.Fatalf("cached custom tool changed: %#v", custom)
	}
	if function["type"] != "function_call" || function["call_id"] != "call_cached_function" || function["name"] != "Read" || function["arguments"] != `{"file_path":"README.md"}` {
		t.Fatalf("cached function tool changed: %#v", function)
	}
}

func assertCachedResponsesContinuation(t *testing.T, messages []cif.CIFMessage, structuredOutput []interface{}, customRawInput string) {
	t.Helper()
	var calls []cif.CIFToolCallPart
	var results []cif.CIFToolResultPart
	for _, message := range messages {
		switch typed := message.(type) {
		case cif.CIFAssistantMessage:
			for _, part := range typed.Content {
				if call, ok := part.(cif.CIFToolCallPart); ok {
					calls = append(calls, call)
				}
			}
		case cif.CIFUserMessage:
			for _, part := range typed.Content {
				if result, ok := part.(cif.CIFToolResultPart); ok {
					results = append(results, result)
				}
			}
		}
	}
	if len(calls) != 2 || len(results) != 2 {
		t.Fatalf("continuation calls=%d results=%d", len(calls), len(results))
	}
	if calls[0].ToolKind != cif.CIFToolKindCustom || calls[0].RawInput == nil || *calls[0].RawInput != customRawInput || calls[0].Namespace != "functions" || calls[0].ToolCallID != results[0].ToolCallID || results[0].ToolKind != cif.CIFToolKindCustom || results[0].CustomOutput != "custom-ok" {
		t.Fatalf("custom continuation changed: call=%#v result=%#v", calls[0], results[0])
	}
	if calls[1].ToolCallID != results[1].ToolCallID || !reflect.DeepEqual(results[1].RawOutput, structuredOutput) {
		t.Fatalf("structured function continuation changed: call=%#v result=%#v", calls[1], results[1])
	}
}

func mustMarshalJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(encoded)
}

func TestCompatibilityAgenticRouteMatrix(t *testing.T) {
	scenario := testcompat.AgenticScenario()
	var captured []*cif.CanonicalRequest
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		captured = append(captured, request)
		return &cif.CanonicalResponse{ID: "agentic-route", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	rows := []struct {
		name     string
		endpoint string
		body     json.RawMessage
		headers  map[string]string
	}{
		{"chat-root", "/chat/completions", testcompat.ChatCompletionsRequest(scenario, false), nil},
		{"chat-v1", "/v1/chat/completions", testcompat.ChatCompletionsRequest(scenario, false), nil},
		{"anthropic", "/v1/messages", testcompat.AnthropicRequest(scenario, false), map[string]string{"anthropic-version": "2023-06-01"}},
		{"responses", "/v1/responses", testcompat.ResponsesRequest(scenario, false), nil},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+row.endpoint, string(row.body), row.headers)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			if len(captured) == 0 {
				t.Fatal("provider did not receive request")
			}
			request := captured[len(captured)-1]
			if request.Model != testcompat.Model {
				t.Fatalf("model = %q", request.Model)
			}
			assertCompatibilityExchanges(t, request.Messages, scenario.Exchanges)
		})
	}
}

func TestCompatibilityStructuredResponsesRoute(t *testing.T) {
	scenario := testcompat.AgenticScenario()
	var captured *cif.CanonicalRequest
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		captured = request
		return &cif.CanonicalResponse{ID: "structured-responses-route", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/v1/responses", string(testcompat.StructuredResponsesRequest(scenario, false)), nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if captured == nil {
		t.Fatal("provider did not receive structured-output request")
	}

	calls := 0
	results := 0
	nextExchange := 0
	for _, message := range captured.Messages {
		switch typed := message.(type) {
		case cif.CIFAssistantMessage:
			for _, part := range typed.Content {
				call, ok := part.(cif.CIFToolCallPart)
				if !ok {
					continue
				}
				if nextExchange >= len(scenario.Exchanges) || call.ToolCallID != scenario.Exchanges[nextExchange].ID {
					t.Fatalf("tool call order changed at %d: %#v", nextExchange, call)
				}
				calls++
			}
		case cif.CIFUserMessage:
			for _, part := range typed.Content {
				result, ok := part.(cif.CIFToolResultPart)
				if !ok {
					continue
				}
				if nextExchange >= len(scenario.Exchanges) || result.ToolCallID != scenario.Exchanges[nextExchange].ID {
					t.Fatalf("tool result order changed at %d: %#v", nextExchange, result)
				}
				wantOutput := testcompat.StructuredResponsesOutput(nextExchange + 1)
				if !reflect.DeepEqual(result.RawOutput, wantOutput) {
					t.Fatalf("structured result %s changed: %#v", result.ToolCallID, result.RawOutput)
				}
				encoded, _ := json.Marshal(wantOutput)
				if result.Content != string(encoded) {
					t.Fatalf("structured fallback %s changed: %q", result.ToolCallID, result.Content)
				}
				results++
				nextExchange++
			}
		}
	}
	if calls != testcompat.MinimumSequentialToolCalls || results != testcompat.MinimumSequentialToolCalls || nextExchange != len(scenario.Exchanges) {
		t.Fatalf("calls=%d results=%d exchanges=%d", calls, results, nextExchange)
	}
	if !strings.Contains(body, testcompat.FinalAnswer) {
		t.Fatalf("terminal response missing: %s", body)
	}
}

func TestCompatibilityStructuredResponsesRouteRejectsMalformedOutput(t *testing.T) {
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		t.Fatalf("provider received malformed request: %#v", request)
		return nil, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	body := `{"model":"compatibility-model","input":[{"type":"function_call_output","call_id":"call_bad","output":[{"type":"input_text"}]}]}`
	resp := postJSON(t, srv.URL+"/v1/responses", body, nil)
	responseBody := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d: %s", resp.StatusCode, responseBody)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	errorObject, ok := envelope["error"].(map[string]interface{})
	if !ok || errorObject["type"] != "invalid_request_error" {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

func assertCompatibilityExchanges(t *testing.T, messages []cif.CIFMessage, want []testcompat.ToolExchange) {
	t.Helper()
	var calls []cif.CIFToolCallPart
	var results []cif.CIFToolResultPart
	for _, message := range messages {
		switch typed := message.(type) {
		case cif.CIFAssistantMessage:
			for _, part := range typed.Content {
				if call, ok := part.(cif.CIFToolCallPart); ok {
					calls = append(calls, call)
				}
			}
		case cif.CIFUserMessage:
			for _, part := range typed.Content {
				if result, ok := part.(cif.CIFToolResultPart); ok {
					results = append(results, result)
				}
			}
		}
	}
	if len(calls) != len(want) || len(results) != len(want) {
		t.Fatalf("calls=%d results=%d want=%d", len(calls), len(results), len(want))
	}
	for index, exchange := range want {
		call := calls[index]
		result := results[index]
		if call.ToolCallID != exchange.ID || call.ToolName != exchange.Name || !reflect.DeepEqual(call.ToolArguments, exchange.Arguments) || call.ToolKind != exchange.ToolKind || !equalOptionalString(call.RawInput, exchange.RawInput) || call.Namespace != exchange.Namespace || result.ToolCallID != exchange.ID || result.Content != exchange.Result || result.ToolKind != exchange.ToolKind {
			t.Errorf("exchange %d (%s) lost order or relationship: call=%#v result=%#v", index, exchange.ID, call, result)
		}
	}
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
