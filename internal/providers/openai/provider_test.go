package openai

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
)

// TestMain provisions a throwaway database: SaveToDB panics when the database
// is uninitialized, and several tests exercise the persistence path.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "openai-provider-test-*")
	if err != nil {
		panic(err)
	}
	if err := database.InitializeDatabase(dir); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestGeneratePKCEProducesValidS256Challenge(t *testing.T) {
	p, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	if p.Verifier == "" || p.Challenge == "" {
		t.Fatal("expected non-empty verifier and challenge")
	}
	// RFC 7636 requires a verifier of 43–128 chars.
	if len(p.Verifier) < 43 || len(p.Verifier) > 128 {
		t.Errorf("verifier length %d outside RFC 7636 range 43-128", len(p.Verifier))
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Errorf("challenge = %q, want S256(verifier) = %q", p.Challenge, want)
	}
	if strings.ContainsAny(p.Challenge, "+/=") {
		t.Errorf("challenge %q must be base64url without padding", p.Challenge)
	}
}

func TestGeneratePKCEIsRandom(t *testing.T) {
	a, _ := GeneratePKCE()
	b, _ := GeneratePKCE()
	if a.Verifier == b.Verifier {
		t.Error("two PKCE verifiers were identical; generation is not random")
	}
}

func TestBuildAuthURLCarriesRequiredParams(t *testing.T) {
	got := BuildAuthURL("test-challenge", "test-state")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BuildAuthURL produced unparseable URL: %v", err)
	}
	q := u.Query()

	for _, tc := range []struct{ key, want string }{
		{"response_type", "code"},
		{"client_id", OAuthClientID},
		{"redirect_uri", RedirectURI()},
		{"code_challenge", "test-challenge"},
		{"code_challenge_method", "S256"},
		{"state", "test-state"},
	} {
		if got := q.Get(tc.key); got != tc.want {
			t.Errorf("query %q = %q, want %q", tc.key, got, tc.want)
		}
	}
	// offline_access is what yields a refresh token; without it the provider
	// would silently stop working after the access token expires.
	if !strings.Contains(q.Get("scope"), "offline_access") {
		t.Errorf("scope %q must include offline_access", q.Get("scope"))
	}
}

func TestRedirectURIMatchesWhitelistedValue(t *testing.T) {
	// The OAuth client accepts exactly this redirect URI; changing it breaks
	// sign-in entirely, so pin it.
	if got, want := RedirectURI(), "http://localhost:1455/auth/callback"; got != want {
		t.Errorf("RedirectURI() = %q, want %q", got, want)
	}
}

func TestBuildAuthURLPinsCompleteQuery(t *testing.T) {
	got := BuildAuthURL("challenge value", "state value")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	endpoint := parsed.Scheme + "://" + parsed.Host + parsed.Path
	if endpoint != oauthAuthorizeURL {
		t.Fatalf("endpoint = %q, want %q", endpoint, oauthAuthorizeURL)
	}
	want := url.Values{
		"client_id":                  {OAuthClientID},
		"code_challenge":             {"challenge value"},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"prompt":                     {"login"},
		"redirect_uri":               {RedirectURI()},
		"response_type":              {"code"},
		"scope":                      {OAuthScopes},
		"state":                      {"state value"},
	}
	if parsed.Query().Encode() != want.Encode() {
		t.Fatalf("query = %q, want %q", parsed.Query().Encode(), want.Encode())
	}
}

