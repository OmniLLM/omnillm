// Package copilot provides GitHub Copilot provider implementation
package copilot

import (
	ghservice "omnillm/internal/services/github"
)

func NewGitHubCopilotProvider(instanceID string) *GitHubCopilotProvider {
	return &GitHubCopilotProvider{
		id:           "github-copilot",
		instanceID:   instanceID,
		name:         "GitHub Copilot",
		baseURL:      "https://api.githubcopilot.com",
		tokenFetcher: ghservice.GetCopilotToken,
	}
}

// Provider interface implementation
func (p *GitHubCopilotProvider) GetID() string         { return p.id }
func (p *GitHubCopilotProvider) GetInstanceID() string { return p.instanceID }
func (p *GitHubCopilotProvider) GetName() string       { return p.name }
func (p *GitHubCopilotProvider) SetName(name string)   { p.name = name }

// SetInstanceID updates the provider's in-memory instance ID.
// Used by auth-and-create flow to assign the canonical ID after successful auth.
func (p *GitHubCopilotProvider) SetInstanceID(newID string) { p.instanceID = newID }

// GetBaseURL returns the configured upstream base URL for Copilot API calls.
func (p *GitHubCopilotProvider) GetBaseURL() string { return p.baseURL }
