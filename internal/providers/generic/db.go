package generic

import (
	"encoding/json"
	"fmt"

	"omnillm/internal/database"

	alibabapkg "omnillm/internal/providers/alibaba"

	"github.com/rs/zerolog/log"
)

// LoadFromDB loads the saved token from the database.
func (p *GenericProvider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if record != nil {
		var tokenData map[string]interface{}
		if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
			p.token = record.TokenData
			return nil
		}

		for _, key := range []string{"token", "api_key", "apiKey", "access_token", "github_token", "copilot_token"} {
			if t, ok := tokenData[key].(string); ok && t != "" {
				p.token = t
				break
			}
		}

		// Token records may also carry non-secret config (e.g. auth_type,
		// base_url, resource_url). Filter out credential fields before merging
		// into p.config so downstream config consumers only see real config keys.
		p.applyConfig(sanitizeTokenConfig(tokenData))
		config, err := p.loadConfigRecord()
		if err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to load provider config during startup")
		} else if config != nil {
			p.applyConfig(config)
		}
		p.configOnce.Do(func() {})

		if p.id == "alibaba" && p.token != "" {
			p.name = alibabapkg.APIKeyProviderName(p.config)
			log.Info().Str("instanceID", p.instanceID).Str("newName", p.name).Msg("Updated Alibaba API key provider name")
		}

		if p.id == "antigravity" {
			// Prefer email-based name if available; otherwise keep the existing name.
			if record != nil {
				var td map[string]interface{}
				if jsonErr := json.Unmarshal([]byte(record.TokenData), &td); jsonErr == nil {
					if email, ok := td["email"].(string); ok && email != "" {
						p.name = "Antigravity (" + email + ")"
					}
				}
			}
		}
	}

	if p.id == "google" && p.token != "" {
		p.baseURL = providerBaseURLs["google"]
		p.name = "Google Gemini"
	}

	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Msg("Loaded generic provider token")
	return nil
}

// ApplyTokenFromDB reloads the saved token from the database into the
// in-memory provider. It is the public equivalent of LoadFromDB and is
// called by route handlers after saving a new token (e.g. after an OAuth
// callback) to ensure the provider is immediately usable without a restart.
func (p *GenericProvider) ApplyTokenFromDB() {
	_ = p.LoadFromDB()
}
