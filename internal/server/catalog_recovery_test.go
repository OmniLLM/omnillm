package server

import (
	"context"
	"encoding/json"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"omnillm/internal/testcompat"
	"testing"
)

func TestCatalogRefreshRestoresAllClientToolHistories(t *testing.T) {
	var expected []testcompat.ToolExchange
	id := registerStubProvider(t, testcompat.Model, func(_ context.Context, r *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		assertCompatibilityExchanges(t, r.Messages, expected)
		return &cif.CanonicalResponse{ID: "catalog-final", Model: r.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: testcompat.FinalAnswer}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	raw, err := registry.GetProviderRegistry().GetProvider(id)
	if err != nil {
		t.Fatal(err)
	}
	p := raw.(*stubProvider)
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: p.GetID(), Name: id}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t)
	defer srv.Close()
	for _, fixture := range testcompat.ClientCacheFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			expected = fixture.Exchanges
			// Activation schedules persistence that borrows the provider; finish it
			// before replacing the fixture's lifecycle and catalog snapshot.
			registry.GetProviderRegistry().WaitForPendingSaves()
			p.models = &types.ModelsResponse{Data: []types.Model{{ID: "fallback"}}, Degraded: true}
			// Replace registration to retire a previous successful snapshot.
			if err := registry.GetProviderRegistry().Register(p, false); err != nil {
				t.Fatal(err)
			}
			headers := map[string]string{"X-OmniLLM-Cache": "off"}
			for k, v := range fixture.Headers {
				headers[k] = v
			}
			failed := postJSON(t, srv.URL+fixture.Endpoint, string(fixture.Request), headers)
			_ = readBody(t, failed)
			if failed.StatusCode == http.StatusOK {
				t.Fatal("degraded catalog unexpectedly routed missing model")
			}
			p.models = &types.ModelsResponse{Data: []types.Model{{ID: testcompat.Model}}}
			refresh := postJSON(t, srv.URL+"/api/admin/providers/"+id+"/models/refresh", "", nil)
			body := readBody(t, refresh)
			if refresh.StatusCode != http.StatusOK {
				t.Fatalf("refresh %d: %s", refresh.StatusCode, body)
			}
			response := postJSON(t, srv.URL+fixture.Endpoint, string(fixture.Request), headers)
			body = readBody(t, response)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("recovery %d: %s", response.StatusCode, body)
			}
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(body), &result); err != nil {
				t.Fatal(err)
			}
		})
	}
}
