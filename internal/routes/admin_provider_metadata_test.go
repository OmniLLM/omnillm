package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	antigravity "omnillm/internal/providers/antigravity"
	"omnillm/internal/providers/copilot"
	openai "omnillm/internal/providers/openai"
	compatible "omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

func registerMetadataFixture(t *testing.T, id, alias string, provider types.Provider) {
	t.Helper()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{
		InstanceID: id, ProviderID: provider.GetID(), Name: id, Subtitle: alias,
	}); err != nil {
		t.Fatal(err)
	}
	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reg.WaitForPendingSaves()
		_ = reg.Remove(id)
		reg.WaitForPendingSaves()
		_ = store.Delete(id)
	})
}

func TestProviderMetadataOAuthAndRedaction(t *testing.T) {
	id := "metadata-antigravity"
	registerMetadataFixture(t, id, "meta-alias", antigravity.NewProvider(id, "Metadata"))
	if err := database.NewTokenStore().Save(id, map[string]any{
		"client_id": "public-client.apps.googleusercontent.com", "client_secret": "CLIENT_SECRET_SENTINEL",
		"access_token": "ACCESS_TOKEN_SENTINEL", "refresh_token": "REFRESH_TOKEN_SENTINEL",
		"auth_url": "https://example.test/?state=STATE_SENTINEL", "code_verifier": "PKCE_SENTINEL",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.NewProviderConfigStore().Save(id, map[string]any{
		"secret": "CONFIG_SECRET_SENTINEL", "unknown": "UNKNOWN_SENTINEL",
	}); err != nil {
		t.Fatal(err)
	}

	reg := registry.GetProviderRegistry()
	reg.WaitForPendingSaves()
	var logs bytes.Buffer
	oldLogger := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = oldLogger })
	activeAuthFlowMu.Lock()
	previousFlow := activeAuthFlow
	activeAuthFlow = &authFlowState{
		ProviderID: id, Status: "awaiting_user",
		InstructionURL: "https://accounts.google.com/o/oauth2/v2/auth?state=FLOW_STATE_SENTINEL",
		UserCode:       "USER_CODE_SENTINEL", deviceCode: "DEVICE_CODE_SENTINEL", codeVerifier: "FLOW_PKCE_SENTINEL",
	}
	activeAuthFlowMu.Unlock()
	t.Cleanup(func() {
		activeAuthFlowMu.Lock()
		activeAuthFlow = previousFlow
		activeAuthFlowMu.Unlock()
	})
	router := newAdminTestRouter()
	providerLoginFlows.Lock()
	before := len(providerLoginFlows.values)
	providerLoginFlows.Unlock()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/meta-alias/metadata", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"id": id, "authorization_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
		"token_endpoint": "https://oauth2.googleapis.com/token", "oauth_client_id": "public-client.apps.googleusercontent.com",
	} {
		if result[key] != want {
			t.Errorf("%s=%v, want %q", key, result[key], want)
		}
	}
	for _, secret := range []string{"CLIENT_SECRET_SENTINEL", "ACCESS_TOKEN_SENTINEL", "REFRESH_TOKEN_SENTINEL", "STATE_SENTINEL", "PKCE_SENTINEL", "USER_CODE_SENTINEL", "DEVICE_CODE_SENTINEL", "CONFIG_SECRET_SENTINEL", "UNKNOWN_SENTINEL"} {
		if strings.Contains(recorder.Body.String()+logs.String(), secret) {
			t.Errorf("metadata or logs exposed %s", secret)
		}
	}
	providerLoginFlows.Lock()
	after := len(providerLoginFlows.values)
	providerLoginFlows.Unlock()
	if after != before {
		t.Fatal("metadata inspection started an OAuth flow")
	}
}

func TestProviderMetadataEndpointOriginAndNoFabricatedOAuth(t *testing.T) {
	id := "metadata-compatible"
	registerMetadataFixture(t, id, "compatible-meta", compatible.NewProvider(id, "Metadata"))
	if err := database.NewProviderConfigStore().Save(id, map[string]any{
		"base_url":   "https://user:USERINFO_SECRET@api.example.test:8443/PATH_SECRET?key=QUERY_SECRET#FRAGMENT_SECRET",
		"api_format": "responses", "api_key": "API_KEY_SECRET", "unknown": "UNKNOWN_SECRET",
	}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	newAdminTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/compatible-meta/metadata", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["api_endpoint_origin"] != "https://api.example.test:8443" {
		t.Errorf("origin=%v", result["api_endpoint_origin"])
	}
	if _, ok := result["oauth_client_id"]; ok {
		t.Error("API-key provider received fabricated OAuth client ID")
	}
	if _, ok := result["authorization_endpoint"]; ok {
		t.Error("API-key provider received fabricated authorization endpoint")
	}
	if strings.Contains(recorder.Body.String(), "SECRET") {
		t.Fatalf("metadata exposed secret: %s", recorder.Body.String())
	}
}

func TestProviderMetadataMissingAndMalformedCredentialRecord(t *testing.T) {
	id := "metadata-antigravity-missing"
	registerMetadataFixture(t, id, "meta-missing", antigravity.NewProvider(id, "Metadata"))
	router := newAdminTestRouter()
	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/"+id+"/metadata", nil))
		return recorder
	}
	result := request()
	if result.Code != http.StatusOK || strings.Contains(result.Body.String(), "oauth_client_id") {
		t.Fatalf("missing credentials status=%d body=%s", result.Code, result.Body.String())
	}
	if err := database.NewTokenStore().Save(id, map[string]any{"client_id": map[string]string{"secret": "MALFORMED_SECRET"}}); err != nil {
		t.Fatal(err)
	}
	result = request()
	if result.Code != http.StatusInternalServerError || strings.Contains(result.Body.String(), "MALFORMED_SECRET") {
		t.Fatalf("malformed credentials status=%d body=%s", result.Code, result.Body.String())
	}
}

func TestProviderMetadataKnownOAuthProviders(t *testing.T) {
	for _, test := range []struct {
		name, id, endpoint, clientID string
		provider                     types.Provider
	}{
		{name: "openai", id: "metadata-openai", endpoint: "https://auth.openai.com/oauth/authorize", clientID: openai.OAuthClientID, provider: openai.NewProvider("metadata-openai", "OpenAI")},
		{name: "copilot", id: "metadata-copilot", endpoint: "https://github.com/login/device/code", clientID: "Iv1.b507a08c87ecfe98", provider: copilot.NewGitHubCopilotProvider("metadata-copilot", "Copilot")},
	} {
		t.Run(test.name, func(t *testing.T) {
			registerMetadataFixture(t, test.id, test.id+"-alias", test.provider)
			recorder := httptest.NewRecorder()
			newAdminTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/"+test.id+"/metadata", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("metadata status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var result map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["authorization_endpoint"] != test.endpoint || result["oauth_client_id"] != test.clientID {
				t.Errorf("unexpected OAuth metadata: %v", result)
			}
		})
	}
}

func TestProviderMetadataAmbiguousReference(t *testing.T) {
	store := database.NewProviderInstanceStore()
	for _, id := range []string{"metadata-ambiguous-a", "metadata-ambiguous-b"} {
		if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "google", Name: id, Subtitle: "metadata-ambiguous"}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Delete(id) })
	}
	recorder := httptest.NewRecorder()
	newAdminTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/metadata-ambiguous/metadata", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "ambiguous_provider_reference") {
		t.Fatalf("unexpected result %d: %s", recorder.Code, recorder.Body.String())
	}
}
