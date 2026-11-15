package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

func TestSecurityAllowLocalEndpointsOptIn(t *testing.T) {
	original := getSecurityOptions()
	defer ConfigureSecurityOptions(original)

	router := newAdminTestRouter()

	ConfigureSecurityOptions(SecurityOptions{AllowLocalEndpoints: false})
	denied := performJSONRequest(t, router, http.MethodPost, "/api/admin/providers/auth-and-create/openai-compatible", `{"endpoint":"http://127.0.0.1:11434/v1","apiKey":"test-key"}`)
	if denied.Code == http.StatusOK {
		t.Fatalf("expected localhost endpoint to be rejected when AllowLocalEndpoints=false, got 200: %s", denied.Body.String())
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"local-model"}]}`))
	}))
	defer upstream.Close()

	ConfigureSecurityOptions(SecurityOptions{AllowLocalEndpoints: true})
	allowed := performJSONRequest(t, router, http.MethodPost, "/api/admin/providers/auth-and-create/openai-compatible", `{"endpoint":"`+upstream.URL+`/v1","apiKey":"test-key"}`)
	if allowed.Code != http.StatusOK {
		t.Fatalf("expected localhost endpoint to succeed when AllowLocalEndpoints=true, got %d: %s", allowed.Code, allowed.Body.String())
	}
	payload := decodeJSONBody[map[string]any](t, allowed)
	providerMap, _ := payload["provider"].(map[string]any)
	instanceID, _ := providerMap["id"].(string)
	if instanceID == "" {
		t.Fatalf("missing provider id in response: %v", payload)
	}
	defer func() {
		_ = performJSONRequest(t, router, http.MethodDelete, "/api/admin/providers/"+instanceID, "")
		registry.GetProviderRegistry().WaitForPendingSaves()
	}()

	// Verify SetupAuth populated allowLocal immediately for runtime model discovery.
	rawProvider, err := registry.GetProviderRegistry().GetProvider(instanceID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	p, ok := rawProvider.(*openaicompatprovider.Provider)
	if !ok {
		t.Fatalf("expected *openaicompatprovider.Provider, got %T", rawProvider)
	}
	if cfg := p.GetConfig(); cfg["allow_local_endpoints"] != true {
		t.Fatalf("expected allow_local_endpoints=true in config, got %v", cfg)
	}
	models, err := p.GetModels()
	if err != nil || models == nil || len(models.Data) != 1 || models.Data[0].ID != "local-model" {
		t.Fatalf("expected GetModels to reach local upstream when opted in, got models=%+v err=%v", models, err)
	}
	_ = types.AuthOptions{}
}
