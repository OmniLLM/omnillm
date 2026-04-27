package server

// Tests for the "<providerPrefix>/<modelID>" routing feature.
//
// Each test registers stub providers and fires chat-completions requests whose
// model field contains a prefix.  The adapter capture function verifies that
// the correct provider was chosen and that the bare model name (no prefix) was
// forwarded upstream.
//
// Most tests use the raw instance ID as the prefix (no DB subtitle lookup
// needed).  A dedicated subtitle-resolution test covers the subtitle→instanceID
// mapping path.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// registerPrefixProvider registers a stub provider with the given instanceID
// and model, activates it, and optionally seeds a subtitle in the DB.
// The capturedModel pointer is set by the adapter when a request arrives.
func registerPrefixProvider(
	t *testing.T,
	instanceID, subtitle, providerType, modelID string,
	capturedModel *string,
) {
	t.Helper()

	model := providertypes.Model{ID: modelID, Name: modelID, Provider: instanceID}
	provider := &stubProvider{
		id:         providerType,
		instanceID: instanceID,
		name:       instanceID,
		models:     &providertypes.ModelsResponse{Object: "list", Data: []providertypes.Model{model}},
	}
	adapter := &stubAdapter{
		executeFn: func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			if capturedModel != nil {
				*capturedModel = req.Model
			}
			return &cif.CanonicalResponse{
				ID:    "resp-prefix-test",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "pong"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
	}
	provider.adapter = adapter
	adapter.provider = provider

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register %s: %v", instanceID, err)
	}
	if _, err := reg.AddActive(instanceID); err != nil {
		t.Fatalf("activate %s: %v", instanceID, err)
	}

	// Seed subtitle into DB so resolveProviderPrefix can match it.
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{
		InstanceID: instanceID,
		ProviderID: providerType,
		Name:       instanceID,
		Subtitle:   subtitle,
		Priority:   0,
		Activated:  true,
	}); err != nil {
		t.Fatalf("seed DB record for %s: %v", instanceID, err)
	}

	t.Cleanup(func() {
		_ = reg.Remove(instanceID)
		_ = store.Delete(instanceID)
	})
}

// chatCompletions fires a POST /v1/chat/completions with the given model and
// returns the raw response body and HTTP status.
func chatCompletions(t *testing.T, srvURL, model string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, model)
	resp := postJSON(t, srvURL+"/v1/chat/completions", body, nil)
	return resp.StatusCode, readBody(t, resp)
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestProviderPrefixRouting_ByInstanceID verifies that using the raw instance
// ID as a prefix routes to the correct provider and forwards the bare model name.
func TestProviderPrefixRouting_ByInstanceID(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-instid-" + uniqueSuffix

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "stub-provider", "custom-model", &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, instanceID+"/custom-model")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "custom-model" {
		t.Errorf("expected bare model name %q forwarded; got %q", "custom-model", capturedModel)
	}
}

// TestProviderPrefixRouting_TwoProvidersSameModel verifies that two providers
// exposing the same model ID can be disambiguated by instance-ID prefix.
func TestProviderPrefixRouting_TwoProvidersSameModel(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	id01 := "pfx-two-01-" + uniqueSuffix
	id02 := "pfx-two-02-" + uniqueSuffix

	var captured01, captured02 string
	registerPrefixProvider(t, id01, "", "stub-provider", "same-model", &captured01)
	registerPrefixProvider(t, id02, "", "stub-provider", "same-model", &captured02)

	srv := newTestServer(t)
	defer srv.Close()

	// Provider 01
	captured01, captured02 = "", ""
	status, body := chatCompletions(t, srv.URL, id01+"/same-model")
	if status != http.StatusOK {
		t.Fatalf("provider 01: expected 200, got %d: %s", status, body)
	}
	if captured01 != "same-model" {
		t.Errorf("provider 01 should have received the request; capturedModel=%q", captured01)
	}
	if captured02 != "" {
		t.Errorf("provider 02 should NOT have received the request; capturedModel=%q", captured02)
	}

	// Provider 02
	captured01, captured02 = "", ""
	status, body = chatCompletions(t, srv.URL, id02+"/same-model")
	if status != http.StatusOK {
		t.Fatalf("provider 02: expected 200, got %d: %s", status, body)
	}
	if captured02 != "same-model" {
		t.Errorf("provider 02 should have received the request; capturedModel=%q", captured02)
	}
	if captured01 != "" {
		t.Errorf("provider 01 should NOT have received the request; capturedModel=%q", captured01)
	}
}