func TestTokenRequestsUseJSONAndPreserveWireFields(t *testing.T) {
	old := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = old })
	requests := make(chan map[string]string, 2)
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			t.Fatalf("unexpected headers: %v", request.Header)
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests <- payload
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"access","refresh_token":"retained"}`)), Header: make(http.Header)}, nil
	})}

	exchanged, err := ExchangeCode("code", "verifier")
	if err != nil || exchanged.RefreshToken != "retained" {
		t.Fatalf("ExchangeCode = %#v, %v", exchanged, err)
	}
	codePayload := <-requests
	wantCode := map[string]string{"grant_type": "authorization_code", "client_id": OAuthClientID, "code": "code", "redirect_uri": RedirectURI(), "code_verifier": "verifier"}
	if !maps.Equal(codePayload, wantCode) {
		t.Fatalf("code payload = %#v, want %#v", codePayload, wantCode)
	}

	if _, err := RefreshAccessToken("refresh"); err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	refreshPayload := <-requests
	wantRefresh := map[string]string{"grant_type": "refresh_token", "client_id": OAuthClientID, "refresh_token": "refresh", "scope": OAuthScopes}
	if !maps.Equal(refreshPayload, wantRefresh) {
		t.Fatalf("refresh payload = %#v, want %#v", refreshPayload, wantRefresh)
	}
}

func TestTokenRequestsSurfaceOAuthErrors(t *testing.T) {
	old := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = old })
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant","error_description":"expired"}`)), Header: make(http.Header)}, nil
	})}
	if _, err := ExchangeCode("code", "verifier"); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("ExchangeCode error = %v", err)
	}
}

