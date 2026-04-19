package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	providertypes "omnimodel/internal/providers/types"
	"omnimodel/internal/registry"
)

type stubProvider struct {
	id         string
	instanceID string
	name       string
	models     *providertypes.ModelsResponse
	adapter    providertypes.ProviderAdapter
}

func (p *stubProvider) GetID() string { return p.id }

func (p *stubProvider) GetInstanceID() string { return p.instanceID }

func (p *stubProvider) GetName() string { return p.name }

func (p *stubProvider) SetupAuth(_ *providertypes.AuthOptions) error { return nil }

func (p *stubProvider) GetToken() string { return "" }

func (p *stubProvider) RefreshToken() error { return nil }

func (p *stubProvider) GetBaseURL() string { return "" }

func (p *stubProvider) GetHeaders(_ bool) map[string]string { return map[string]string{} }

func (p *stubProvider) GetModels() (*providertypes.ModelsResponse, error) { return p.models, nil }

func (p *stubProvider) CreateChatCompletions(_ map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("not implemented in tests")
}

func (p *stubProvider) CreateEmbeddings(_ map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("not implemented in tests")
}

func (p *stubProvider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{"requests": 0}, nil
}

func (p *stubProvider) GetAdapter() providertypes.ProviderAdapter { return p.adapter }

type stubAdapter struct {
	provider  providertypes.Provider
	executeFn func(*cif.CanonicalRequest) (*cif.CanonicalResponse, error)
	streamFn  func(*cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error)
	remapFn   func(string) string
}

func (a *stubAdapter) GetProvider() providertypes.Provider { return a.provider }

func (a *stubAdapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	if a.executeFn == nil {
		return nil, errors.New("execute not configured")
	}
	return a.executeFn(request)
}

func (a *stubAdapter) ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.streamFn == nil {
		return nil, errors.New("stream not configured")
	}
	return a.streamFn(request)
}

func (a *stubAdapter) RemapModel(canonicalModel string) string {
	if a.remapFn != nil {
		return a.remapFn(canonicalModel)
	}
	return canonicalModel
}

func registerStubModelsProvider(
	t *testing.T,
	models []providertypes.Model,
	active bool,
) string {
	t.Helper()

	instanceID := fmt.Sprintf("stub-models-%d", time.Now().UnixNano())
	provider := &stubProvider{
		id:         "stub-provider",
		instanceID: instanceID,
		name:       "Stub Provider",
		models: &providertypes.ModelsResponse{
			Object: "list",
			Data:   models,
		},
	}

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("failed to register stub models provider: %v", err)
	}
	if active {
		if _, err := reg.AddActive(instanceID); err != nil {
			t.Fatalf("failed to activate stub models provider: %v", err)
		}
	}

	t.Cleanup(func() {
		_ = reg.Remove(instanceID)
	})

	return instanceID
}

func registerStubProvider(
	t *testing.T,
	modelID string,
	executeFn func(*cif.CanonicalRequest) (*cif.CanonicalResponse, error),
	streamFn func(*cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error),
) string {
	return registerStubProviderWithType(t, "stub-provider", modelID, executeFn, streamFn)
}

func registerStubProviderWithType(
	t *testing.T,
	providerID string,
	modelID string,
	executeFn func(*cif.CanonicalRequest) (*cif.CanonicalResponse, error),
	streamFn func(*cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error),
) string {
	t.Helper()

	instanceID := fmt.Sprintf("stub-%d", time.Now().UnixNano())
	provider := &stubProvider{
		id:         providerID,
		instanceID: instanceID,
		name:       "Stub Provider",
		models: &providertypes.ModelsResponse{
			Object: "list",
			Data: []providertypes.Model{
				{
					ID:       modelID,
					Name:     modelID,
					Provider: instanceID,
				},
			},
		},
	}
	adapter := &stubAdapter{
		executeFn: executeFn,
		streamFn:  streamFn,
	}
	provider.adapter = adapter
	adapter.provider = provider

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("failed to register stub provider: %v", err)
	}
	if _, err := reg.AddActive(instanceID); err != nil {
		t.Fatalf("failed to activate stub provider: %v", err)
	}

	t.Cleanup(func() {
		_ = reg.Remove(instanceID)
	})

	return instanceID
}

func postJSON(t *testing.T, url string, body string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(body)
}


