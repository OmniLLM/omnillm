// Package generic provides a generic provider implementation for alibaba, antigravity, azure-openai, and google.
// It acts as a facade that delegates to provider-specific sub-packages while maintaining
// backward compatibility for callers that use *GenericProvider type assertions.
package generic

import (
	"fmt"

	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"
	"omnillm/internal/providers/types"
)

// ─── Constructor ──────────────────────────────────────────────────────────────

// providerDisplayNames maps provider type IDs to human-friendly display names.
var providerDisplayNames = map[string]string{
	"azure-openai": "Azure OpenAI",
	"antigravity":  "Antigravity",
	"alibaba":      "Alibaba",
	"google":       "Google",
	"kimi":         "Kimi",
}

// NewGenericProvider creates a new GenericProvider for the given provider type.
func NewGenericProvider(providerType, instanceID, name string) *GenericProvider {
	baseURL := providerBaseURLs[providerType]
	displayName := name
	if displayName == "" {
		if friendly, ok := providerDisplayNames[providerType]; ok {
			displayName = friendly
		} else {
			displayName = instanceID
		}
	}
	return &GenericProvider{
		id:         providerType,
		instanceID: instanceID,
		name:       displayName,
		baseURL:    baseURL,
	}
}

// ─── Identity ─────────────────────────────────────────────────────────────────

func (p *GenericProvider) GetID() string         { return p.id }
func (p *GenericProvider) GetInstanceID() string { return p.instanceID }
func (p *GenericProvider) GetName() string       { return p.name }
func (p *GenericProvider) SetName(name string)   { p.name = name }

// SetInstanceID updates the provider's in-memory instance ID after a registry rename.
func (p *GenericProvider) SetInstanceID(newID string) { p.instanceID = newID }

// ─── Headers ──────────────────────────────────────────────────────────────────

func (p *GenericProvider) GetHeaders(forVision bool) map[string]string {
	p.loadConfigFromDB()

	switch p.id {
	case "alibaba":
		return alibabapkg.Headers(p.ensureFreshAlibabaToken(), false, p.config)
	case "azure-openai":
		return azurepkg.Headers(p.token)
	case "google":
		return googlepkg.Headers(p.token)
	case "kimi":
		return kimipkg.Headers(p.token, false, p.config)
	}

	return map[string]string{
		"Authorization": "Bearer " + p.token,
		"Content-Type":  "application/json",
	}
}

// ensureFreshAlibabaToken returns the current token without refresh since we only support API keys now.
func (p *GenericProvider) ensureFreshAlibabaToken() string {
	return p.token
}

// ─── Models ───────────────────────────────────────────────────────────────────

func (p *GenericProvider) GetModels() (*types.ModelsResponse, error) {
	p.loadConfigFromDB()

	switch p.id {
	case "azure-openai":
		return azurepkg.GetModels(p.instanceID, p.config), nil
	case "alibaba":
		return alibabapkg.GetModels(p.instanceID, p.token, p.baseURL, p.config)
	case "google":
		return googlepkg.FetchModels(p.instanceID, p.token, p.baseURL)
	case "kimi":
		return kimipkg.GetModels(p.instanceID, p.token, p.baseURL, p.config)
	}

	models := providerModels[p.id]
	if models == nil {
		models = []types.Model{}
	}
	result := make([]types.Model, len(models))
	for i, m := range models {
		result[i] = m
		result[i].Provider = p.instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}, nil
}

// ─── Legacy methods ───────────────────────────────────────────────────────────

func (p *GenericProvider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: use the adapter for chat completions", p.id)
}

func (p *GenericProvider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: embeddings not yet implemented in Go backend", p.id)
}

func (p *GenericProvider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *GenericProvider) GetAdapter() types.ProviderAdapter {
	return &GenericAdapter{provider: p}
}