func TestTokenResponseErrorsDoNotExposeRawCredentialBody(t *testing.T) {
	old := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = old })
	const credentialSentinel = "refresh-secret-must-not-leak"
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"refresh_token":"` + credentialSentinel + `"}`)), Header: make(http.Header)}, nil
	})}
	_, err := RefreshAccessToken("submitted-refresh")
	if err == nil {
		t.Fatal("expected missing access token error")
	}
	if strings.Contains(err.Error(), credentialSentinel) {
		t.Fatalf("token error exposed response credential: %v", err)
	}
}

type openAIRoundTripFunc func(*http.Request) (*http.Response, error)

func (f openAIRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// makeJWT builds an unsigned JWT with the given claims payload.
func makeJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(body) + ".signature"
}

func TestParseTokenClaimsExtractsAccountID(t *testing.T) {
	token := makeJWT(t, map[string]interface{}{
		"email": "user@example.com",
		"exp":   1786333895,
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-123",
			"chatgpt_plan_type":  "business",
		},
	})

	claims, err := ParseTokenClaims(token)
	if err != nil {
		t.Fatalf("ParseTokenClaims: %v", err)
	}
	if claims.AccountID != "acct-123" {
		t.Errorf("AccountID = %q, want acct-123", claims.AccountID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q, want user@example.com", claims.Email)
	}
	if claims.PlanType != "business" {
		t.Errorf("PlanType = %q, want business", claims.PlanType)
	}
	if claims.ExpiresAt != 1786333895 {
		t.Errorf("ExpiresAt = %d, want 1786333895", claims.ExpiresAt)
	}
}

func TestParseTokenClaimsRejectsMalformed(t *testing.T) {
	for name, token := range map[string]string{
		"empty":       "",
		"not a jwt":   "abc",
		"one segment": "onlyheader",
		"bad base64":  "header.!!!not-base64!!!.sig",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTokenClaims(token); err == nil {
				t.Errorf("expected error for %q, got nil", token)
			}
		})
	}
}

func TestClaimsFromTokenResponseFallsBackToAccessToken(t *testing.T) {
	// Refresh responses may omit the ID token; the account ID must still be
	// recoverable from the access token.
	access := makeJWT(t, map[string]interface{}{
		"email": "fallback@example.com",
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-fallback",
		},
	})
	claims := ClaimsFromTokenResponse(&TokenResponse{AccessToken: access})
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.AccountID != "acct-fallback" {
		t.Errorf("AccountID = %q, want acct-fallback", claims.AccountID)
	}
}

func TestClaimsFromTokenResponsePrefersIDToken(t *testing.T) {
	idToken := makeJWT(t, map[string]interface{}{
		"https://api.openai.com/auth": map[string]interface{}{"chatgpt_account_id": "from-id"},
	})
	access := makeJWT(t, map[string]interface{}{
		"https://api.openai.com/auth": map[string]interface{}{"chatgpt_account_id": "from-access"},
	})
	claims := ClaimsFromTokenResponse(&TokenResponse{IDToken: idToken, AccessToken: access})
	if claims.AccountID != "from-id" {
		t.Errorf("AccountID = %q, want from-id", claims.AccountID)
	}
}

func TestClaimsFromTokenResponseNilSafe(t *testing.T) {
	if got := ClaimsFromTokenResponse(nil); got != nil {
		t.Errorf("expected nil for nil response, got %+v", got)
	}
}

func TestGetHeadersIncludesAccountID(t *testing.T) {
	p := NewProvider("openai-1", "")
	p.accessToken = "tok-abc"
	p.accountID = "acct-xyz"
	// Far-future expiry so GetToken does not attempt a refresh.
	p.expiresAt = time.Now().Add(24 * time.Hour).Unix()

	h := p.GetHeaders(false)

	if got, want := h["Authorization"], "Bearer tok-abc"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	// Without this header the Codex backend rejects the request.
	if got, want := h["chatgpt-account-id"], "acct-xyz"; got != want {
		t.Errorf("chatgpt-account-id = %q, want %q", got, want)
	}
	if got, want := h["OpenAI-Beta"], "responses=experimental"; got != want {
		t.Errorf("OpenAI-Beta = %q, want %q", got, want)
	}
	if got, want := h["originator"], "codex_cli_rs"; got != want {
		t.Errorf("originator = %q, want %q", got, want)
	}
	if h["session_id"] == "" {
		t.Error("session_id must be set")
	}
}

func TestGetHeadersOmitsAccountIDWhenUnknown(t *testing.T) {
	p := NewProvider("openai-1", "")
	p.accessToken = "tok"
	p.expiresAt = time.Now().Add(time.Hour).Unix()

	if _, ok := p.GetHeaders(false)["chatgpt-account-id"]; ok {
		t.Error("chatgpt-account-id should be absent when no account ID is known")
	}
}

func TestGetTokenSkipsRefreshWithoutRefreshToken(t *testing.T) {
	// An expired token with no refresh token must return as-is rather than
	// making a doomed network call.
	p := NewProvider("openai-1", "")
	p.accessToken = "stale"
	p.expiresAt = time.Now().Add(-time.Hour).Unix()

	if got := p.GetToken(); got != "stale" {
		t.Errorf("GetToken() = %q, want stale", got)
	}
}

func TestRefreshTokenErrorsWithoutRefreshToken(t *testing.T) {
	p := NewProvider("openai-1", "")
	err := p.RefreshToken()
	if err == nil {
		t.Fatal("expected an error when no refresh token is stored")
	}
	if !strings.Contains(err.Error(), "no refresh token") {
		t.Errorf("error = %v, want it to mention the missing refresh token", err)
	}
}

func TestRefreshTokenRecoversFromNewerDurableRotatedToken(t *testing.T) {
	saveTestProviderInstance(t, "openai-rotation-recovery")
	p := NewProvider("openai-rotation-recovery", "")
	p.accessToken = "old-access"
	p.refreshToken = "consumed-refresh"
	p.expiresAt = time.Now().Add(-time.Hour).Unix()
	if err := p.SaveToDB(); err != nil {
		t.Fatalf("save initial tokens: %v", err)
	}

	oldClient := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = oldClient })
	var requests atomic.Int32
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode token request: %v", err)
		}
		switch requests.Add(1) {
		case 1:
			if payload["refresh_token"] != "consumed-refresh" {
				t.Fatalf("first refresh token = %q", payload["refresh_token"])
			}
			if err := database.NewTokenStore().Save(p.instanceID, map[string]interface{}{
				"access_token": "winner-access", "refresh_token": "winner-refresh",
			}); err != nil {
				t.Fatalf("persist winning rotation: %v", err)
			}
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Your refresh token has already been used to generate a new access token. Please try signing in again.","type":"invalid_request_error","code":"refresh_token_already_used"}}`)), Header: make(http.Header)}, nil
		case 2:
			if payload["refresh_token"] != "winner-refresh" {
				t.Fatalf("recovery refresh token = %q, want winner-refresh", payload["refresh_token"])
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"recovered-access","refresh_token":"recovered-refresh"}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected token request %d", requests.Load())
			return nil, nil
		}
	})}

	if err := p.RefreshToken(); err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("token requests = %d, want 2", requests.Load())
	}
	if p.accessToken != "recovered-access" || p.refreshToken != "recovered-refresh" {
		t.Fatalf("recovered tokens = %q, %q", p.accessToken, p.refreshToken)
	}
}

