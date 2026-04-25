package copilot

import (
	"fmt"

	"omnillm/internal/providers/types"
	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

// SetupAuth configures the provider using provided auth options. If a GitHub
// token is supplied, it will be exchanged for a Copilot token immediately.
func (p *GitHubCopilotProvider) SetupAuth(options *types.AuthOptions) error {
	// If a GitHub token is provided directly, use it
	if options != nil && options.GithubToken != "" {
		p.githubToken = options.GithubToken
		// Exchange GitHub OAuth token for Copilot token
		if err := p.RefreshToken(); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to get initial Copilot token from GitHub OAuth")
			return err
		}

		// Fetch user info to get the login
		user, err := ghservice.GetUser(p.githubToken)
		if err == nil {
			p.name = ghservice.CopilotProviderName(user)
			log.Info().Str("provider", p.instanceID).Str("name", p.name).Msg("GitHub Copilot authenticated")
		}

		return nil
	}

	// If no token provided, device code OAuth is needed but not supported in this blocking context
	// Return error instructing to use InitiateDeviceCodeFlow instead
	return fmt.Errorf("GitHub token required. Use InitiateDeviceCodeFlow endpoint for OAuth")
}

// InitiateDeviceCodeFlow starts the GitHub OAuth device code flow
func (p *GitHubCopilotProvider) InitiateDeviceCodeFlow() (*ghservice.DeviceCodeResponse, error) {
	log.Info().Str("provider", p.instanceID).Msg("Initiating GitHub OAuth device code flow")

	deviceCode, err := ghservice.GetDeviceCode()
	if err != nil {
		return nil, fmt.Errorf("failed to get device code: %w", err)
	}

	log.Info().
		Str("user_code", deviceCode.UserCode).
		Str("verification_uri", deviceCode.VerificationURI).
		Msg("GitHub OAuth device code generated")

	return deviceCode, nil
}

// PollAndCompleteDeviceCodeFlow polls for the access token after user authorizes
func (p *GitHubCopilotProvider) PollAndCompleteDeviceCodeFlow(deviceCode *ghservice.DeviceCodeResponse) error {
	log.Info().Str("provider", p.instanceID).Msg("Polling for GitHub access token")

	accessToken, err := ghservice.PollAccessToken(deviceCode)
	if err != nil {
		return fmt.Errorf("failed to poll access token: %w", err)
	}

	p.githubToken = accessToken
	log.Info().Str("provider", p.instanceID).Msg("GitHub access token received")

	// Get user info to update the provider name
	user, err := ghservice.GetUser(accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get user info after OAuth")
	} else {
		p.name = ghservice.CopilotProviderName(user)
		log.Info().
			Str("instance_id", p.instanceID).
			Str("name", p.name).
			Msg("GitHub Copilot authenticated via device code")
	}

	// Exchange GitHub token for Copilot token
	if err := p.RefreshToken(); err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to get Copilot token")
		return err
	}

	return nil
}
