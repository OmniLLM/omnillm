package routes

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"omnillm/internal/database"
	openaipkg "omnillm/internal/providers/openai"
	"omnillm/internal/registry"
)

func TestOpenAIOAuthReauthenticationHoldsLeaseUntilCancellation(t *testing.T) {
	resetAdminTestState(t)
	const id = "oauth-lease-provider"
	store := database.NewProviderInstanceStore()
	if _, err := store.Create(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "openai", Name: "OpenAI"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	reg := registry.GetProviderRegistry()
	if err := reg.Register(openaipkg.NewProvider(id, "OpenAI"), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	openaiOAuthMu.Lock()
	openaiOAuthResults[id] = &openaiOAuthResult{Done: true, ProviderID: id}
	openaiOAuthMu.Unlock()
	started := make(chan struct{})
	oldListen := listenOpenAIOAuth
	listenOpenAIOAuth = func(ctx context.Context, _ string, _ time.Duration) (*openaipkg.CallbackResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() {
		cancelOpenAIOAuth(id)
		listenOpenAIOAuth = oldListen
		resetAdminTestState(t)
		reg.WaitForPendingSaves()
		_ = store.Delete(id)
	})

	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/openai/start-oauth",
		`{"provider_id":"oauth-lease-provider"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	openaiOAuthMu.Lock()
	stale := openaiOAuthResults[id]
	openaiOAuthMu.Unlock()
	if stale != nil {
		t.Fatal("new OAuth flow retained a previous completion result")
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth callback listener did not start")
	}
	if _, err := reg.TryLockIdentity(id); !errors.Is(err, registry.ErrProviderBusy) {
		t.Fatalf("identity lock during reauthentication = %v, want busy", err)
	}
	busy := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch, "/api/admin/providers/"+id+"/identity", `{"new_id":"openai-after-auth"}`,
	)
	if busy.Code != http.StatusConflict {
		t.Fatalf("rename during OAuth = %d: %s", busy.Code, busy.Body.String())
	}
	cancelOpenAIOAuth(id)
	deadline := time.After(5 * time.Second)
	for {
		unlock, err := reg.TryLockIdentity(id)
		if err == nil {
			unlock()
			break
		}
		select {
		case <-deadline:
			t.Fatalf("provider lease remained held after cancellation: %v", err)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestOpenAIOAuthNewProviderKeepsReservedUUID(t *testing.T) {
	resetAdminTestState(t)
	reg := registry.GetProviderRegistry()
	oldListen, oldExchange := listenOpenAIOAuth, exchangeOpenAICode
	listenOpenAIOAuth = func(context.Context, string, time.Duration) (*openaipkg.CallbackResult, error) {
		return &openaipkg.CallbackResult{Code: "code"}, nil
	}
	exchangeOpenAICode = func(string, string) (*openaipkg.TokenResponse, error) {
		return &openaipkg.TokenResponse{AccessToken: "test-access", RefreshToken: "test-refresh", ExpiresIn: 3600}, nil
	}
	t.Cleanup(func() {
		listenOpenAIOAuth, exchangeOpenAICode = oldListen, oldExchange
		resetAdminTestState(t)
		reg.WaitForPendingSaves()
	})
	response := performJSONRequest(t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/openai/start-oauth", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
	}](t, response)
	requireProviderUUID(t, result.ProviderID)
	deadline := time.After(5 * time.Second)
	for {
		openaiOAuthMu.Lock()
		completed := openaiOAuthResults[result.ProviderID]
		openaiOAuthMu.Unlock()
		if completed != nil && completed.Done {
			if completed.Error != "" || completed.ProviderID != result.ProviderID {
				t.Fatalf("OAuth completion: %#v", completed)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("OAuth did not complete")
		case <-time.After(time.Millisecond):
		}
	}
	if !reg.IsRegistered(result.ProviderID) {
		t.Fatalf("provider %q was not registered", result.ProviderID)
	}
	record, err := database.NewTokenStore().Get(result.ProviderID)
	if err != nil || record == nil {
		t.Fatalf("OAuth token: %#v, %v", record, err)
	}
	reg.WaitForPendingSaves()
	rename := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/"+result.ProviderID+"/identity", `{"new_id":"openai-custom"}`,
	)
	if rename.Code != http.StatusOK {
		t.Fatalf("rename OAuth provider: %d %s", rename.Code, rename.Body.String())
	}
	provider, err := reg.GetProvider("openai-custom")
	if err != nil || provider.GetToken() != "test-access" {
		t.Fatalf("renamed OAuth provider credentials: %v", err)
	}
	record, err = database.NewTokenStore().Get("openai-custom")
	if err != nil || record == nil {
		t.Fatalf("renamed OAuth token: %#v, %v", record, err)
	}
}

func TestOpenAIOAuthReauthenticationKeepsLegacyIdentity(t *testing.T) {
	resetAdminTestState(t)
	const id = "legacy-openai-id"
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "openai", Name: "Custom", Subtitle: "custom-alias", Priority: 4}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	provider := openaipkg.NewProvider(id, "Custom")
	if err := provider.ApplyTokens(&openaipkg.TokenResponse{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresIn: 3600}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	oldListen, oldExchange := listenOpenAIOAuth, exchangeOpenAICode
	listenOpenAIOAuth = func(context.Context, string, time.Duration) (*openaipkg.CallbackResult, error) {
		return &openaipkg.CallbackResult{Code: "code"}, nil
	}
	exchangeOpenAICode = func(string, string) (*openaipkg.TokenResponse, error) {
		return &openaipkg.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600}, nil
	}
	t.Cleanup(func() {
		listenOpenAIOAuth, exchangeOpenAICode = oldListen, oldExchange
		resetAdminTestState(t)
		reg.WaitForPendingSaves()
	})
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/openai/start-oauth",
		`{"provider_id":"legacy-openai-id"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	deadline := time.After(5 * time.Second)
	for {
		openaiOAuthMu.Lock()
		completed := openaiOAuthResults[id]
		_, active := openaiOAuthActive[id]
		openaiOAuthMu.Unlock()
		if completed != nil && completed.Done && !active {
			if completed.Error != "" || completed.IsNew || completed.ProviderID != id {
				t.Fatalf("reauthentication result: %#v", completed)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("reauthentication did not complete")
		case <-time.After(time.Millisecond):
		}
	}
	if !reg.IsRegistered(id) || provider.GetToken() != "new-access" {
		t.Fatal("reauthentication changed the identity or failed to update credentials")
	}
	record, err := store.Get(id)
	if err != nil || record == nil || record.Name != "Custom" || record.Subtitle != "custom-alias" || record.Priority != 4 {
		t.Fatalf("provider metadata after reauthentication: %#v, %v", record, err)
	}
}
