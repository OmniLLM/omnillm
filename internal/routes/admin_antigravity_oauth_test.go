package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"omnillm/internal/database"
	antigravitypkg "omnillm/internal/providers/antigravity"
	"omnillm/internal/registry"
)

func resetAntigravityOAuthTestState(t *testing.T) {
	t.Helper()
	oldGenerate := generateAntigravityOAuthState
	oldExchange := exchangeAntigravityOAuthCode
	oldDiscover := discoverAntigravityProject
	oldEmail := fetchAntigravityEmail
	agOAuthMu.Lock()
	agOAuthStates = map[string]*antigravityOAuthState{}
	agOAuthActive = map[string]*antigravityOAuthState{}
	agOAuthMu.Unlock()
	t.Cleanup(func() {
		generateAntigravityOAuthState = oldGenerate
		exchangeAntigravityOAuthCode = oldExchange
		discoverAntigravityProject = oldDiscover
		fetchAntigravityEmail = oldEmail
		agOAuthMu.Lock()
		agOAuthStates = map[string]*antigravityOAuthState{}
		agOAuthActive = map[string]*antigravityOAuthState{}
		agOAuthMu.Unlock()
	})
}

func TestAntigravityNewOAuthReservesUUIDAndCleansUpOnFailure(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	exchangeAntigravityOAuthCode = func(string, string, string, string) (*antigravitypkg.OAuthTokenResponse, error) {
		return nil, errors.New("test exchange failure")
	}
	router := newAdminTestRouter()
	response := performJSONRequest(
		t, router, http.MethodPost, "/api/admin/providers/antigravity/start-oauth",
		`{"client_id":"client","client_secret":"secret"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
		State      string `json:"state"`
	}](t, response)
	requireProviderUUID(t, result.ProviderID)
	store := database.NewProviderInstanceStore()
	record, err := store.Get(result.ProviderID)
	if err != nil || record == nil {
		t.Fatalf("reserved provider: %#v, %v", record, err)
	}
	callback := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(callback)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/callback?code=code&state="+url.QueryEscape(result.State), nil)
	handleAntigravityOAuthCallback(ctx)
	record, err = store.Get(result.ProviderID)
	if err != nil || record != nil {
		t.Fatalf("provider after failed OAuth: %#v, %v", record, err)
	}
}

func TestAntigravityReauthenticationHoldsLeaseUntilCancellation(t *testing.T) {
	resetAdminTestState(t)
	resetAntigravityOAuthTestState(t)
	const id = "antigravity-oauth-lease"
	store := database.NewProviderInstanceStore()
	if _, err := store.Create(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "antigravity", Name: "Antigravity"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	reg := registry.GetProviderRegistry()
	if err := reg.Register(antigravitypkg.NewProvider(id, "Antigravity"), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	agOAuthResultsMu.Lock()
	agOAuthResults[id] = &antigravityOAuthResult{Done: true, ProviderID: id}
	agOAuthResultsMu.Unlock()
	t.Cleanup(func() {
		cancelAntigravityOAuth(id)
		resetAdminTestState(t)
		reg.WaitForPendingSaves()
		_ = store.Delete(id)
	})
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/antigravity/start-oauth",
		`{"client_id":"client","client_secret":"secret","provider_id":"antigravity-oauth-lease"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	agOAuthResultsMu.Lock()
	stale := agOAuthResults[id]
	agOAuthResultsMu.Unlock()
	if stale != nil {
		t.Fatal("new OAuth flow retained a previous completion result")
	}
	duplicate := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/antigravity/start-oauth",
		`{"client_id":"client","client_secret":"secret","provider_id":"antigravity-oauth-lease"}`,
	)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate OAuth flow = %d: %s", duplicate.Code, duplicate.Body.String())
	}
	if _, err := reg.TryLockIdentity(id); !errors.Is(err, registry.ErrProviderBusy) {
		t.Fatalf("identity lock during reauthentication = %v, want busy", err)
	}
	busy := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch, "/api/admin/providers/"+id+"/identity", `{"new_id":"antigravity-after-auth"}`,
	)
	if busy.Code != http.StatusConflict {
		t.Fatalf("rename during OAuth = %d: %s", busy.Code, busy.Body.String())
	}
	if !cancelAntigravityOAuth(id) {
		t.Fatal("expected to cancel the pending OAuth state")
	}
	unlock, err := reg.TryLockIdentity(id)
	if err != nil {
		t.Fatalf("identity lock after cancellation: %v", err)
	}
	unlock()
}