func TestRefreshTokenRetiresRejectedTokenAndRequiresSignIn(t *testing.T) {
	saveTestProviderInstance(t, "openai-rotation-terminal")
	p := NewProvider("openai-rotation-terminal", "")
	p.accessToken = "still-usable-access"
	p.refreshToken = "consumed-refresh"
	p.expiresAt = time.Now().Add(-time.Hour).Unix()
	if err := p.SaveToDB(); err != nil {
		t.Fatalf("save initial tokens: %v", err)
	}

	oldClient := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = oldClient })
	var requests atomic.Int32
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Your refresh token has already been used to generate a new access token. Please try signing in again.","type":"invalid_request_error","code":"refresh_token_already_used"}}`)), Header: make(http.Header)}, nil
	})}

	err := p.RefreshToken()
	if err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("RefreshToken error = %v, want sign-in guidance", err)
	}
	if strings.Contains(err.Error(), "failed to parse token response") {
		t.Fatalf("nested OAuth error was misclassified: %v", err)
	}
	if p.refreshToken != "" {
		t.Fatalf("refresh token = %q, want retired", p.refreshToken)
	}
	if p.accessToken != "still-usable-access" {
		t.Fatalf("access token = %q, want preserved", p.accessToken)
	}
	if err := p.RefreshToken(); err == nil || !strings.Contains(err.Error(), "no refresh token") {
		t.Fatalf("second RefreshToken error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("token requests = %d, want 1", requests.Load())
	}

	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil {
		t.Fatalf("load persisted tokens: %v", err)
	}
	var persisted map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &persisted); err != nil {
		t.Fatalf("decode persisted tokens: %v", err)
	}
	if persisted["refresh_token"] != "" {
		t.Fatalf("persisted refresh token = %#v, want empty", persisted["refresh_token"])
	}
	if persisted["access_token"] != "still-usable-access" {
		t.Fatalf("persisted access token = %#v, want preserved", persisted["access_token"])
	}
}

