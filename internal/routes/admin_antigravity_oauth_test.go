package routes

import (
	"bytes"
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
	antigravitypkg "omnillm/internal/providers/antigravity"
)

func resetAntigravityOAuthTestState(t *testing.T) {
	t.Helper()
	oldGenerate := generateAntigravityOAuthState
	oldExchange := exchangeAntigravityOAuthCode
	agOAuthMu.Lock()
	agOAuthStates = map[string]*antigravityOAuthState{}
	agOAuthMu.Unlock()
	t.Cleanup(func() {
		generateAntigravityOAuthState = oldGenerate
		exchangeAntigravityOAuthCode = oldExchange
		agOAuthMu.Lock()
		agOAuthStates = map[string]*antigravityOAuthState{}
		agOAuthMu.Unlock()
	})
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