func TestAnthropicMessagesRoute_NonStreamingThinkingSuppression(t *testing.T) {
	registerStubProvider(t, "thinking-model", func(_ *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		return &cif.CanonicalResponse{
			ID:    "msg_thinking",
			Model: "thinking-model",
			Content: []cif.CIFContentPart{
				cif.CIFThinkingPart{Type: "thinking", Thinking: "Let me think this through."},
				cif.CIFTextPart{Type: "text", Text: "Final answer."},
			},
			StopReason: cif.StopReasonEndTurn,
		}, nil
	}, nil)

	srv := newTestServer(t)
	defer srv.Close()

	requestBody := `{"model":"thinking-model","max_tokens":100,"messages":[{"role":"user","content":"Hi"}]}`

	t.Run("suppresses thinking by default", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			requestBody,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(payload.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d: %#v", len(payload.Content), payload.Content)
		}
		if payload.Content[0].Type != "text" || payload.Content[0].Text != "Final answer." {
			t.Fatalf("unexpected content: %#v", payload.Content)
		}
	})

	t.Run("preserves thinking when opted in", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			requestBody,
			map[string]string{
				"anthropic-version": "2023-06-01",
				"anthropic-beta":   "interleaved-thinking-2025-05-14",
			},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(payload.Content) != 2 {
			t.Fatalf("expected 2 content blocks, got %d: %#v", len(payload.Content), payload.Content)
		}
		if payload.Content[0].Type != "thinking" || payload.Content[0].Thinking != "Let me think this through." {
			t.Fatalf("unexpected first content block: %#v", payload.Content[0])
		}
		if payload.Content[1].Type != "text" || payload.Content[1].Text != "Final answer." {
			t.Fatalf("unexpected second content block: %#v", payload.Content[1])
		}
	})
}
func TestModelsEndpointReturnsOnlyActiveProviderModels(t *testing.T) {
	activeInstanceID := registerStubModelsProvider(t, []providertypes.Model{
		{
			ID:       "alibaba-live-model",
			Name:     "Alibaba Live Model",
			Provider: "alibaba",
		},
	}, true)

	registerStubModelsProvider(t, []providertypes.Model{
		{
			ID:       "inactive-model",
			Name:     "Inactive Model",
			Provider: "alibaba",
		},
	}, false)

	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/models")
	if err != nil {
		t.Fatalf("GET /models: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Object  string `json:"object"`
		HasMore bool   `json:"has_more"`
		Data    []struct {
			ID          string `json:"id"`
			OwnedBy     string `json:"owned_by"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if payload.Object != "list" {
		t.Fatalf("unexpected object: %q", payload.Object)
	}
	if payload.HasMore {
		t.Fatalf("expected has_more=false, got true")
	}
	if len(payload.Data) != 1 {
		t.Fatalf("expected 1 active model, got %d: %s", len(payload.Data), body)
	}
	if payload.Data[0].ID != "alibaba-live-model" {
		t.Fatalf("unexpected model id: %q", payload.Data[0].ID)
	}
	if payload.Data[0].OwnedBy != activeInstanceID {
		t.Fatalf("unexpected owned_by: %q", payload.Data[0].OwnedBy)
	}
	if payload.Data[0].DisplayName != "Alibaba Live Model" {
		t.Fatalf("unexpected display_name: %q", payload.Data[0].DisplayName)
	}
	if strings.Contains(body, "inactive-model") {
		t.Fatalf("inactive provider model leaked into /models response: %s", body)
	}
}

func TestModelsEndpointReturnsEmptyListWithoutActiveProviders(t *testing.T) {
	registerStubModelsProvider(t, []providertypes.Model{
		{
			ID:       "registered-but-inactive",
			Name:     "Registered But Inactive",
			Provider: "alibaba",
		},
	}, false)

	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/models")
	if err != nil {
		t.Fatalf("GET /models: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Object  string `json:"object"`
		HasMore bool   `json:"has_more"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if payload.Object != "list" {
		t.Fatalf("unexpected object: %q", payload.Object)
	}
	if payload.HasMore {
		t.Fatalf("expected has_more=false, got true")
	}
	if len(payload.Data) != 0 {
		t.Fatalf("expected no models, got %d: %s", len(payload.Data), body)
	}
	if !strings.Contains(body, "\"data\":[]") {
		t.Fatalf("expected empty data array, got: %s", body)
	}
	if strings.Contains(body, "\"gpt-4o\"") {
		t.Fatalf("unexpected fallback models in response: %s", body)
	}
}

func TestCreateChatSessionHonorsProvidedSessionID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	sessionID := fmt.Sprintf("client-session-%d", time.Now().UnixNano())

	resp := postJSON(
		t,
		srv.URL+"/api/admin/chat/sessions",
		fmt.Sprintf(`{"session_id":"%s","title":"hi","model_id":"qwen3.6-plus","api_shape":"openai"}`, sessionID),
		nil,
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var createPayload struct {
		Success   bool   `json:"success"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(body), &createPayload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !createPayload.Success {
		t.Fatalf("expected success=true, got false: %s", body)
	}
	if createPayload.SessionID != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, createPayload.SessionID)
	}

	getResp, err := http.Get(srv.URL + "/api/admin/chat/sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /api/admin/chat/sessions/:id failed: %v", err)
	}
	getBody := readBody(t, getResp)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 fetching created session, got %d: %s", getResp.StatusCode, getBody)
	}

	var sessionPayload struct {
		ID      string `json:"id"`
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal([]byte(getBody), &sessionPayload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if sessionPayload.ID != sessionID {
		t.Fatalf("expected fetched session id %q, got %q", sessionID, sessionPayload.ID)
	}
	if sessionPayload.ModelID != "qwen3.6-plus" {
		t.Fatalf("unexpected model_id: %q", sessionPayload.ModelID)
	}
}

func TestAPIShapeEndpointsUseGoSerializers(t *testing.T) {
	modelID := "shape-model"
	registerStubProvider(
		t,
		modelID,
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_shape",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "pong"},
				},
				StopReason: cif.StopReasonEndTurn,
				Usage:      &cif.CIFUsage{InputTokens: 3, OutputTokens: 1},
			}, nil
		},
		nil,
	)

	srv := newTestServer(t)
	defer srv.Close()

	t.Run("chat completions", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", `{"model":"shape-model","messages":[{"role":"user","content":"ping"}]}`, nil)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Object  string `json:"object"`
			Choices []struct {
				Message struct {
					Role    string  `json:"role"`
					Content *string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if payload.Object != "chat.completion" {
			t.Fatalf("unexpected object: %q", payload.Object)
		}
		if len(payload.Choices) != 1 || payload.Choices[0].Message.Content == nil || *payload.Choices[0].Message.Content != "pong" {
			t.Fatalf("unexpected chat completion payload: %#v", payload)
		}
	})

	t.Run("anthropic messages", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			`{"model":"shape-model","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if payload.Type != "message" || payload.Role != "assistant" {
			t.Fatalf("unexpected anthropic response: %#v", payload)
		}
		if len(payload.Content) != 1 || payload.Content[0].Text != "pong" {
			t.Fatalf("unexpected anthropic content: %#v", payload.Content)
		}
	})

	t.Run("responses api", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/responses",
			`{"model":"shape-model","input":[{"type":"message","role":"user","content":[{"type":"output_text","text":"ping"}]}]}`,
			nil,
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Object string `json:"object"`
			Output []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if payload.Object != "realtime.response" {
			t.Fatalf("unexpected responses object: %q", payload.Object)
		}
		if len(payload.Output) != 1 || payload.Output[0].Type != "message" || payload.Output[0].Content[0].Text != "pong" {
			t.Fatalf("unexpected responses payload: %#v", payload)
		}
	})
}

func TestAnthropicMessagesRouteNormalizesAliasesBeforeProviderExecution(t *testing.T) {
	testCases := []struct {
		name          string
		registeredID  string
		requestModel  string
		expectedModel string
	}{
		{
			name:          "dated haiku alias",
			registeredID:  "claude-haiku-4.5",
			requestModel:  "claude-haiku-4-5-20251001",
			expectedModel: "claude-haiku-4.5",
		},
		{
			name:          "sonnet shorthand alias",
			registeredID:  "claude-sonnet-4.6",
			requestModel:  "sonnet-4.6",
			expectedModel: "claude-sonnet-4.6",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedModel string

			registerStubProvider(
				t,
				tc.registeredID,
				func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
					capturedModel = req.Model
					return &cif.CanonicalResponse{
						ID:    "resp_alias",
						Model: req.Model,
						Content: []cif.CIFContentPart{
							cif.CIFTextPart{Type: "text", Text: "pong"},
						},
						StopReason: cif.StopReasonEndTurn,
					}, nil
				},
				nil,
			)

			srv := newTestServer(t)
			defer srv.Close()

			resp := postJSON(
				t,
				srv.URL+"/v1/messages",
				fmt.Sprintf(`{"model":"%s","max_tokens":20,"messages":[{"role":"user","content":"ping"}]}`, tc.requestModel),
				map[string]string{"anthropic-version": "2023-06-01"},
			)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
			}

			if capturedModel != tc.expectedModel {
				t.Fatalf("expected provider to receive model %q, got %q", tc.expectedModel, capturedModel)
			}
		})
	}
}

