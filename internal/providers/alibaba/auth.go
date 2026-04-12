// Package alibaba implements the Qwen / DashScope OAuth 2.0 device-code flow.
package alibaba

import (
	"context"
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
)

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	OAuthDeviceCodeEndpoint = "https://chat.qwen.ai/api/v1/oauth2/device/code"
	OAuthTokenEndpoint      = "https://chat.qwen.ai/api/v1/oauth2/token"
	OAuthClientID           = "f0304373b74a44d2b584a3fb70ca9e56"
	OAuthScope              = "openid profile email model.completion"
	OAuthGrantType          = "urn:ietf:params:oauth:grant-type:device_code"

	// OAuthBaseURL is the default API base for OAuth-authenticated requests.
	// This is the portal URL used by Qwen Code for OAuth, not the DashScope API.
	OAuthBaseURL = "https://portal.qwen.ai/v1"
	// ModelsBaseURL is the DashScope API for model listing (OAuth tokens work here too).
	ModelsBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

	// BaseURLChina is the DashScope endpoint for mainland China.
	BaseURLChina = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	// BaseURLGlobal is the DashScope endpoint for the international (non-China) region.
	BaseURLGlobal = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"

	// RefreshSkew is how early we proactively refresh a token before it expires.
	RefreshSkew = 5 * time.Minute
)

// ─── Types ────────────────────────────────────────────────────────────────────

// TokenData is the persisted credential record for an Alibaba instance.
// It is stored as JSON in the token database and must stay compatible with the
// TypeScript AlibabaTokenData shape.
type TokenData struct {
	AuthType     string `json:"auth_type"`    // "oauth" | "api-key"
	AccessToken  string `json:"access_token"` // OAuth access token or API key
	RefreshToken string `json:"refresh_token"`
	ResourceURL  string `json:"resource_url"` // base URL from OAuth token response
	ExpiresAt    int64  `json:"expires_at"`   // Unix milliseconds; 0 means never (api-key)
	BaseURL      string `json:"base_url"`     // explicit base URL for api-key auth
}

// DeviceFlowResponse contains the server's reply to the device-authorisation request.
// CodeVerifier holds the PKCE code verifier generated locally; it is not part of
// the server response but is stored here so callers can pass it to PollForToken.
type DeviceFlowResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"` // seconds
	Interval                int    `json:"interval"`   // minimum seconds between polls
	CodeVerifier            string `json:"-"`          // PKCE verifier; not from server
}

// tokenResponse is the JSON shape returned by the token endpoint on success.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ResourceURL  string `json:"resource_url"`
	ExpiresIn    int    `json:"expires_in"` // seconds
}

// ─── PKCE helpers ─────────────────────────────────────────────────────────────

// generateCodeVerifier creates a cryptographically random PKCE code verifier
// (RFC 7636 §4.1): 32 random bytes encoded as base64url without padding.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alibaba: failed to generate code verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge derives the S256 PKCE code challenge from a verifier:
// BASE64URL(SHA-256(ASCII(verifier))) with no padding.
func generateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ─── Device flow initiation ──────────────────────────────────────────────────

// InitiateDeviceFlow starts the OAuth 2.0 device-authorisation flow with PKCE
// (S256) and returns the device-code details the user needs to authorise in a
// browser.  The generated code verifier is stored in DeviceFlowResponse.CodeVerifier
// so callers can pass it to PollForToken.
func InitiateDeviceFlow(ctx context.Context) (*DeviceFlowResponse, error) {
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	codeChallenge := generateCodeChallenge(codeVerifier)

	params := url.Values{}
	params.Set("client_id", OAuthClientID)
	params.Set("scope", OAuthScope)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthDeviceCodeEndpoint,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba: device authorisation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alibaba: device authorisation failed (%d): %s", resp.StatusCode, string(body))
	}

	var flow DeviceFlowResponse
	if err := json.Unmarshal(body, &flow); err != nil {
		return nil, fmt.Errorf("alibaba: failed to parse device code response: %w", err)
	}
	if flow.DeviceCode == "" {
		return nil, fmt.Errorf("alibaba: device_code missing in response")
	}

	flow.CodeVerifier = codeVerifier
	return &flow, nil
}

