package antigravity

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func withOAuthHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	old := oauthHTTPClient
	oauthHTTPClient = &http.Client{Transport: fn}
	t.Cleanup(func() { oauthHTTPClient = old })
}

func TestBuildAuthURLPinsCompleteQuery(t *testing.T) {
	got := BuildAuthURL("client id", "http://gateway.test/callback?a=1", "state value")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	endpoint := parsed.Scheme + "://" + parsed.Host + parsed.Path
	if endpoint != googleAuthURL {
		t.Fatalf("endpoint = %q, want %q", endpoint, googleAuthURL)
	}
	want := url.Values{
		"access_type":   {"offline"},
		"client_id":     {"client id"},
		"prompt":        {"consent"},
		"redirect_uri":  {"http://gateway.test/callback?a=1"},
		"response_type": {"code"},
		"scope":         {OAuthScopes},
		"state":         {"state value"},
	}
	if parsed.Query().Encode() != want.Encode() {
		t.Fatalf("query = %q, want %q", parsed.Query().Encode(), want.Encode())
	}
}

func TestExchangeCodeUsesFormAndCallerRedirect(t *testing.T) {
	withOAuthHTTPClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(request.Body)
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse request: %v", err)
		}
		want := url.Values{"client_id": {"client"}, "client_secret": {"secret"}, "code": {"code"}, "grant_type": {"authorization_code"}, "redirect_uri": {"http://exact.test/callback"}}
		if values.Encode() != want.Encode() {
			t.Fatalf("form = %q, want %q", values.Encode(), want.Encode())
		}
		return jsonResponse(`{"access_token":"access","refresh_token":"refresh"}`), nil
	})
	response, err := ExchangeCode("client", "secret", "code", "http://exact.test/callback")
	if err != nil || response.RefreshToken != "refresh" {
		t.Fatalf("ExchangeCode = %#v, %v", response, err)
	}
}

func TestRefreshAccessTokenUsesFormAndAllowsOmittedRefreshToken(t *testing.T) {
	withOAuthHTTPClient(t, func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		values, _ := url.ParseQuery(string(body))
		want := url.Values{"client_id": {"client"}, "client_secret": {"secret"}, "grant_type": {"refresh_token"}, "refresh_token": {"old"}}
		if values.Encode() != want.Encode() {
			t.Fatalf("form = %q, want %q", values.Encode(), want.Encode())
		}
		return jsonResponse(`{"access_token":"new"}`), nil
	})
	response, err := RefreshAccessToken("client", "secret", "old")
	if err != nil || response.AccessToken != "new" || response.RefreshToken != "" {
		t.Fatalf("RefreshAccessToken = %#v, %v", response, err)
	}
}

func TestTokenRequestsSurfaceOAuthErrors(t *testing.T) {
	withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"error":"invalid_grant","error_description":"expired"}`), nil
	})
	if _, err := ExchangeCode("client", "secret", "code", "redirect"); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("ExchangeCode error = %v", err)
	}
	if _, err := RefreshAccessToken("client", "secret", "refresh"); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("RefreshAccessToken error = %v", err)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
