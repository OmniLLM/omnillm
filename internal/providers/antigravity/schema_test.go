package antigravity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/providers/shared"
)

// Reproduces the untyped nested union in Claude Code's SendMessage schema.
const nestedToolSchema = `{
	"type":"object",
	"properties":{
		"recipient":{"type":"string","minLength":1},
		"message":{"anyOf":[
			{"type":"string"},
			{"anyOf":[
				{"type":"object","properties":{"type":{"const":"notify"},"id":{"$ref":"#/$defs/id"}},
				 "required":["type","id"],"additionalProperties":false},
				{"type":"object","properties":{"type":{"const":"ack"},"allowed":{"type":"boolean"}},
				 "required":["type","allowed"],"additionalProperties":false}
			]}
		]},
		"const":{"type":"string"},
		"unused":false
	},
	"$defs":{"id":{"type":"string","pattern":"^[a-z]+$"}},
	"required":["recipient","message"],
	"additionalProperties":false
}`

func schemaRequest(t *testing.T, shape, model string) *cif.CanonicalRequest {
	t.Helper()
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(nestedToolSchema), &schema); err != nil {
		t.Fatal(err)
	}
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{{
			"role": "user", "content": "Inspect the event.",
		}},
		"max_tokens": 128,
	}
	tool := map[string]interface{}{"name": "dispatch_event", "description": "Dispatch an event."}
	parse := ingestion.ParseAnthropicMessages
	switch shape {
	case "messages":
		tool["input_schema"] = schema
		body["tools"] = []interface{}{tool}
	case "chat":
		parse = ingestion.ParseOpenAIChatCompletions
		tool["parameters"] = schema
		body["tools"] = []interface{}{map[string]interface{}{"type": "function", "function": tool}}
	case "responses":
		parse = ingestion.ParseResponsesPayload
		tool["type"] = "function"
		tool["parameters"] = schema
		body["tools"] = []interface{}{tool}
		body["input"] = "Inspect the event."
		delete(body, "messages")
	default:
		t.Fatalf("unknown shape %q", shape)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestClaudeToolSchemaTransportAcrossIngress(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-6-thinking"} {
		for _, shape := range []string{"messages", "chat", "responses"} {
			for _, mode := range []string{"streaming", "collected"} {
				t.Run(model+"/"+shape+"/"+mode, func(t *testing.T) {
					request := schemaRequest(t, shape, model)
					request.Stream = mode == "streaming"
					before, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					old := antigravityStreamClient
					t.Cleanup(func() { antigravityStreamClient = old })
					calls := 0
					antigravityStreamClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						calls++
						var payload struct {
							Model   string `json:"model"`
							Request struct {
								Tools []struct {
									Declarations []map[string]interface{} `json:"functionDeclarations"`
								} `json:"tools"`
							} `json:"request"`
						}
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Fatal(err)
						}
						if payload.Model != model || len(payload.Request.Tools) != 1 {
							t.Fatal("model or tool group changed")
						}
						declarations := payload.Request.Tools[0].Declarations
						if len(declarations) != 1 {
							t.Fatalf("tool count = %d", len(declarations))
						}
						decl := declarations[0]
						if decl["name"] != "dispatch_event" || decl["description"] != "Dispatch an event." {
							t.Fatal("tool identity changed")
						}
						if _, present := decl["parameters"]; present {
							t.Error("Claude tool uses restricted OpenAPI schema transport")
						}
						if !reflect.DeepEqual(decl["parametersJsonSchema"], request.Tools[0].ParametersSchema) {
							t.Error("full canonical tool schema was not preserved")
						}
						return jsonResponse("data: {\"response\":{\"candidates\":[" +
							"{\"content\":{\"parts\":[{\"text\":\"SCHEMA_OK\"}]},\"finishReason\":\"STOP\"}]}}\n\n"), nil
					})}
					var response *cif.CanonicalResponse
					if request.Stream {
						stream, streamErr := Stream(
							t.Context(),
							"token",
							"http://upstream.test",
							"project",
							request,
						)
						if streamErr != nil {
							t.Fatal(streamErr)
						}
						response, err = shared.CollectStream(stream)
					} else {
						response, err = Execute(
							t.Context(),
							"token",
							"http://upstream.test",
							"project",
							request,
						)
					}
					if err != nil {
						t.Fatal(err)
					}
					if response.StopReason != cif.StopReasonEndTurn || calls != 1 {
						t.Fatalf("stop=%q attempts=%d", response.StopReason, calls)
					}
					after, err := json.Marshal(request)
					if err != nil || string(before) != string(after) {
						t.Fatal("canonical request mutated")
					}
				})
			}
		}
	}
}

func TestGeminiToolSchemaTransportUnchanged(t *testing.T) {
	request := schemaRequest(t, "messages", "gemini-3.8-flash-low")
	old := antigravityStreamClient
	t.Cleanup(func() { antigravityStreamClient = old })
	antigravityStreamClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		req := payload["request"].(map[string]interface{})
		group := req["tools"].([]interface{})[0].(map[string]interface{})
		decl := group["functionDeclarations"].([]interface{})[0].(map[string]interface{})
		if _, present := decl["parametersJsonSchema"]; present {
			t.Error("Gemini schema representation changed")
		}
		if !reflect.DeepEqual(decl["parameters"], shared.SanitizeGeminiSchema(request.Tools[0].ParametersSchema)) {
			t.Error("Gemini sanitization changed")
		}
		return jsonResponse("data: [DONE]\n\n"), nil
	})}
	ch, err := Stream(
		t.Context(),
		"token",
		"http://upstream.test",
		"project",
		request,
	)
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
}

func TestAntigravityHTTPErrorFallbackPolicy(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 422, 429, 500, 502, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			err := error(&upstreamHTTPError{status: status, body: "synthetic failure"})
			policy, ok := err.(interface{ NonStreamingFallbackAllowed() bool })
			if !ok {
				t.Fatal("HTTP error does not expose fallback eligibility")
			}
			if got, want := policy.NonStreamingFallbackAllowed(), status >= 500; got != want {
				t.Fatalf("fallback allowed = %v, want %v", got, want)
			}
		})
	}
}
