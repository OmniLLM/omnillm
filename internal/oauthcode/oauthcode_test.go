package oauthcode

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func withRandomReader(t *testing.T, reader io.Reader) {
	t.Helper()
	old := randomReader
	randomReader = reader
	t.Cleanup(func() { randomReader = old })
}

func TestGenerateStateEncodingsAndLengths(t *testing.T) {
	withRandomReader(t, strings.NewReader(strings.Repeat("\xab", 32)))

	base64State, err := GenerateState(16, StateEncodingBase64URL)
	if err != nil {
		t.Fatalf("GenerateState base64url: %v", err)
	}
	if len(base64State) != 22 || strings.ContainsAny(base64State, "+/=") {
		t.Fatalf("base64url state = %q, want 22 unpadded URL-safe characters", base64State)
	}

	hexState, err := GenerateState(16, StateEncodingHex)
	if err != nil {
		t.Fatalf("GenerateState hex: %v", err)
	}
	if len(hexState) != 32 || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(hexState) {
		t.Fatalf("hex state = %q, want 32 lowercase hexadecimal characters", hexState)
	}
}

func TestGenerateStateRejectsUnsupportedInputs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		length   int
		encoding StateEncoding
	}{
		{"zero length", 0, StateEncodingBase64URL},
		{"negative length", -1, StateEncodingHex},
		{"unsupported encoding", 16, StateEncoding(255)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateState(tc.length, tc.encoding); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestGenerateStatePropagatesRandomFailure(t *testing.T) {
	withRandomReader(t, errorReader{})
	if _, err := GenerateState(16, StateEncodingHex); !errors.Is(err, errRandom) {
		t.Fatalf("GenerateState error = %v, want wrapped random error", err)
	}
}

func TestGeneratePKCEUsesS256(t *testing.T) {
	withRandomReader(t, strings.NewReader(strings.Repeat("x", 32)))
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	if len(pkce.Verifier) != 43 || strings.ContainsAny(pkce.Verifier, "+/=") {
		t.Fatalf("verifier = %q, want 43 unpadded base64url characters", pkce.Verifier)
	}
	digest := sha256.Sum256([]byte(pkce.Verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if pkce.Challenge != wantChallenge {
		t.Fatalf("challenge = %q, want %q", pkce.Challenge, wantChallenge)
	}
}

func TestGeneratePKCEPropagatesRandomFailure(t *testing.T) {
	withRandomReader(t, errorReader{})
	if _, err := GeneratePKCE(); !errors.Is(err, errRandom) {
		t.Fatalf("GeneratePKCE error = %v, want wrapped random error", err)
	}
}

func TestBuildAuthorizationURLEncodesValues(t *testing.T) {
	got, err := BuildAuthorizationURL("https://example.test/oauth?existing=kept", url.Values{
		"client_id":    {"client id/+"},
		"redirect_uri": {"http://localhost/callback?a=1&b=two"},
		"scope":        {"openid email"},
	})
	if err != nil {
		t.Fatalf("BuildAuthorizationURL: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if parsed.Query().Get("existing") != "kept" || parsed.Query().Get("client_id") != "client id/+" || parsed.Query().Get("redirect_uri") != "http://localhost/callback?a=1&b=two" {
		t.Fatalf("unexpected query: %v", parsed.Query())
	}
	if !strings.Contains(got, "client_id=client+id%2F%2B") {
		t.Fatalf("URL is not deterministically encoded: %s", got)
	}
}

func TestBuildAuthorizationURLRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"relative/path", "://bad"} {
		if _, err := BuildAuthorizationURL(endpoint, nil); err == nil {
			t.Fatalf("expected error for %q", endpoint)
		}
	}
}

func TestDecodeTokenResponse(t *testing.T) {
	got, err := DecodeTokenResponse([]byte(`{"access_token":"a","refresh_token":"r","id_token":"i","token_type":"Bearer","expires_in":3600,"error":"invalid","error_description":"bad"}`))
	if err != nil {
		t.Fatalf("DecodeTokenResponse: %v", err)
	}
	if got.AccessToken != "a" || got.RefreshToken != "r" || got.IDToken != "i" || got.TokenType != "Bearer" || got.ExpiresIn != 3600 || got.Error != "invalid" || got.ErrorDesc != "bad" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestDecodeTokenResponseRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeTokenResponse([]byte(`{"access_token":`)); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}

var errRandom = errors.New("random failed")

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errRandom }