func TestAnthropicMessagesRouteRoutesVirtualModelsBeforeProviderExecution(t *testing.T) {
	const upstreamModel = "claude-haiku-4.5"
	const virtualModel = "claude-mythos-5.0"

	var capturedModel string

	registerStubProvider(
		t,
		upstreamModel,
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			capturedModel = req.Model
			return &cif.CanonicalResponse{
				ID:    "resp_virtual_model",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "pong"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	vmodelStore := database.NewVirtualModelStore()
	if err := vmodelStore.Create(&database.VirtualModelRecord{
		VirtualModelID: virtualModel,
		Name:           "Claude Mythos 5.0",
		Description:    "Anthropic virtual model alias",
		APIShape:       "anthropic",
		LbStrategy:     database.LbStrategyRoundRobin,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("failed to create virtual model: %v", err)
	}
	t.Cleanup(func() {
		_ = vmodelStore.Delete(virtualModel)
	})

	upstreamStore := database.NewVirtualModelUpstreamStore()
	if err := upstreamStore.SetForVModel(virtualModel, []database.VirtualModelUpstreamRecord{{
		VirtualModelID: virtualModel,
		ModelID:        upstreamModel,
		Weight:         1,
		Priority:       0,
	}}); err != nil {
		t.Fatalf("failed to set virtual model upstreams: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages",
		fmt.Sprintf(`{"model":"%s","max_tokens":20,"messages":[{"role":"user","content":"ping"}]}`, virtualModel),
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	if capturedModel != upstreamModel {
		t.Fatalf("expected provider to receive upstream model %q, got %q", upstreamModel, capturedModel)
	}
}

func TestAnthropicMessagesRouteRewritesAlibabaAnthropicAliasBeforeProxy(t *testing.T) {
	var capturedModel string

	registerStubProviderWithType(
		t,
		"alibaba",
		"claude-haiku-4.5",
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			capturedModel = req.Model
			return &cif.CanonicalResponse{
				ID:    "resp_alibaba_alias",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "pong"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages",
		`{"model":"claude-haiku-4-5","max_tokens":20,"messages":[{"role":"user","content":"ping"}]}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	if capturedModel != "claude-haiku-4.5" {
		t.Fatalf("expected provider to receive model %q, got %q", "claude-haiku-4.5", capturedModel)
	}
}

func TestStreamingEndpointsExposeExpectedEventShapes(t *testing.T) {
	modelID := "stream-model"
	registerStubProvider(
		t,
		modelID,
		nil,
		func(req *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
			ch := make(chan cif.CIFStreamEvent, 3)
			ch <- cif.CIFStreamStart{Type: "stream_start", ID: "stream_123", Model: req.Model}
			ch <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        0,
				ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
				Delta:        cif.TextDelta{Type: "text_delta", Text: "pong"},
			}
			ch <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: cif.StopReasonEndTurn,
				Usage:      &cif.CIFUsage{InputTokens: 4, OutputTokens: 1},
			}
			close(ch)
			return ch, nil
		},
	)

	srv := newTestServer(t)
	defer srv.Close()

	t.Run("chat completions", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", `{"model":"stream-model","stream":true,"messages":[{"role":"user","content":"ping"}]}`, nil)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("expected event-stream content type, got %q", resp.Header.Get("Content-Type"))
		}
		if !strings.Contains(body, "chat.completion.chunk") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("unexpected OpenAI stream body: %s", body)
		}
	})

	t.Run("anthropic messages", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			`{"model":"stream-model","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: message_stop") {
			t.Fatalf("unexpected Anthropic stream body: %s", body)
		}
	})

	t.Run("responses api", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/responses",
			`{"model":"stream-model","stream":true,"input":[{"type":"message","role":"user","content":"ping"}]}`,
			nil,
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "event: response.created") || !strings.Contains(body, "event: response.completed") {
			t.Fatalf("unexpected Responses stream body: %s", body)
		}
	})
}

func TestMessagesEndpointHandlesLongMixedConversation(t *testing.T) {
	modelID := "long-conversation-model"
	registerStubProvider(
		t,
		modelID,
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			if len(req.Messages) != 9 {
				return nil, fmt.Errorf("expected 9 messages, got %d", len(req.Messages))
			}
			lastUser, ok := req.Messages[8].(cif.CIFUserMessage)
			if !ok {
				return nil, fmt.Errorf("expected final message to be user, got %T", req.Messages[8])
			}
			lastText, ok := lastUser.Content[0].(cif.CIFTextPart)
			if !ok || !strings.Contains(lastText.Text, "Final question") {
				return nil, fmt.Errorf("unexpected final user content: %#v", lastUser.Content)
			}
			return &cif.CanonicalResponse{
				ID:    "resp_long",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Handled long conversation."},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/messages",
		`{"model":"long-conversation-model","max_tokens":100,"messages":[
			{"role":"user","content":"Start of conversation"},
			{"role":"assistant","content":[{"type":"text","text":"I understand, let's continue."}]},
			{"role":"user","content":[{"type":"text","text":"Question 2"}]},
			{"role":"assistant","content":"Answer 2"},
			{"role":"user","content":[{"type":"text","text":"Question 3"}]},
			{"role":"assistant","content":[{"type":"text","text":"Answer 3"}]},
			{"role":"user","content":"Question 4"},
			{"role":"assistant","content":[{"type":"text","text":"Answer 4"}]},
			{"role":"user","content":[{"type":"text","text":"Final question with mixed content formats"}]}
		]}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestChatCompletionsStreamingDoesNotFallbackForGitHubCopilotCompatMode(t *testing.T) {
	modelID := "copilot-stream-no-fallback-model"
	var executeCalls int
	var streamCalls int

	registerStubProviderWithType(
		t,
		string(providertypes.ProviderGitHubCopilot),
		modelID,
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			executeCalls++
			return &cif.CanonicalResponse{
				ID:         "resp_unexpected_nonstream",
				Model:      req.Model,
				Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "non-stream fallback should not run"}},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		func(_ *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
			streamCalls++
			return nil, errors.New("simulated upstream streaming failure")
		},
	)

	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/chat/completions",
		fmt.Sprintf(`{"model":"%s","stream":true,"messages":[{"role":"user","content":"ping"}]}`, modelID),
		nil,
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", resp.StatusCode, body)
	}
	if executeCalls != 0 {
		t.Fatalf("expected zero non-stream fallback calls, got %d", executeCalls)
	}
	if streamCalls != 1 {
		t.Fatalf("expected one stream attempt, got %d", streamCalls)
	}
}

func TestToolCallShapesAcrossChatAndMessagesEndpoints(t *testing.T) {
	modelID := "tool-shape-model"
	registerStubProvider(
		t,
		modelID,
		func(req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_tool",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_weather",
						ToolName:      "get_weather",
						ToolArguments: map[string]interface{}{"location": "Shanghai"},
					},
				},
				StopReason: cif.StopReasonToolUse,
			}, nil
		},
		nil,
	)

	srv := newTestServer(t)
	defer srv.Close()

	t.Run("chat completions tool_calls", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/chat/completions",
			`{"model":"tool-shape-model","messages":[{"role":"user","content":"Check the weather"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}]}`,
			nil,
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
				Message      struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(payload.Choices) != 1 || payload.Choices[0].FinishReason == nil || *payload.Choices[0].FinishReason != "tool_calls" {
			t.Fatalf("unexpected chat tool response: %#v", payload)
		}
		if len(payload.Choices[0].Message.ToolCalls) != 1 || payload.Choices[0].Message.ToolCalls[0].Function.Name != "get_weather" {
			t.Fatalf("unexpected tool call payload: %#v", payload.Choices[0].Message.ToolCalls)
		}
	})

	t.Run("anthropic tool_use", func(t *testing.T) {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			`{"model":"tool-shape-model","max_tokens":100,"tools":[{"name":"get_weather","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"Check the weather"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			StopReason *string `json:"stop_reason"`
			Content    []struct {
				Type  string                 `json:"type"`
				ID    string                 `json:"id"`
				Name  string                 `json:"name"`
				Input map[string]interface{} `json:"input"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if payload.StopReason == nil || *payload.StopReason != "tool_use" {
			t.Fatalf("unexpected anthropic stop reason: %#v", payload.StopReason)
		}
		if len(payload.Content) != 1 || payload.Content[0].Type != "tool_use" || payload.Content[0].Name != "get_weather" {
			t.Fatalf("unexpected anthropic tool payload: %#v", payload.Content)
		}
		if payload.Content[0].Input["location"] != "Shanghai" {
			t.Fatalf("unexpected tool input: %#v", payload.Content[0].Input)
		}
	})
}