func TestConcurrentRefreshTokenUsesOneRotatingExchange(t *testing.T) {
	saveTestProviderInstance(t, "openai-concurrent-refresh")
	p := NewProvider("openai-concurrent-refresh", "")
	p.refreshToken = "single-use-refresh"
	if err := p.SaveToDB(); err != nil {
		t.Fatalf("save initial tokens: %v", err)
	}

	oldClient := oauthHTTPClient
	t.Cleanup(func() { oauthHTTPClient = oldClient })
	var requests atomic.Int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	oauthHTTPClient = &http.Client{Transport: openAIRoundTripFunc(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			close(requestStarted)
		}
		<-releaseRequest
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh"}`)), Header: make(http.Header)}, nil
	})}

	const callers = 16
	errors := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-start
			errors <- p.RefreshToken()
		}()
	}
	ready.Wait()
	close(start)
	<-requestStarted
	close(releaseRequest)
	for range callers {
		if err := <-errors; err != nil {
			t.Fatalf("concurrent RefreshToken: %v", err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("token requests = %d, want 1", requests.Load())
	}
}

func TestSetupAuthRejectsAPIKey(t *testing.T) {
	// This provider is OAuth-only; an API key must be refused with guidance
	// rather than silently accepted.
	p := NewProvider("openai-1", "")
	err := p.SetupAuth(nil)
	if err == nil {
		t.Fatal("expected SetupAuth to reject direct configuration")
	}
	if !strings.Contains(err.Error(), "start-oauth") {
		t.Errorf("error %v should point the caller at the sign-in flow", err)
	}
}

func TestRemapModelPassesThroughSupported(t *testing.T) {
	for _, m := range Models {
		if got := RemapModel(m.ID); got != m.ID {
			t.Errorf("RemapModel(%q) = %q, want unchanged", m.ID, got)
		}
	}
}

func TestRemapModelRedirectsRejectedModels(t *testing.T) {
	// These are rejected upstream for ChatGPT accounts with
	// "not supported when using Codex with a ChatGPT account"; steering them
	// to a supported model beats surfacing a 400.
	for _, model := range []string{"gpt-5-codex", "gpt-5.1-codex", "gpt-5", "gpt-4o", "o3-mini"} {
		got := RemapModel(model)
		if got == model {
			t.Errorf("RemapModel(%q) returned it unchanged; expected a supported model", model)
		}
		var supported bool
		for _, m := range Models {
			if got == m.ID {
				supported = true
				break
			}
		}
		if !supported {
			t.Errorf("RemapModel(%q) = %q, which is not in the model catalog", model, got)
		}
	}
}

func TestGetModelsScopesProviderToInstance(t *testing.T) {
	resp := GetModels("openai-7")
	if len(resp.Data) != len(Models) {
		t.Fatalf("got %d models, want %d", len(resp.Data), len(Models))
	}
	for _, m := range resp.Data {
		if m.Provider != "openai-7" {
			t.Errorf("model %s has Provider %q, want openai-7", m.ID, m.Provider)
		}
	}
	// The package-level catalog must not be mutated by scoping.
	for _, m := range Models {
		if m.Provider != "openai" {
			t.Errorf("package catalog was mutated: %s has Provider %q", m.ID, m.Provider)
		}
	}
}

func TestModelCatalogExcludesCodexModels(t *testing.T) {
	// Guards against someone "helpfully" adding a *-codex model that the
	// ChatGPT-account backend rejects.
	for _, m := range Models {
		if strings.Contains(m.ID, "codex") {
			t.Errorf("model %q is a codex model, which the ChatGPT backend rejects", m.ID)
		}
	}
}

func saveTestProviderInstance(t *testing.T, instanceID string) {
	t.Helper()
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{
		InstanceID: instanceID,
		ProviderID: "openai",
		Name:       "OpenAI",
	}); err != nil {
		t.Fatalf("save provider instance: %v", err)
	}
	t.Cleanup(func() { _ = database.NewProviderInstanceStore().Delete(instanceID) })
}

func TestApplyTokensPopulatesNameFromEmail(t *testing.T) {
	saveTestProviderInstance(t, "openai-1")
	p := NewProvider("openai-1", "")
	idToken := makeJWT(t, map[string]interface{}{
		"email": "someone@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-1",
		},
	})
	if err := p.ApplyTokens(&TokenResponse{AccessToken: "a", RefreshToken: "r", IDToken: idToken}); err != nil {
		t.Fatalf("ApplyTokens: %v", err)
	}

	if p.accountID != "acct-1" {
		t.Errorf("accountID = %q, want acct-1", p.accountID)
	}
	if p.refreshToken != "r" {
		t.Errorf("refreshToken = %q, want r", p.refreshToken)
	}
	if !strings.Contains(p.GetName(), "someone@example.com") {
		t.Errorf("name %q should include the signed-in email", p.GetName())
	}
}

func TestApplyTokensKeepsExistingRefreshTokenWhenOmitted(t *testing.T) {
	// Refresh responses sometimes omit the refresh token; dropping it would
	// permanently break unattended re-auth.
	saveTestProviderInstance(t, "openai-preserve-refresh")
	p := NewProvider("openai-preserve-refresh", "")
	p.refreshToken = "original"
	if err := p.ApplyTokens(&TokenResponse{AccessToken: "new-access"}); err != nil {
		t.Fatalf("ApplyTokens: %v", err)
	}

	if p.refreshToken != "original" {
		t.Errorf("refreshToken = %q, want the original to be preserved", p.refreshToken)
	}
}

