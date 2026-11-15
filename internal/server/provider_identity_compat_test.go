package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"omnillm/internal/testcompat"
)

// Every client shape completes five ordered calls while routing holds provider leases.
func TestProviderLeasesPreserveClientToolLoops(t *testing.T) {
	var expected []testcompat.ToolExchange
	turn := 0
	registerStubProvider(t, testcompat.Model, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		assertCompatibilityExchanges(t, request.Messages, expected[:turn])
		if turn == len(expected) {
			return &cif.CanonicalResponse{
				ID: "lease-terminal", Model: request.Model,
				Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		}
		exchange := expected[turn]
		turn++
		return &cif.CanonicalResponse{
			ID: "lease-tool", Model: request.Model,
			Content: []cif.CIFContentPart{cif.CIFToolCallPart{
				Type: "tool_call", ToolCallID: exchange.ID, ToolName: exchange.Name,
				ToolArguments: exchange.Arguments, ToolKind: exchange.ToolKind,
				RawInput: exchange.RawInput, Namespace: exchange.Namespace,
			}},
			StopReason: cif.StopReasonToolUse,
		}, nil
	}, nil)
	srv := newTestServer(t)
	defer srv.Close()

	for _, fixture := range testcompat.ClientCacheFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			expected = append([]testcompat.ToolExchange{}, fixture.Exchanges...)
			request := append(json.RawMessage{}, fixture.Request...)
			if fixture.Name == "claude-code" {
				for i := range expected {
					oldID := expected[i].ID
					expected[i].ID = "toolu_" + oldID
					request = bytes.ReplaceAll(request, []byte(`"`+oldID+`"`), []byte(`"`+expected[i].ID+`"`))
				}
			}
			turn = 0
			for completed := 0; completed <= len(expected); completed++ {
				body := compatibilityHistoryPrefix(t, request, fixture.Endpoint, completed)
				response := postJSON(t, srv.URL+fixture.Endpoint, body, fixture.Headers)
				responseBody := readBody(t, response)
				if response.StatusCode != http.StatusOK {
					t.Fatalf("turn %d status %d: %s", completed, response.StatusCode, responseBody)
				}
				if completed == len(expected) {
					assertNoCompatibilityToolCall(t, fixture.Endpoint, responseBody)
					var payload map[string]interface{}
					if err := json.Unmarshal([]byte(responseBody), &payload); err != nil {
						t.Fatalf("decode terminal response: %v", err)
					}
					if !containsJSONValue(payload, testcompat.FinalAnswer) {
						t.Fatalf("missing terminal response: %s", responseBody)
					}
					continue
				}
				assertToolResponseID(t, fixture.Endpoint, responseBody, expected[completed].ID)
			}
			if turn != testcompat.MinimumSequentialToolCalls {
				t.Fatalf("tool calls = %d, want %d", turn, testcompat.MinimumSequentialToolCalls)
			}
		})
	}
}

func TestProviderRenamePreservesNativeModelStartingWithOldID(t *testing.T) {
	const oldID = "native-prefix-old"
	const newID = "native-prefix-new"
	model := oldID + "/native-model"
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "stub-provider", Name: "Stub"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	var gotModel string
	newStub := func(id string) *stubProvider {
		provider := &stubProvider{id: "stub-provider", instanceID: id, name: "Stub", models: &providertypes.ModelsResponse{Data: []providertypes.Model{{ID: model}}}}
		provider.adapter = &stubAdapter{provider: provider, executeFn: func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			gotModel = request.Model
			return &cif.CanonicalResponse{ID: "native-result", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}}, StopReason: cif.StopReasonEndTurn}, nil
		}}
		return provider
	}
	if err := reg.Register(newStub(oldID), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.AddActive(oldID); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(oldID)
		_ = reg.Remove(newID)
		reg.WaitForPendingSaves()
		_ = store.Delete(oldID)
		_ = store.Delete(newID)
	})
	lease, err := reg.BeginIdentityChange(oldID)
	if err != nil {
		t.Fatalf("begin identity change: %v", err)
	}
	if err := lease.Commit(context.Background(), newStub(newID)); err != nil {
		lease.Release()
		t.Fatalf("commit identity change: %v", err)
	}
	lease.Release()
	srv := newTestServer(t)
	defer srv.Close()
	response := postJSON(t, srv.URL+"/v1/chat/completions", `{"model":"native-prefix-old/native-model","messages":[{"role":"user","content":"hello"}]}`, nil)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || gotModel != model {
		t.Fatalf("native model after rename = %q, status %d: %s", gotModel, response.StatusCode, body)
	}
}

