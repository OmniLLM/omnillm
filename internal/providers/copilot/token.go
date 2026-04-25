package copilot

import (
	"encoding/json"
	"fmt"
	"time"

	"omnillm/internal/database"
	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

// SetGitHubToken sets the long-lived GitHub OAuth token (used for Copilot token refresh)
func (p *GitHubCopilotProvider) SetGitHubToken(token string) {
	p.githubToken = token
}

func (p *GitHubCopilotProvider) GetToken() string {
	if p.githubToken != "" {
		needsRefresh := p.token == "" || p.expiresAt == 0 || time.Now().Unix() > p.expiresAt-300
		if needsRefresh {
			if err := p.RefreshToken(); err != nil {
				log.Warn().Err(err).Msg("Failed to auto-refresh Copilot token")
			}
		}
	}
	return p.token
}

func (p *GitHubCopilotProvider) RefreshToken() error {
	if p.githubToken == "" {
		log.Debug().Str("provider", p.instanceID).Msg("No GitHub token available for refresh")
		return nil
	}

	fetcher := p.tokenFetcher
	if fetcher == nil {
		fetcher = ghservice.GetCopilotToken
	}

	copilotToken, err := fetcher(p.githubToken)
	if err != nil {
		return fmt.Errorf("failed to refresh Copilot token: %w", err)
	}

	p.token = copilotToken.Token
	p.expiresAt = copilotToken.ExpiresAt

	log.Info().Str("provider", p.instanceID).Msg("Copilot token refreshed")
	return nil
}

// LoadFromDB loads saved tokens from the database
func (p *GitHubCopilotProvider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if record == nil {
		return nil // no saved token
	}

	var tokenData map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
		return fmt.Errorf("failed to parse token data: %w", err)
	}

	if gt, ok := tokenData["github_token"].(string); ok {
		p.githubToken = gt
	}
	if ct, ok := tokenData["copilot_token"].(string); ok {
		p.token = ct
	}
	if ea, ok := tokenData["expires_at"].(float64); ok {
		p.expiresAt = int64(ea)
	}
	if name, ok := tokenData["name"].(string); ok && name != "" {
		p.name = name
	}

	// If we have a GitHub token, refresh the Copilot token if expired
	if p.githubToken != "" && (p.token == "" || time.Now().Unix() > p.expiresAt-300) {
		if err := p.RefreshToken(); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to refresh token on load")
		}
	}

	if p.token != "" {
		log.Info().Str("provider", p.instanceID).Msg("Loaded saved token")
	}

	return nil
}

// SaveToDB saves the GitHub OAuth token and Copilot API token to database
func (p *GitHubCopilotProvider) SaveToDB() error {
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"github_token":  p.githubToken,
		"copilot_token": p.token,
		"expires_at":    p.expiresAt,
		"name":          p.name,
	}

	return tokenStore.Save(p.instanceID, p.id, tokenData)
}