// ─── Token polling ────────────────────────────────────────────────────────────

// PollForToken polls the token endpoint until the user has authorised the
// device or the device code expires.  intervalSec and expiresIn come from the
// device-flow response; pass 0 to use safe defaults.  codeVerifier is the
// PKCE verifier generated during InitiateDeviceFlow and must be included in
// every poll request per the Alibaba OAuth server requirements.
func PollForToken(ctx context.Context, deviceCode, codeVerifier string, intervalSec, expiresIn int) (*TokenData, error) {
	if intervalSec <= 0 {
		intervalSec = 5
	}
	if expiresIn <= 0 {
		expiresIn = 300
	}

	pollInterval := time.Duration(intervalSec) * time.Second
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		params := url.Values{}
		params.Set("grant_type", OAuthGrantType)
		params.Set("client_id", OAuthClientID)
		params.Set("device_code", deviceCode)
		if codeVerifier != "" {
			params.Set("code_verifier", codeVerifier)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenEndpoint,
			strings.NewReader(params.Encode()))
		if err != nil {
			return nil, fmt.Errorf("alibaba: failed to build token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// Transient network error – keep polling
			continue
		}

		rawBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var tr tokenResponse
			if err := json.Unmarshal(rawBody, &tr); err != nil {
				return nil, fmt.Errorf("alibaba: failed to parse token response: %w", err)
			}
			expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - RefreshSkew).UnixMilli()
			return &TokenData{
				AuthType:     "oauth",
				AccessToken:  tr.AccessToken,
				RefreshToken: tr.RefreshToken,
				ResourceURL:  tr.ResourceURL,
				ExpiresAt:    expiresAt,
				BaseURL:      "",
			}, nil
		}

		// RFC 8628 error handling
		if resp.StatusCode == http.StatusBadRequest {
			var errBody struct {
				Error            string `json:"error"`
				ErrorDescription string `json:"error_description"`
			}
			if jsonErr := json.Unmarshal(rawBody, &errBody); jsonErr == nil {
				switch errBody.Error {
				case "authorization_pending":
					continue
				case "slow_down":
					pollInterval = min(pollInterval+5*time.Second, 10*time.Second)
					continue
				case "expired_token":
					return nil, fmt.Errorf("alibaba: device code expired – please restart authentication")
				case "access_denied":
					return nil, fmt.Errorf("alibaba: authorisation denied – please restart authentication")
				default:
					return nil, fmt.Errorf("alibaba: token poll error: %s – %s", errBody.Error, errBody.ErrorDescription)
				}
			}
		}

		return nil, fmt.Errorf("alibaba: token poll failed (%d): %s", resp.StatusCode, string(rawBody))
	}

	return nil, fmt.Errorf("alibaba: authentication timed out – please restart authentication")
}

// ─── Token refresh ────────────────────────────────────────────────────────────

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, refreshToken string) (*TokenData, error) {
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", OAuthClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenEndpoint,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba: token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if jsonErr := json.Unmarshal(body, &errBody); jsonErr == nil && errBody.Error != "" {
			return nil, fmt.Errorf("alibaba: token refresh failed: %s – %s", errBody.Error, errBody.ErrorDescription)
		}
		return nil, fmt.Errorf("alibaba: token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("alibaba: failed to parse refresh response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - RefreshSkew).UnixMilli()
	return &TokenData{
		AuthType:     "oauth",
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ResourceURL:  tr.ResourceURL,
		ExpiresAt:    expiresAt,
		BaseURL:      "",
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// IsExpiringSoon reports whether the token will expire within RefreshSkew.
// Always returns false for api-key tokens (ExpiresAt == 0).
func IsExpiringSoon(data *TokenData) bool {
	if data == nil || data.AuthType == "api-key" || data.ExpiresAt == 0 {
		return false
	}
	return time.Now().After(time.UnixMilli(data.ExpiresAt))
}

// NormaliseBaseURL ensures a resource URL ends in "/v1" with an https scheme.
func NormaliseBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		s = "https://" + s
	}
	s = strings.TrimRight(s, "/")
	if !strings.HasSuffix(strings.ToLower(s), "/v1") {
		s += "/v1"
	}
	return s
}

// min is a helper for Go versions older than 1.21.
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
