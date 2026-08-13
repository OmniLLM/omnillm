// Package copilot provides GitHub Copilot provider implementation
package copilot

import (
	"strings"

	ghservice "omnillm/internal/services/github"
)

func NewGitHubCopilotProvider(instanceID, name string) *GitHubCopilotProvider {
	if name == "" {
		name = "GitHub Copilot"
	}
	return &GitHubCopilotProvider{
		id:           "github-copilot",
		instanceID:   instanceID,
		name:         name,
		baseURL:      "https://api.githubcopilot.com",
		tokenFetcher: ghservice.GetCopilotToken,
	}
}

// Provider interface implementation
func (p *GitHubCopilotProvider) GetID() string         { return p.id }
func (p *GitHubCopilotProvider) GetInstanceID() string { return p.instanceID }
func (p *GitHubCopilotProvider) GetName() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}
func (p *GitHubCopilotProvider) SetName(name string) {
	p.mu.Lock()
	p.name = name
	p.mu.Unlock()
}

// SetInstanceID updates the provider's in-memory instance ID.
// Used by auth-and-create flow to assign the canonical ID after successful auth.
func (p *GitHubCopilotProvider) SetInstanceID(newID string) { p.instanceID = newID }

// GetBaseURL returns the configured upstream base URL for Copilot API calls.
func (p *GitHubCopilotProvider) GetBaseURL() string { return p.baseURL }

// IsResponsesOnlyModel reports whether the given Copilot model is known to
// require the /responses endpoint and cannot be served via /chat/completions.
//
// Returns true only when the shape cache (populated by GetModels) explicitly
// marks the model as responses-only. Returns false on cache miss, nil cache,
// or empty model name — callers should fall back to their existing heuristics
// (e.g. shared.IsGPT5Family) in that case.
//
// Provider-dispatch code uses this via an interface assertion to avoid the
// import cycle that would arise from depending on the copilot package directly.
func (p *GitHubCopilotProvider) publishShapeCache(cache modelShapeCache) {
	p.shapeMu.Lock()
	p.shapeCache = cache
	p.shapeMu.Unlock()
}

func (p *GitHubCopilotProvider) lookupShape(model string) (copilotAPIShape, bool) {
	if p == nil {
		return "", false
	}
	p.shapeMu.RLock()
	shape, ok := p.shapeCache[model]
	p.shapeMu.RUnlock()
	return shape, ok
}

func (p *GitHubCopilotProvider) IsResponsesOnlyModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	shape, ok := p.lookupShape(normalized)
	return ok && shape == shapeResponses
}