func TestAntigravityOAuthKeepsUUIDThroughCompletionAndRename(t *testing.T) {
	resetAdminTestState(t)
	resetAntigravityOAuthTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	exchangeAntigravityOAuthCode = func(string, string, string, string) (*antigravitypkg.OAuthTokenResponse, error) {
		return &antigravitypkg.OAuthTokenResponse{AccessToken: "oauth-access", RefreshToken: "oauth-refresh", ExpiresIn: 3600}, nil
	}
	discoverAntigravityProject = func(string) string { return "project" }
	fetchAntigravityEmail = func(string) string { return "test@example.test" }
	router := newAdminTestRouter()
	response := performJSONRequest(
		t, router, http.MethodPost, "/api/admin/providers/antigravity/start-oauth",
		`{"client_id":"client","client_secret":"secret"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
		State      string `json:"state"`
	}](t, response)
	requireProviderUUID(t, result.ProviderID)
	callback := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(callback)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/callback?code=code&state="+url.QueryEscape(result.State), nil)
	handleAntigravityOAuthCallback(ctx)
	if !strings.Contains(callback.Body.String(), "Authenticated successfully") {
		t.Fatalf("callback failed: %s", callback.Body.String())
	}
	rec, err := database.NewTokenStore().Get(result.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal([]byte(rec.TokenData), &saved); err != nil {
		t.Fatal(err)
	}
	expiry, _ := saved["expires_at"].(float64)
	if expiry < float64(time.Now().Unix()+3500) {
		t.Fatal("OAuth expiry not persisted")
	}
	reg := registry.GetProviderRegistry()
	reg.WaitForPendingSaves()
	if !reg.IsRegistered(result.ProviderID) {
		t.Fatal("OAuth completion changed the reserved provider ID")
	}
	rename := performJSONRequest(
		t, router, http.MethodPatch, "/api/admin/providers/"+result.ProviderID+"/identity", `{"new_id":"antigravity-custom"}`,
	)
	if rename.Code != http.StatusOK {
		t.Fatalf("rename provider: %d %s", rename.Code, rename.Body.String())
	}
	provider, err := reg.GetProvider("antigravity-custom")
	if err != nil || provider.GetToken() != "oauth-access" {
		t.Fatalf("renamed provider credentials: %v", err)
	}
}

func TestAntigravityOAuthReauthenticationKeepsLegacyIdentity(t *testing.T) {
	resetAdminTestState(t)
	resetAntigravityOAuthTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	const id = "legacy-antigravity-id"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "antigravity", Name: "Custom", Subtitle: "custom-alias", Priority: 4}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	provider := antigravitypkg.NewProvider(id, "Custom")
	if err := registry.GetProviderRegistry().Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	exchangeAntigravityOAuthCode = func(string, string, string, string) (*antigravitypkg.OAuthTokenResponse, error) {
		return &antigravitypkg.OAuthTokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"}, nil
	}
	discoverAntigravityProject = func(string) string { return "project" }
	fetchAntigravityEmail = func(string) string { return "test@example.test" }
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/antigravity/start-oauth",
		`{"client_id":"client","client_secret":"secret","provider_id":"legacy-antigravity-id"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
		State      string `json:"state"`
	}](t, response)
	if result.ProviderID != id {
		t.Fatalf("pending ID = %q, want %q", result.ProviderID, id)
	}
	callback := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(callback)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/callback?code=code&state="+url.QueryEscape(result.State), nil)
	handleAntigravityOAuthCallback(ctx)
	if !strings.Contains(callback.Body.String(), "Authenticated successfully") || provider.GetToken() != "new-access" {
		t.Fatalf("reauthentication did not complete: %s", callback.Body.String())
	}
	record, err := store.Get(id)
	if err != nil || record == nil || record.Name != "Custom" || record.Subtitle != "custom-alias" || record.Priority != 4 {
		t.Fatalf("provider metadata after reauthentication: %#v, %v", record, err)
	}
}

func TestAntigravityCancellationConflictKeepsLoginFlow(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	flowID, err := newProviderLoginFlow(providerLoginFlow{Kind: "antigravity", ProviderID: "callback-in-progress"})
	if err != nil {
		t.Fatalf("create login flow: %v", err)
	}
	t.Cleanup(func() {
		providerLoginFlows.Lock()
		delete(providerLoginFlows.values, flowID)
		providerLoginFlows.Unlock()
	})
	response := performJSONRequest(t, newAdminTestRouter(), http.MethodDelete, "/api/admin/providers/login/"+flowID, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("cancel while callback completes = %d: %s", response.Code, response.Body.String())
	}
	providerLoginFlows.Lock()
	_, exists := providerLoginFlows.values[flowID]
	providerLoginFlows.Unlock()
	if !exists {
		t.Fatal("cancel conflict removed the flow before it could be polled")
	}
}

