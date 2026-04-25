package generic

import (
	"encoding/json"
	"strings"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"

	alibabapkg "omnillm/internal/providers/alibaba"
	kimipkg "omnillm/internal/providers/kimi"

	"github.com/rs/zerolog/log"
)

func (p *GenericProvider) GetToken() string { return p.token }

func (p *GenericProvider) RefreshToken() error { return nil }

// GetConfig returns the provider's config map.
// The config is lazily loaded from the database on first access.
func (p *GenericProvider) GetConfig() map[string]interface{} {
	p.loadConfigFromDB()
	return p.config
}

func (p *GenericProvider) AlibabaAPIMode() string {
	if p.id != string(types.ProviderAlibaba) {
		return ""
	}
	// Always return OpenAI-compatible mode since we removed other modes
	return "openai-compatible"
}

// GetBaseURL returns configured base URL, ensuring config is loaded.
func (p *GenericProvider) GetBaseURL() string {
	p.loadConfigFromDB()
	return p.baseURL
}

func (p *GenericProvider) loadConfigRecord() (map[string]interface{}, error) {
	configStore := database.NewProviderConfigStore()
	record, err := configStore.Get(p.instanceID)
	if err != nil || record == nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (p *GenericProvider) loadConfigFromDB() {
	// sync.Once guarantees this runs exactly once across concurrent goroutines,
	// replacing the racy if p.configLoaded { return } pattern.
	p.configOnce.Do(func() {
		config, err := p.loadConfigRecord()
		if err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to load provider config")
			return
		}
		if config != nil {
			p.applyConfig(config)
		}
	})
}

func (p *GenericProvider) applyConfig(config map[string]interface{}) {
	if len(config) == 0 {
		return
	}

	if p.config == nil {
		p.config = make(map[string]interface{}, len(config))
	}
	for key, value := range config {
		p.config[key] = value
	}

	switch p.id {
	case "alibaba":
		p.baseURL = alibabapkg.NormalizeBaseURL(p.config)
	case "azure-openai":
		if endpoint, ok := firstString(config, "endpoint"); ok {
			p.baseURL = strings.TrimRight(endpoint, "/")
			// Update display name from endpoint if still using default/instance-id name
			if name := deriveAzureName(endpoint); name != "" {
				if p.name == "" || p.name == p.instanceID || p.name == providerDisplayNames["azure-openai"] {
					p.name = name
				}
			}
		}
	case "google":
		if p.baseURL == "" {
			p.baseURL = providerBaseURLs["google"]
		}
	case "kimi":
		p.baseURL = kimipkg.NormalizeBaseURL(p.config)
	}
}

func sanitizeTokenConfig(tokenData map[string]interface{}) map[string]interface{} {
	if len(tokenData) == 0 {
		return nil
	}

	filtered := make(map[string]interface{}, len(tokenData))
	for key, value := range tokenData {
		switch key {
		case "token", "api_key", "apiKey", "access_token", "github_token", "copilot_token", "refresh_token", "refreshToken", "id_token", "idToken", "expires_at", "expiresAt", "expiry", "expiry_date":
			continue
		default:
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
