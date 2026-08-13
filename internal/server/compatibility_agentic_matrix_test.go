package server

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"omnillm/internal/cif"
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
	want := []testcompat.ToolExchange{{
		ID:        testcompat.DroidCallID,
		Name:      testcompat.DroidToolName,
		Arguments: map[string]interface{}{"input": testcompat.DroidRawInput},
		Result:    testcompat.DroidToolResult,
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

func assertCompatibilityExchanges(t *testing.T, messages []cif.CIFMessage, want []testcompat.ToolExchange) {
	t.Helper()
	calls := make(map[string]cif.CIFToolCallPart)
	results := make(map[string]cif.CIFToolResultPart)
	for _, message := range messages {
		switch typed := message.(type) {
		case cif.CIFAssistantMessage:
			for _, part := range typed.Content {
				if call, ok := part.(cif.CIFToolCallPart); ok {
					calls[call.ToolCallID] = call
				}
			}
		case cif.CIFUserMessage:
			for _, part := range typed.Content {
				if result, ok := part.(cif.CIFToolResultPart); ok {
					results[result.ToolCallID] = result
				}
			}
		}
	}
	if len(calls) != len(want) || len(results) != len(want) {
		t.Fatalf("calls=%d results=%d want=%d", len(calls), len(results), len(want))
	}
	for _, exchange := range want {
		call := calls[exchange.ID]
		result := results[exchange.ID]
		if call.ToolName != exchange.Name || !reflect.DeepEqual(call.ToolArguments, exchange.Arguments) || result.Content != exchange.Result {
			t.Errorf("exchange %s lost relationship: call=%#v result=%#v", exchange.ID, call, result)
		}
	}
}