func TestApplyTokensRejectsNil(t *testing.T) {
	p := NewProvider("openai-1", "")
	if err := p.ApplyTokens(nil); err == nil {
		t.Error("expected an error for a nil token response")
	}
}

func TestSaveAndLoadFromDBRoundTrip(t *testing.T) {
	// This is the path exercised on server restart; if it drops the account ID
	// or refresh token the provider silently stops working.
	saveTestProviderInstance(t, "openai-roundtrip")
	orig := NewProvider("openai-roundtrip", "")
	orig.accessToken = "access-tok"
	orig.refreshToken = "refresh-tok"
	orig.accountID = "acct-round"
	orig.email = "round@example.com"
	orig.expiresAt = time.Now().Add(24 * time.Hour).Unix()
	orig.name = "OpenAI (round@example.com)"

	if err := orig.SaveToDB(); err != nil {
		t.Fatalf("SaveToDB: %v", err)
	}

	loaded := NewProvider("openai-roundtrip", "")
	if err := loaded.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	if loaded.accessToken != orig.accessToken {
		t.Errorf("accessToken = %q, want %q", loaded.accessToken, orig.accessToken)
	}
	if loaded.refreshToken != orig.refreshToken {
		t.Errorf("refreshToken = %q, want %q", loaded.refreshToken, orig.refreshToken)
	}
	if loaded.accountID != orig.accountID {
		t.Errorf("accountID = %q, want %q", loaded.accountID, orig.accountID)
	}
	if loaded.expiresAt != orig.expiresAt {
		t.Errorf("expiresAt = %d, want %d", loaded.expiresAt, orig.expiresAt)
	}
	if loaded.GetName() != orig.name {
		t.Errorf("name = %q, want %q", loaded.GetName(), orig.name)
	}
}

func TestLoadFromDBWithNoRecordIsNoError(t *testing.T) {
	p := NewProvider("openai-does-not-exist", "")
	if err := p.LoadFromDB(); err != nil {
		t.Errorf("LoadFromDB for an unknown instance should be a no-op, got %v", err)
	}
}

func TestProviderIdentity(t *testing.T) {
	p := NewProvider("openai-3", "")
	if got := p.GetID(); got != "openai" {
		t.Errorf("GetID() = %q, want openai", got)
	}
	if got := p.GetInstanceID(); got != "openai-3" {
		t.Errorf("GetInstanceID() = %q, want openai-3", got)
	}
	if got := p.GetBaseURL(); got != "https://chatgpt.com/backend-api/codex" {
		t.Errorf("GetBaseURL() = %q, want the Codex backend", got)
	}
}

// The Codex backend rejects the standard Responses "user" and
// "max_output_tokens" fields with {"detail":"Unsupported parameter: ..."}.
// UserID is populated from metadata.user_id on Anthropic-shape requests, which
// Claude Code always sends, so failing to strip it broke every such request.
func TestBuildCodexPayloadStripsUnsupportedParams(t *testing.T) {
	userID := "user-abc123"
	maxTokens := 256
	req := &cif.CanonicalRequest{
		Model:     "gpt-5.4-mini",
		UserID:    &userID,
		MaxTokens: &maxTokens,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "hi"},
			}},
		},
	}

	payload := buildCodexPayload("gpt-5.4-mini", req)

	if _, ok := payload["user"]; ok {
		t.Fatalf("expected 'user' to be stripped, got payload: %v", payload)
	}
	if _, ok := payload["max_output_tokens"]; ok {
		t.Fatalf("expected 'max_output_tokens' to be stripped, got payload: %v", payload)
	}
	if payload["stream"] != true {
		t.Errorf("expected stream=true, got %v", payload["stream"])
	}
	if payload["store"] != false {
		t.Errorf("expected store=false, got %v", payload["store"])
	}
	if payload["model"] != "gpt-5.4-mini" {
		t.Errorf("expected model to be preserved, got %v", payload["model"])
	}
}
