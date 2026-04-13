// Package types defines interfaces and types for providers
package types

import (
	"omnimodel/internal/cif"
)

type ProviderID string

const (
	ProviderGitHubCopilot ProviderID = "github-copilot"
	ProviderAntigravity   ProviderID = "antigravity"
	ProviderAlibaba       ProviderID = "alibaba"
	ProviderAzureOpenAI   ProviderID = "azure-openai"
	ProviderGoogle        ProviderID = "google"
	ProviderKimi          ProviderID = "kimi"
)

type AuthOptions struct {
	Method       string `json:"method,omitempty"`
	Force        bool   `json:"force,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	GithubToken  string `json:"github_token,omitempty"`
	Token        string `json:"token,omitempty"` // alias for GithubToken from frontend
	APIKey       string `json:"apiKey,omitempty"`
	Region       string `json:"region,omitempty"`
	Plan         string `json:"plan,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
}

type Model struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
	MaxTokens    int                    `json:"max_tokens,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
}

type ModelsResponse struct {
	Data   []Model `json:"data"`
	Object string  `json:"object"`
}

type ProviderAdapter interface {
	GetProvider() Provider
	Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error)
	ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error)
	RemapModel(canonicalModel string) string
}

type Provider interface {
	// Identity
	GetID() string         // Provider type (e.g. "antigravity")
	GetInstanceID() string // Unique instance identifier (e.g. "antigravity-1")
	GetName() string

	// Authentication
	SetupAuth(options *AuthOptions) error
	GetToken() string
	RefreshToken() error

	// API Configuration
	GetBaseURL() string
	GetHeaders(forVision bool) map[string]string

	// Models
	GetModels() (*ModelsResponse, error)

	// Legacy Request Methods (to be deprecated)
	CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error)
	CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error)
	GetUsage() (map[string]interface{}, error)

	// CIF Adapter (optional during migration)
	GetAdapter() ProviderAdapter
}

type ProviderConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config,omitempty"`
}
