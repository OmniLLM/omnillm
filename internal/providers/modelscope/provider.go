// Package modelscope implements types.Provider for Alibaba ModelScope
// (api-inference.modelscope.cn).  Unlike the DashScope provider, the adapter
// does NOT inject enable_thinking — ModelScope's hosted models do not support
// the parameter and return empty responses when it is present.
package modelscope

import (
	"encoding/json"
	"fmt"
	"net/http"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const UserAgent = "OmniLLM/1.0"

// Default ModelScope inference endpoint.
const BaseURL = "https://api-inference.modelscope.cn/v1"

var httpClient = &http.Client{Timeout: 120 * time.Second}

// TokenData is the persisted credential record for a ModelScope instance.
type TokenData struct {
	AuthType    string `json:"auth_type"`          // always "api-key"
	AccessToken string `json:"access_token"`       // the ModelScope API key
	BaseURL     string `json:"base_url,omitempty"` // optional explicit base URL
}

// Provider implements types.Provider for Alibaba ModelScope.
type Provider struct {
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	configOnce sync.Once
}

// NewProvider creates a new ModelScope Provider.
func NewProvider(instanceID, name string) *Provider {
	return &Provider{
		instanceID: instanceID,
		name:       name,
		baseURL:    BaseURL,
	}
}

// ─── Identity ────────────────────────────────────────────────────────────────

func (p *Provider) GetID() string         { return "alibaba-modelscope" }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }
func (p *Provider) SetName(name string)   { p.name = name }

// ─── Authentication ──────────────────────────────────────────────────────────

func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	if options.APIKey == "" {
		return fmt.Errorf("modelscope: API key is required")
	}

	p.token = options.APIKey

	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint != "" {
		p.baseURL = ensureBaseURL(endpoint)
	}

	p.name = displayName(p.baseURL)

	// Persist token.
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(p.instanceID, tokenData); err != nil {
		return fmt.Errorf("modelscope: failed to save token: %w", err)
	}

	// Persist config.
	cfg := map[string]interface{}{
		"auth_type": "api-key",
		"base_url":  p.baseURL,
	}
	p.config = cfg

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(p.instanceID, cfg); err != nil {
		return fmt.Errorf("modelscope: failed to save config: %w", err)
	}

	log.Info().Str("provider", p.instanceID).Str("base_url", p.baseURL).
		Msg("ModelScope authenticated via API key")

	return nil
}

func (p *Provider) GetToken() string    { return p.token }
func (p *Provider) RefreshToken() error { return nil }

// ─── API Configuration ───────────────────────────────────────────────────────

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	return headers(p.token)
}

// ─── Models ──────────────────────────────────────────────────────────────────

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	return fetchModels(p.instanceID, p.token, p.baseURL)
}

// ─── Legacy stubs ────────────────────────────────────────────────────────────

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("modelscope: use the adapter for chat completions")
}

func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("modelscope: embeddings not implemented")
}

func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// ─── Adapter ─────────────────────────────────────────────────────────────────

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

// ─── Database loading ────────────────────────────────────────────────────────

func (p *Provider) LoadFromDB() error {
	// Load token.
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("modelscope: failed to load token: %w", err)
	}
	if record != nil {
		var td TokenData
		if err := json.Unmarshal([]byte(record.TokenData), &td); err == nil {
			if td.AccessToken != "" {
				p.token = td.AccessToken
			}
		}
	}

	// Load config.
	configStore := database.NewProviderConfigStore()
	cfgRecord, err := configStore.Get(p.instanceID)
	if err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("modelscope: failed to load provider config")
	} else if cfgRecord != nil {
		var cfg map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(cfgRecord.ConfigData), &cfg); jsonErr != nil {
			log.Warn().Err(jsonErr).Str("provider", p.instanceID).Msg("modelscope: failed to parse provider config")
		} else {
			p.config = cfg
			if baseURL, ok := shared.FirstString(cfg, "base_url", "baseUrl"); ok && baseURL != "" {
				p.baseURL = ensureBaseURL(baseURL)
			}
			// Migrate legacy access_token from config if token table was empty.
			if p.token == "" {
				if at, ok := shared.FirstString(cfg, "access_token"); ok && at != "" {
					p.token = at
				}
			}
		}
	}

	if p.name == "" {
		p.name = displayName(p.baseURL)
	}

	// Mark configOnce as done so ensureConfig is a no-op.
	p.configOnce.Do(func() {})
	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Str("base_url", p.baseURL).Msg("ModelScope: loaded from DB")
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (p *Provider) ensureConfig() {
	p.configOnce.Do(func() {
		store := database.NewProviderConfigStore()
		rec, err := store.Get(p.instanceID)
		if err != nil || rec == nil {
			return
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(rec.ConfigData), &cfg); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("modelscope: failed to parse config")
			return
		}
		p.config = cfg
		if baseURL, ok := shared.FirstString(cfg, "base_url", "baseUrl"); ok && baseURL != "" {
			p.baseURL = ensureBaseURL(baseURL)
		}
	})
}

func headers(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
}

func chatURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

func ensureBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return BaseURL
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	s = strings.TrimRight(s, "/")
	if !strings.HasSuffix(s, "/v1") {
		s += "/v1"
	}
	return s
}

func displayName(baseURL string) string {
	return "ModelScope"
}

func fetchModels(instanceID, token, baseURL string) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("modelscope: not authenticated")
	}

	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("modelscope: failed to create models request: %w", err)
	}
	for k, v := range headers(token) {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("modelscope: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("modelscope: models fetch failed (%d)", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("modelscope: failed to decode models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" {
			continue
		}
		models = append(models, types.Model{
			ID:       item.ID,
			Name:     item.ID,
			Provider: instanceID,
		})
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}