func TestProviderRenameIsBusyDuringStreaming(t *testing.T) {
	for _, row := range []struct {
		name, endpoint, body string
	}{
		{"chat", "/v1/chat/completions", `{"model":"stream-lease-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`},
		{"messages", "/v1/messages", `{"model":"stream-lease-model","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`},
		{"responses", "/v1/responses", `{"model":"stream-lease-model","stream":true,"input":"hello"}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseStream := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseStream()
			providerID := registerStubProvider(t, "stream-lease-model", nil, func(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
				events := make(chan cif.CIFStreamEvent, 2)
				events <- cif.CIFStreamStart{Type: "stream_start", ID: "lease-stream", Model: request.Model}
				close(started)
				go func() {
					defer close(events)
					select {
					case <-release:
						events <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
					case <-ctx.Done():
					}
				}()
				return events, nil
			})
			registry.GetProviderRegistry().WaitForPendingSaves()
			srv := newTestServer(t)
			defer func() {
				releaseStream()
				srv.Close()
			}()
			finished := make(chan int, 1)
			go func() {
				response := postJSON(t, srv.URL+row.endpoint, row.body, nil)
				_ = readBody(t, response)
				finished <- response.StatusCode
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("stream did not start")
			}
			rename := patchJSON(t, srv.URL+"/api/admin/providers/"+providerID+"/identity", `{"new_id":"stream-renamed"}`)
			body := readBody(t, rename)
			if rename.StatusCode != http.StatusConflict || !strings.Contains(strings.ToLower(body), "busy") {
				t.Fatalf("rename during stream = %d: %s", rename.StatusCode, body)
			}
			releaseStream()
			select {
			case status := <-finished:
				if status != http.StatusOK {
					t.Fatalf("stream status = %d", status)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stream did not finish")
			}
		})
	}
}

func compatibilityHistoryPrefix(t *testing.T, raw json.RawMessage, endpoint string, completed int) string {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	field := "messages"
	if endpoint == "/v1/responses" {
		field = "input"
	}
	history, ok := payload[field].([]interface{})
	if !ok || len(history) < 2+2*completed {
		t.Fatalf("fixture history %q is incomplete", field)
	}
	selected := append([]interface{}{}, history[:1+2*completed]...)
	if completed == testcompat.MinimumSequentialToolCalls {
		selected = append(selected, history[len(history)-1])
	}
	payload[field] = selected
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return string(encoded)
}

func assertToolResponseID(t *testing.T, endpoint, body, id string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	var calls []interface{}
	switch endpoint {
	case "/v1/messages":
		calls, _ = payload["content"].([]interface{})
	case "/v1/responses":
		calls, _ = payload["output"].([]interface{})
	default:
		choices, _ := payload["choices"].([]interface{})
		if len(choices) > 0 {
			choice, _ := choices[0].(map[string]interface{})
			message, _ := choice["message"].(map[string]interface{})
			calls, _ = message["tool_calls"].([]interface{})
		}
	}
	matches := 0
	for _, raw := range calls {
		call, _ := raw.(map[string]interface{})
		kind, _ := call["type"].(string)
		if kind != "tool_use" && kind != "function" && kind != "function_call" && kind != "custom_tool_call" {
			continue
		}
		if call["id"] == id || call["call_id"] == id {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("%s response contained %d matching tool calls for %q: %s", endpoint, matches, id, body)
	}
}

func containsJSONValue(value interface{}, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case []interface{}:
		for _, child := range typed {
			if containsJSONValue(child, target) {
				return true
			}
		}
	case map[string]interface{}:
		for _, child := range typed {
			if containsJSONValue(child, target) {
				return true
			}
		}
	}
	return false
}
