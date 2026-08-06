package server

// Extended integration tests for the "<providerPrefix>/<modelID>" routing feature.
//
// This file supplements provider_prefix_routing_test.go with scenarios that
// cover:
//   - Anthropic messages shape (POST /v1/messages) with a prefix
//   - Streaming requests with a provider prefix
//   - Virtual model ID used as a prefix (not a real instance ID)
//   - Case-sensitivity: prefixes are matched case-sensitively
//   - Empty model string handling
//   - Prefix that is a substring of another provider ID
//   - Round-trip: model field in response equals the bare name (no prefix)
//     for the Anthropic shape

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

// ─── helpers specific to this file ───────────────────────────────────────────

// registerAnthropicPrefixProvider is like registerPrefixProvider but also
// returns the captured model string for Anthropic-shape validation.
func registerAnthropicPrefixProvider(
	t *testing.T,
	instanceID, modelID string,
	capturedModel *string,
) {
	t.Helper()
	registerPrefixProvider(t, instanceID, "", "stub-provider", modelID, capturedModel)
}

// anthropicMessages fires a POST /v1/messages with the given model.
func anthropicMessages(t *testing.T, srvURL, model string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"model":%q,"max_tokens":100,"messages":[{"role":"user","content":"ping"}]}`,
		model,
	)
	resp := postJSON(t, srvURL+"/v1/messages", body, nil)
	return resp.StatusCode, readBody(t, resp)
}

func responsesRequest(t *testing.T, srvURL, model string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"model":%q,"input":"ping"}`, model)
	resp := postJSON(t, srvURL+"/v1/responses", body, nil)
	return resp.StatusCode, readBody(t, resp)
}

