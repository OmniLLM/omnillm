// Package oauthcode provides provider-neutral OAuth authorization-code primitives.
package oauthcode

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

// StateEncoding constrains the supported OAuth state encodings.
type StateEncoding uint8

const (
	StateEncodingBase64URL StateEncoding = iota + 1
	StateEncodingHex
)

// TokenResponse contains the common fields returned by OAuth token endpoints.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// DecodeTokenResponse decodes a JSON OAuth token response.
func DecodeTokenResponse(data []byte) (*TokenResponse, error) {
	var response TokenResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode OAuth token response: %w", err)
	}
	return &response, nil
}

// GenerateState returns byteLength bytes of secure randomness using encoding.
func GenerateState(byteLength int, encoding StateEncoding) (string, error) {
	return generateState(rand.Reader, byteLength, encoding)
}

func generateState(reader io.Reader, byteLength int, encoding StateEncoding) (string, error) {
	if byteLength <= 0 {
		return "", fmt.Errorf("OAuth state byte length must be positive")
	}
	var encode func([]byte) string
	switch encoding {
	case StateEncodingBase64URL:
		encode = base64.RawURLEncoding.EncodeToString
	case StateEncodingHex:
		encode = hex.EncodeToString
	default:
		return "", fmt.Errorf("unsupported OAuth state encoding %d", encoding)
	}
	bytes := make([]byte, byteLength)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	return encode(bytes), nil
}

// PKCE contains an S256 code verifier/challenge pair.
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE generates a 32-byte, unpadded-base64url S256 PKCE pair.
func GeneratePKCE() (*PKCE, error) {
	return generatePKCE(rand.Reader)
}

func generatePKCE(reader io.Reader) (*PKCE, error) {
	verifier, err := generateState(reader, 32, StateEncodingBase64URL)
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	digest := sha256.Sum256([]byte(verifier))
	return &PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(digest[:]),
	}, nil
}

// BuildAuthorizationURL adds an encoded query to a caller-owned endpoint.
func BuildAuthorizationURL(endpoint string, values url.Values) string {
	if len(values) == 0 {
		return endpoint
	}
	separator := "?"
	if parsed, err := url.Parse(endpoint); err == nil && parsed.RawQuery != "" {
		separator = "&"
	}
	return endpoint + separator + values.Encode()
}
