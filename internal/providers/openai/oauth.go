// Package openai implements the ChatGPT (OAuth) provider.
//
// This provider authenticates with a ChatGPT account via the OAuth 2.0
// authorization-code flow with PKCE against auth.openai.com, and routes
// requests to the Codex backend at chatgpt.com/backend-api/codex.
//
// It is distinct from the "codex" provider, which authenticates with an
// OpenAI platform API key and talks to api.openai.com/v1.
package openai

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"omnillm/internal/providers/shared"
)

const (
	// oauthAuthorizeURL is the ChatGPT authorization endpoint.
	oauthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	// oauthTokenURL exchanges/refreshes authorization codes and tokens.
	oauthTokenURL = "https://auth.openai.com/oauth/token"

	// OAuthClientID is the public client ID used by the Codex CLI. The ChatGPT
	// OAuth service only accepts this client for the Codex backend.
	OAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// OAuthScopes are the scopes required for Codex backend access.
	// offline_access is required to receive a refresh token.
	OAuthScopes = "openid profile email offline_access"

	// CallbackPort is fixed: the OAuth client whitelists exactly this redirect
	// URI, so the callback listener must bind this port. It cannot be changed
	// to the OmniLLM server port.
	CallbackPort = 1455
	// CallbackPath is the whitelisted redirect path.
	CallbackPath = "/auth/callback"
)

// RedirectURI returns the one redirect URI accepted by the OAuth client.
func RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s", CallbackPort, CallbackPath)
}

var oauthHTTPClient = shared.DefaultHTTPClient(30 * time.Second)

// PKCE holds a generated code verifier/challenge pair.
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a cryptographically random S256 PKCE pair.
func GeneratePKCE() (*PKCE, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("openai: failed to generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return &PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

// RandomState returns a random state nonce for CSRF protection.
func RandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("openai: failed to generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BuildAuthURL returns the URL the user must visit to grant access.
func BuildAuthURL(challenge, state string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", OAuthClientID)
	v.Set("redirect_uri", RedirectURI())
	v.Set("scope", OAuthScopes)
	v.Set("code_challenge", challenge)
	v.Set("code_challenge_method", "S256")
	v.Set("state", state)
	v.Set("prompt", "login")
	v.Set("id_token_add_organizations", "true")
	return oauthAuthorizeURL + "?" + v.Encode()
}

// TokenResponse is the payload returned by the ChatGPT token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// postToken sends a JSON body to the token endpoint and decodes the result.
func postToken(payload map[string]string) (*TokenResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to encode token request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, oauthTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var t TokenResponse
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("openai: failed to parse token response (status %d): %s",
			resp.StatusCode, truncate(string(raw), 200))
	}
	if t.Error != "" {
		return nil, fmt.Errorf("openai: token request failed: %s — %s", t.Error, t.ErrorDesc)
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("openai: no access_token in response (status %d): %s",
			resp.StatusCode, truncate(string(raw), 200))
	}
	return &t, nil
}

// ExchangeCode swaps an authorization code for access + refresh tokens.
func ExchangeCode(code, verifier string) (*TokenResponse, error) {
	return postToken(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     OAuthClientID,
		"code":          code,
		"redirect_uri":  RedirectURI(),
		"code_verifier": verifier,
	})
}

// RefreshAccessToken exchanges a refresh token for a fresh token pair.
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	return postToken(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     OAuthClientID,
		"refresh_token": refreshToken,
		"scope":         OAuthScopes,
	})
}

// Claims holds the ChatGPT-specific fields extracted from the ID token.
type Claims struct {
	Email     string
	AccountID string
	PlanType  string
	ExpiresAt int64
}

// ParseTokenClaims decodes the (unverified) payload of a JWT issued by
// auth.openai.com and extracts the fields needed to call the Codex backend.
//
// The signature is deliberately not verified: the token is used solely as an
// opaque bearer credential that the upstream API validates. We only read it to
// discover the account ID and a human-friendly label.
func ParseTokenClaims(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("openai: malformed JWT")
	}
	segment := parts[1]
	if pad := len(segment) % 4; pad != 0 {
		segment += strings.Repeat("=", 4-pad)
	}
	raw, err := base64.URLEncoding.DecodeString(segment)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to decode JWT payload: %w", err)
	}

	var payload struct {
		Email string `json:"email"`
		Exp   int64  `json:"exp"`
		Auth  struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			ChatGPTPlanType  string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("openai: failed to parse JWT claims: %w", err)
	}

	return &Claims{
		Email:     payload.Email,
		AccountID: payload.Auth.ChatGPTAccountID,
		PlanType:  payload.Auth.ChatGPTPlanType,
		ExpiresAt: payload.Exp,
	}, nil
}

// ClaimsFromTokenResponse extracts claims from the ID token, falling back to
// the access token when no ID token was returned (e.g. on refresh).
func ClaimsFromTokenResponse(t *TokenResponse) *Claims {
	if t == nil {
		return nil
	}
	for _, candidate := range []string{t.IDToken, t.AccessToken} {
		if candidate == "" {
			continue
		}
		if claims, err := ParseTokenClaims(candidate); err == nil && claims != nil {
			return claims
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