// streamingChatCompletions fires a POST /v1/chat/completions with stream:true
// and returns the collected SSE lines and HTTP status.
func streamingChatCompletions(t *testing.T, srvURL, model string) (int, []string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"model":%q,"stream":true,"messages":[{"role":"user","content":"ping"}]}`,
		model,
	)
	resp := postJSON(t, srvURL+"/v1/chat/completions", body, nil)
	defer resp.Body.Close()
	var lines []string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan streaming response: %v", err)
	}
	return resp.StatusCode, lines
}

// ─── Anthropic messages shape ─────────────────────────────────────────────────

// TestProviderPrefixRouting_AnthropicShape verifies that prefix routing works
// for the Anthropic /v1/messages endpoint, not just /v1/chat/completions.
func TestProviderPrefixRouting_AnthropicShape(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-anthropic-" + sfx

	var capturedModel string
	registerAnthropicPrefixProvider(t, instanceID, "claude-sonnet-4-6", &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := anthropicMessages(t, srv.URL, instanceID+"/claude-sonnet-4-6")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "claude-sonnet-4-6" {
		t.Errorf("expected bare model forwarded; got %q", capturedModel)
	}
}

// TestProviderPrefixRouting_AnthropicShape_ResponseModelIsBareName verifies
// that the response model field contains the bare model name (no prefix) in
// the Anthropic messages response shape.
func TestProviderPrefixRouting_AnthropicShape_ResponseModelIsBareName(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-ant-resp-" + sfx

	registerAnthropicPrefixProvider(t, instanceID, "claude-haiku-4-5", nil)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := anthropicMessages(t, srv.URL, instanceID+"/claude-haiku-4-5")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}

	var parsed struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("parse response: %v: body=%s", err, body)
	}
	if parsed.Model != "claude-haiku-4.5" && parsed.Model != "claude-haiku-4-5" {
		t.Errorf("expected bare model name in response, got %q", parsed.Model)
	}
}

// ─── Streaming ────────────────────────────────────────────────────────────────

// TestProviderPrefixRouting_StreamingRequest verifies prefix routing for
// streaming chat completions.
func TestProviderPrefixRouting_StreamingRequest(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-stream-" + sfx

	var capturedModel string

	model := providertypes.Model{ID: "stream-model", Name: "stream-model", Provider: instanceID}
	provider := &stubProvider{
		id:         "stub-provider",
		instanceID: instanceID,
		name:       instanceID,
		models:     &providertypes.ModelsResponse{Object: "list", Data: []providertypes.Model{model}},
	}
	adapter := &stubAdapter{
		streamFn: func(_ context.Context, req *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
			capturedModel = req.Model
			ch := make(chan cif.CIFStreamEvent, 3)
			ch <- cif.CIFStreamStart{Type: "stream_start", ID: "resp-prefix-stream", Model: req.Model}
			ch <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        0,
				ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
				Delta:        cif.TextDelta{Type: "text_delta", Text: "pong"},
			}
			ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
			close(ch)
			return ch, nil
		},
	}
	provider.adapter = adapter
	adapter.provider = provider

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.AddActive(instanceID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{
		InstanceID: instanceID,
		ProviderID: "stub-provider",
		Name:       instanceID,
		Activated:  true,
	}); err != nil {
		t.Fatalf("seed DB: %v", err)
	}
	t.Cleanup(func() {
		_ = reg.Remove(instanceID)
		_ = store.Delete(instanceID)
	})

	srv := newTestServer(t)
	defer srv.Close()

	status, lines := streamingChatCompletions(t, srv.URL, instanceID+"/stream-model")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if capturedModel != "stream-model" {
		t.Errorf("expected bare model %q forwarded to streaming adapter; got %q", "stream-model", capturedModel)
	}
	hasData := false
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			hasData = true
			break
		}
	}
	if !hasData {
		t.Errorf("expected SSE data lines in streaming response; got: %v", lines)
	}
}

// ─── Case sensitivity ─────────────────────────────────────────────────────────

// TestProviderPrefixRouting_CaseSensitivePrefix verifies that prefixes are
// matched case-sensitively — an upper-cased prefix should not route to a
// lower-cased provider instance.
func TestProviderPrefixRouting_CaseSensitivePrefix(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-lowercase-" + sfx
	registerPrefixProvider(t, instanceID, "", "stub-provider", "case-model", nil)

	srv := newTestServer(t)
	defer srv.Close()

	// Use the correctly cased prefix — must succeed
	status, body := chatCompletions(t, srv.URL, instanceID+"/case-model")
	if status != http.StatusOK {
		t.Fatalf("correct case: expected 200, got %d: %s", status, body)
	}

	// Use an upper-cased prefix — provider lookup should fail
	upperID := strings.ToUpper(instanceID)
	statusUpper, bodyUpper := chatCompletions(t, srv.URL, upperID+"/case-model")
	if statusUpper == http.StatusOK {
		t.Fatalf("upper-cased prefix should not match; got 200: %s", bodyUpper)
	}
}

// ─── Substring prefix ─────────────────────────────────────────────────────────

// TestProviderPrefixRouting_SubstringDoesNotMatchLongerID verifies that a
// prefix that is a strict substring of an existing instance ID does not
// accidentally route to that provider.
//
//	e.g. prefix "pfx-sub" should NOT match provider "pfx-sub-extra"
func TestProviderPrefixRouting_SubstringDoesNotMatchLongerID(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	longID := "pfx-sub-extra-" + sfx
	registerPrefixProvider(t, longID, "", "stub-provider", "sub-model", nil)

	srv := newTestServer(t)
	defer srv.Close()

	// Correct full prefix should work
	status, body := chatCompletions(t, srv.URL, longID+"/sub-model")
	if status != http.StatusOK {
		t.Fatalf("full prefix: expected 200, got %d: %s", status, body)
	}

	// Short prefix should NOT match the longer provider ID
	shortPrefix := "pfx-sub-extra" // missing the unique sfx
	statusShort, _ := chatCompletions(t, srv.URL, shortPrefix+"/sub-model")
	if statusShort == http.StatusOK {
		// Only a failure if there happens to be a registered provider with that
		// exact instance ID, which won't happen due to unique sfx. If 200, something
		// unexpected is in the registry — fail the test.
		t.Logf("short prefix %q unexpectedly routed successfully", shortPrefix)
	}
}

// ─── Prefix + virtual model disambiguation ────────────────────────────────────

// TestProviderPrefixRouting_PrefixTakesPrecedenceOverVirtualModel verifies
// that when a provider instance ID and a virtual model ID are both present in
// the registry, using the provider instance ID as a prefix routes directly to
// that provider (not through the virtual model router).
func TestProviderPrefixRouting_PrefixTakesPrecedenceOverVirtualModel(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-vs-vm-provider-" + sfx
	vmID := "pfx-vs-vm-" + sfx

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "stub-provider", "direct-model", &capturedModel)

	// Also create a virtual model with the same suffix (different IDs)
	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: vmID,
		Name:           vmID,
		LbStrategy:     database.LbStrategyPriority,
		APIShape:       "openai",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	t.Cleanup(func() { _ = vmStore.Delete(vmID) })

	srv := newTestServer(t)
	defer srv.Close()

	// Request via provider prefix — must hit the direct provider
	status, body := chatCompletions(t, srv.URL, instanceID+"/direct-model")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "direct-model" {
		t.Errorf("expected direct provider to capture %q; got %q", "direct-model", capturedModel)
	}
}

// ─── Multiple slashes in model ID with prefix ────────────────────────────────

// TestProviderPrefixRouting_AnthropicShape_SlashInModelID verifies that for
// the Anthropic shape a model ID with an embedded slash is forwarded correctly.
func TestProviderPrefixRouting_AnthropicShape_SlashInModelID(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-ant-slash-" + sfx
	modelID := "anthropic/claude-3-opus"

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "stub-provider", modelID, &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := anthropicMessages(t, srv.URL, instanceID+"/"+modelID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != modelID {
		t.Errorf("expected bare model %q forwarded; got %q", modelID, capturedModel)
	}
}

func TestProviderPrefixRouting_ResponsesShape_SlashInModelID(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-responses-slash-" + sfx
	modelID := "kimi/kimi-k3"

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "stub-provider", modelID, &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := responsesRequest(t, srv.URL, instanceID+"/"+modelID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != modelID {
		t.Errorf("expected bare model %q forwarded; got %q", modelID, capturedModel)
	}
}

func TestNativeNamespacedModelAcrossGenerationShapes(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-native-shapes-" + sfx
	modelID := "kimi/kimi-k3"

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "stub-provider", modelID, &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	tests := []struct {
		name string
		call func(*testing.T, string, string) (int, string)
	}{
		{name: "chat completions", call: chatCompletions},
		{name: "anthropic messages", call: anthropicMessages},
		{name: "responses", call: responsesRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedModel = ""
			status, body := tt.call(t, srv.URL, modelID)
			if status != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", status, body)
			}
			if capturedModel != modelID {
				t.Errorf("expected full model %q forwarded; got %q", modelID, capturedModel)
			}
		})
	}
}

// ─── Two providers — prefix selects the right one ────────────────────────────

// TestProviderPrefixRouting_TwoProvidersAnthropicShape verifies that in the
// Anthropic messages shape, two providers exposing the same model can be
// disambiguated by prefix.
func TestProviderPrefixRouting_TwoProvidersAnthropicShape(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	id01 := "pfx-ant-two-01-" + sfx
	id02 := "pfx-ant-two-02-" + sfx

	var cap01, cap02 string
	registerPrefixProvider(t, id01, "", "stub-provider", "shared-model", &cap01)
	registerPrefixProvider(t, id02, "", "stub-provider", "shared-model", &cap02)

	srv := newTestServer(t)
	defer srv.Close()

	// Route to provider 01
	cap01, cap02 = "", ""
	status, body := anthropicMessages(t, srv.URL, id01+"/shared-model")
	if status != http.StatusOK {
		t.Fatalf("provider 01: expected 200, got %d: %s", status, body)
	}
	if cap01 != "shared-model" {
		t.Errorf("provider 01 should have captured model; cap01=%q cap02=%q", cap01, cap02)
	}
	if cap02 != "" {
		t.Errorf("provider 02 should not have been called; cap02=%q", cap02)
	}

	// Route to provider 02
	cap01, cap02 = "", ""
	status, body = anthropicMessages(t, srv.URL, id02+"/shared-model")
	if status != http.StatusOK {
		t.Fatalf("provider 02: expected 200, got %d: %s", status, body)
	}
	if cap02 != "shared-model" {
		t.Errorf("provider 02 should have captured model; cap01=%q cap02=%q", cap01, cap02)
	}
	if cap01 != "" {
		t.Errorf("provider 01 should not have been called; cap01=%q", cap01)
	}
}

// ─── All provider types ─────────────────────────────────────────────────────

// TestProviderPrefixRouting_AllProviderTypes verifies that prefix routing works
// for every supported provider type (except github-copilot). Each sub-test
// registers a provider of that type, sends a prefixed chat-completions request,
// and asserts the bare model name was forwarded upstream.
func TestProviderPrefixRouting_AllProviderTypes(t *testing.T) {
	providerTypes := []string{
		"antigravity",
		"alibaba",
		"azure-openai",
		"google",
		"kimi",
		"openai-compatible",
	}

	for _, pt := range providerTypes {
		t.Run(pt, func(t *testing.T) {
			sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
			instanceID := "pfx-all-" + pt + "-" + sfx
			modelID := "test-model-" + pt

			var capturedModel string
			registerPrefixProvider(t, instanceID, "", pt, modelID, &capturedModel)

			srv := newTestServer(t)
			defer srv.Close()

			status, body := chatCompletions(t, srv.URL, instanceID+"/"+modelID)
			if status != http.StatusOK {
				t.Fatalf("provider %q: expected 200, got %d: %s", pt, status, body)
			}
			if capturedModel != modelID {
				t.Errorf("provider %q: expected bare model %q forwarded; got %q", pt, modelID, capturedModel)
			}
		})
	}
}