func TestNewAntigravityOAuthStateUsesLowercaseHex(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	state, err := newAntigravityOAuthState("provider", "client", "secret", "http://redirect", true)
	if err != nil {
		t.Fatalf("newAntigravityOAuthState: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(state.State) {
		t.Fatalf("state = %q, want 32 lowercase hexadecimal characters", state.State)
	}
	if getAntigravityOAuthState(state.State) != state {
		t.Fatal("state was not stored")
	}
}

func TestAntigravityStartOAuthFailsClosedOnRandomFailure(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	generateAntigravityOAuthState = func() (string, error) { return "", errors.New("entropy unavailable") }

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "http://gateway.test/api/admin/providers/antigravity/start-oauth", bytes.NewBufferString(`{"client_id":"client","client_secret":"secret","provider_id":"antigravity-test"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	handleAntigravityStartOAuth(context)

	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "auth_url") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "entropy unavailable") {
		t.Fatalf("response leaked internal error: %s", recorder.Body.String())
	}
	agOAuthMu.Lock()
	defer agOAuthMu.Unlock()
	if len(agOAuthStates) != 0 {
		t.Fatalf("stored %d states after generation failure", len(agOAuthStates))
	}
}

func TestAntigravityCallbackRejectsMissingUnknownAndExpiredState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		setup func()
	}{
		{"missing", "?code=code", func() {}},
		{"unknown", "?code=code&state=unknown", func() {}},
		{"expired", "?code=code&state=expired", func() {
			agOAuthMu.Lock()
			agOAuthStates["expired"] = &antigravityOAuthState{State: "expired", Expiry: time.Now().Add(-time.Minute)}
			agOAuthMu.Unlock()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetAntigravityOAuthTestState(t)
			called := false
			exchangeAntigravityOAuthCode = func(string, string, string, string) (*antigravitypkg.OAuthTokenResponse, error) {
				called = true
				return nil, errors.New("unexpected exchange")
			}
			tc.setup()
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/callback"+tc.query, nil)
			handleAntigravityOAuthCallback(context)
			if called {
				t.Fatal("authorization code was exchanged")
			}
			if !strings.Contains(recorder.Body.String(), "OAuth failed") {
				t.Fatalf("response body = %q", recorder.Body.String())
			}
		})
	}
}

func TestAntigravityCallbackConsumesStateAtomically(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	agOAuthMu.Lock()
	agOAuthStates["valid"] = &antigravityOAuthState{ProviderID: "provider", ClientID: "client", ClientSecret: "secret", RedirectURI: "http://redirect", State: "valid", Expiry: time.Now().Add(time.Minute)}
	agOAuthMu.Unlock()

	var exchanges atomic.Int32
	exchangeAntigravityOAuthCode = func(string, string, string, string) (*antigravitypkg.OAuthTokenResponse, error) {
		exchanges.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil, errors.New("stop after exchange")
	}

	start := make(chan struct{})
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			<-start
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/callback?code=code&state=valid", nil)
			handleAntigravityOAuthCallback(context)
			done <- struct{}{}
		}()
	}
	close(start)
	<-done
	<-done
	if got := exchanges.Load(); got != 1 {
		t.Fatalf("token exchanges = %d, want 1", got)
	}
}

func TestAntigravityCallbackConsumesStateBeforeExchangeAndReusesRedirect(t *testing.T) {
	resetAntigravityOAuthTestState(t)
	const redirectURI = "http://gateway.test/exact/callback?a=1"
	agOAuthMu.Lock()
	agOAuthStates["valid"] = &antigravityOAuthState{ProviderID: "provider", ClientID: "client", ClientSecret: "secret", RedirectURI: redirectURI, State: "valid", Expiry: time.Now().Add(time.Minute)}
	agOAuthMu.Unlock()

	exchangeAntigravityOAuthCode = func(clientID, clientSecret, code, gotRedirect string) (*antigravitypkg.OAuthTokenResponse, error) {
		if getAntigravityOAuthState("valid") != nil {
			t.Fatal("state remained stored during token exchange")
		}
		if clientID != "client" || clientSecret != "secret" || code != "code" || gotRedirect != redirectURI {
			t.Fatalf("exchange args = %q %q %q %q", clientID, clientSecret, code, gotRedirect)
		}
		return nil, errors.New("stop after verifying exchange")
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/callback?code="+url.QueryEscape("code")+"&state=valid", nil)
	handleAntigravityOAuthCallback(context)
	if !strings.Contains(recorder.Body.String(), "Token exchange failed") {
		t.Fatalf("response body = %q", recorder.Body.String())
	}
}
