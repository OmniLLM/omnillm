// Package providermetadata defines the allowlisted provider inspection fields.
package providermetadata

import (
	"net/url"
	"regexp"
	"strings"

	antigravity "omnillm/internal/providers/antigravity"
	openai "omnillm/internal/providers/openai"
	github "omnillm/internal/services/github"
)

// Metadata contains only non-secret fields intended for provider inspection.
type Metadata struct {
	ID                    string `json:"id"`
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string `json:"token_endpoint,omitempty"`
	OAuthClientID         string `json:"oauth_client_id,omitempty"`
	APIEndpointOrigin     string `json:"api_endpoint_origin,omitempty"`
	Region                string `json:"region,omitempty"`
	Plan                  string `json:"plan,omitempty"`
	APIFormat             string `json:"api_format,omitempty"`
	APIVersion            string `json:"api_version,omitempty"`
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)

// OAuthCapabilities returns provider-owned endpoints and public fixed client IDs.
// These fields describe available OAuth capabilities, not the instance's auth method.
func OAuthCapabilities(providerType string) Metadata {
	switch providerType {
	case "antigravity":
		return Metadata{AuthorizationEndpoint: antigravity.AuthorizationEndpoint, TokenEndpoint: antigravity.TokenEndpoint}
	case "openai":
		return Metadata{AuthorizationEndpoint: openai.AuthorizationEndpoint, TokenEndpoint: openai.TokenEndpoint, OAuthClientID: openai.OAuthClientID}
	case "github-copilot":
		return Metadata{AuthorizationEndpoint: github.DeviceAuthorizationEndpoint, TokenEndpoint: github.OAuthTokenEndpoint, OAuthClientID: github.ClientID}
	default:
		return Metadata{}
	}
}

// Origin strips all credential-bearing URL components, including the path.
func Origin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}

// Sanitize enforces the same allowlist at the server and CLI trust boundaries.
func Sanitize(providerType string, input Metadata) Metadata {
	output := Metadata{ID: input.ID, APIEndpointOrigin: Origin(input.APIEndpointOrigin)}
	known := OAuthCapabilities(providerType)
	if known.AuthorizationEndpoint != "" && input.AuthorizationEndpoint == known.AuthorizationEndpoint {
		output.AuthorizationEndpoint = input.AuthorizationEndpoint
	}
	if known.TokenEndpoint != "" && input.TokenEndpoint == known.TokenEndpoint {
		output.TokenEndpoint = input.TokenEndpoint
	}
	if providerType == "antigravity" && safeIdentifier.MatchString(input.OAuthClientID) {
		output.OAuthClientID = input.OAuthClientID
	} else if known.OAuthClientID != "" && input.OAuthClientID == known.OAuthClientID {
		output.OAuthClientID = input.OAuthClientID
	}
	if providerType == "alibaba" || providerType == "kimi" {
		switch input.Region {
		case "global", "china":
			output.Region = input.Region
		}
	}
	if providerType == "alibaba" {
		switch input.Plan {
		case "standard", "coding-plan":
			output.Plan = input.Plan
		}
	}
	if providerType == "alibaba" || providerType == "openai-compatible" {
		switch input.APIFormat {
		case "chat.completions", "responses":
			output.APIFormat = input.APIFormat
		}
	}
	if providerType == "azure-openai" && safeIdentifier.MatchString(input.APIVersion) {
		output.APIVersion = input.APIVersion
	}
	return output
}