// TestProviderPrefixRouting_Copilot verifies the copilot-jzhu-abk/gpt-5-mini
// prefix pattern specifically requested in the issue.
func TestProviderPrefixRouting_Copilot(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-copilot-abk-" + uniqueSuffix

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "copilot", "gpt-5-mini", &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, instanceID+"/gpt-5-mini")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "gpt-5-mini" {
		t.Errorf("expected bare model %q forwarded; got %q", "gpt-5-mini", capturedModel)
	}
}

// TestProviderPrefixRouting_SlashInModelID verifies that a model ID containing
// "/" (e.g. "deepseek-ai/DeepSeek-V4-Flash") is forwarded correctly when only
// the first slash is the separator.
func TestProviderPrefixRouting_SlashInModelID(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-slash-" + uniqueSuffix
	modelID := "deepseek-ai/DeepSeek-V4-Flash"

	var capturedModel string
	registerPrefixProvider(t, instanceID, "", "openai-compat", modelID, &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	// Full request model: "<instanceID>/deepseek-ai/DeepSeek-V4-Flash"
	requestModel := instanceID + "/" + modelID
	status, body := chatCompletions(t, srv.URL, requestModel)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != modelID {
		t.Errorf("expected bare model %q forwarded; got %q", modelID, capturedModel)
	}
}

// TestProviderPrefixRouting_UnknownPrefix verifies that a prefix that matches
// no known instance ID produces an error rather than silently routing elsewhere.
func TestProviderPrefixRouting_UnknownPrefix(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, "totally-unknown-prefix-xyz/gpt-4o")
	if status == http.StatusOK {
		t.Fatalf("expected error response for unknown prefix, got 200: %s", body)
	}
}

// TestProviderPrefixRouting_NoPrefixUnaffected verifies that requests without
// a "/" continue to work with normal provider selection (no regression).
func TestProviderPrefixRouting_NoPrefixUnaffected(t *testing.T) {
	const modelID = "no-prefix-model"

	var capturedModel string
	registerStubProvider(
		t,
		modelID,
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			capturedModel = req.Model
			return &cif.CanonicalResponse{
				ID:    "resp-no-prefix",
				Model: req.Model,
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "pong"}},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, modelID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != modelID {
		t.Errorf("expected model %q, got %q", modelID, capturedModel)
	}
}

// TestProviderPrefixRouting_SubtitleResolution verifies that a subtitle prefix
// is resolved to the correct instance ID via the DB-backed lookup.
func TestProviderPrefixRouting_SubtitleResolution(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-subtitle-" + uniqueSuffix
	subtitle := "mysubtitlepfx" + uniqueSuffix

	var capturedModel string
	registerPrefixProvider(t, instanceID, subtitle, "stub-provider", "subtitle-model", &capturedModel)

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, subtitle+"/subtitle-model")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "subtitle-model" {
		t.Errorf("expected model %q forwarded; got %q", "subtitle-model", capturedModel)
	}
}

// TestProviderPrefixRouting_ResponseModelIsBareName verifies that the model
// field in the JSON response contains the bare model name (no prefix).
func TestProviderPrefixRouting_ResponseModelIsBareName(t *testing.T) {
	uniqueSuffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	instanceID := "pfx-resp-" + uniqueSuffix

	registerPrefixProvider(t, instanceID, "", "stub-provider", "resp-model", nil)

	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(
		t,
		srv.URL+"/v1/chat/completions",
		fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, instanceID+"/resp-model"),
		nil,
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var parsed struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if parsed.Model != "resp-model" {
		t.Errorf("expected response model %q (no prefix), got %q", "resp-model", parsed.Model)
	}
}
