package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/database"
)

func TestProviderDisplayNameQualificationAcrossDialects(t *testing.T) {
	suffix := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	modelID := "reference-dialect-model-" + suffix
	displayName := "Reference Dialect Provider " + suffix
	var captured []string
	providerID := registerStubProvider(t, modelID, func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		captured = append(captured, request.Model)
		return &cif.CanonicalResponse{ID: "reference-response", Model: request.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: providerID, ProviderID: "stub-provider", Name: displayName, Subtitle: "reference-alias-" + suffix, Activated: true}); err != nil {
		t.Fatalf("save identity: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(providerID) })

	srv := newTestServer(t)
	defer srv.Close()
	qualified := displayName + "/" + modelID
	tests := []struct {
		name, path, body string
		headers          map[string]string
	}{
		{name: "chat", path: "/v1/chat/completions", body: fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, qualified)},
		{name: "messages", path: "/v1/messages", body: fmt.Sprintf(`{"model":%q,"max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`, qualified), headers: map[string]string{"anthropic-version": "2023-06-01"}},
		{name: "responses", path: "/v1/responses", body: fmt.Sprintf(`{"model":%q,"input":"ping"}`, qualified)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postJSON(t, srv.URL+test.path, test.body, test.headers)
			body := readBody(t, response)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	if len(captured) != len(tests) {
		t.Fatalf("captured %d requests, want %d", len(captured), len(tests))
	}
	for _, model := range captured {
		if model != modelID {
			t.Fatalf("captured model=%q, want %q", model, modelID)
		}
	}
}
