// Package openaicompatprovider is the "openai-compatible" provider type.
//
// It lets users connect OmniModel to any endpoint that speaks the OpenAI
// chat-completions wire protocol (Ollama, vLLM, LM Studio, llama.cpp, etc.)
// by supplying just a base URL and an optional API key.
//
// All HTTP work is delegated to internal/providers/openaicompat — this package
// is purely config / auth / persistence logic on top of that shared layer.
package openaicompatprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	"omnimodel/internal/providers/openaicompat"
	"omnimodel/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// ─── HTTP clients ─────────────────────────────────────────────────────────────

var (
	httpClient   = &http.Client{Timeout: 120 * time.Second}
	streamClient = &http.Client{}
)

// ─── Provider ─────────────────────────────────────────────────────────────────

// Provider implements types.Provider for any OpenAI-compatible endpoint.
type Provider struct {
	instanceID   string
	name         string
	token        string // API key — may be empty for open endpoints
	baseURL      string // required; e.g. "http://localhost:11434/v1"
	config       map[string]interface{}
	configLoaded bool
}

// NewProvider creates a new openai-compatible Provider.
func NewProvider(instanceID, name string) *Provider {
	return &Provider{
		instanceID: instanceID,
		name:       name,
	}
}

// ─── Identity ─────────────────────────────────────────────────────────────────

func (p *Provider) GetID() string         { return "openai-compatible" }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }

// ─── Auth ─────────────────────────────────────────────────────────────────────

// SetupAuth persists credentials and config.
// AuthOptions.Endpoint is required; AuthOptions.APIKey is optional.
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	token, baseURL, name, cfg, err := SetupProviderAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	p.config = cfg
	p.configLoaded = true
	return nil
}

func (p *Provider) GetToken() string    { return p.token }
func (p *Provider) RefreshToken() error { return nil } // API keys don't expire

// ─── Config ───────────────────────────────────────────────────────────────────

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	return buildHeaders(p.token, false)
}

func (p *Provider) GetConfig() map[string]interface{} {
	p.ensureConfig()
	return p.config
}

func (p *Provider) ensureConfig() {
	if p.configLoaded {
		return
	}
	p.configLoaded = true
	store := database.NewProviderConfigStore()
	rec, err := store.Get(p.instanceID)
	if err != nil || rec == nil {
		return
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(rec.ConfigData), &cfg); err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("openai-compatible: failed to parse config")
		return
	}
	p.applyConfig(cfg)
}

func (p *Provider) applyConfig(cfg map[string]interface{}) {
	if p.config == nil {
		p.config = make(map[string]interface{}, len(cfg))
	}
	for k, v := range cfg {
		p.config[k] = v
	}
	if raw, ok := cfg["base_url"].(string); ok && raw != "" {
		p.baseURL = raw
	}
	if n, ok := cfg["name"].(string); ok && n != "" {
		p.name = n
	}
}

// ─── Models ───────────────────────────────────────────────────────────────────

// GetModels fetches models from <baseURL>/models.
// On failure it returns an empty list rather than an error so the provider
// can still be used when /models is not available.
func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	if p.baseURL == "" {
		return &types.ModelsResponse{Data: []types.Model{}, Object: "list"}, nil
	}
	resp, err := fetchModels(p.baseURL, p.token)
	if err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("openai-compatible: /models fetch failed; returning empty list")
		return &types.ModelsResponse{Data: []types.Model{}, Object: "list"}, nil
	}
	// Tag each model with this provider's instance ID.
	for i := range resp.Data {
		resp.Data[i].Provider = p.instanceID
	}
	return resp, nil
}

// ─── Legacy stubs ─────────────────────────────────────────────────────────────

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("openai-compatible: use the adapter for chat completions")
}
func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("openai-compatible: embeddings not implemented")
}
func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// ─── Adapter ──────────────────────────────────────────────────────────────────

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

// Adapter implements types.ProviderAdapter — pure pass-through to openaicompat.
type Adapter struct {
	provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

// RemapModel is a no-op: model IDs are forwarded as-is.
func (a *Adapter) RemapModel(model string) string { return strings.TrimSpace(model) }

func (a *Adapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	cr, err := openaicompat.BuildChatRequest(a.RemapModel(request.Model), request, false, openaicompat.Config{})
	if err != nil {
		return nil, err
	}
	return openaicompat.Execute(chatURL(a.provider.baseURL), buildHeaders(a.provider.token, false), cr)
}

func (a *Adapter) ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	cr, err := openaicompat.BuildChatRequest(a.RemapModel(request.Model), request, true, openaicompat.Config{
		IncludeUsageInStream: true,
	})
	if err != nil {
		return nil, err
	}
	return openaicompat.Stream(chatURL(a.provider.baseURL), buildHeaders(a.provider.token, true), cr)
}

// ─── LoadFromDB ───────────────────────────────────────────────────────────────

// LoadFromDB restores persisted credentials and config from the database.
func (p *Provider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("openai-compatible: failed to load token: %w", err)
	}
	if record != nil {
		var td struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal([]byte(record.TokenData), &td); err == nil {
			p.token = td.AccessToken
		}
	}

	configStore := database.NewProviderConfigStore()
	cfgRecord, err := configStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("openai-compatible: failed to load config: %w", err)
	}
	if cfgRecord != nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(cfgRecord.ConfigData), &cfg); err == nil {
			p.applyConfig(cfg)
		}
	}
	p.configLoaded = true
	log.Debug().Str("provider", p.instanceID).Str("base_url", p.baseURL).Msg("openai-compatible: loaded from DB")
	return nil
}

// ─── Auth helper ──────────────────────────────────────────────────────────────

// SetupProviderAuth validates and persists credentials.
// Returns (token, baseURL, displayName, config, error).
func SetupProviderAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, cfg map[string]interface{}, err error) {
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		return "", "", "", nil, fmt.Errorf("openai-compatible: base URL (endpoint) is required")
	}
	// Normalise: ensure https or http scheme, strip trailing slash.
	endpoint = normalizeBaseURL(endpoint)

	apiKey := strings.TrimSpace(options.APIKey)

	// Persist token (may be empty — that's fine).
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": apiKey}
	if err := tokenStore.Save(instanceID, "openai-compatible", tokenData); err != nil {
		return "", "", "", nil, fmt.Errorf("openai-compatible: failed to save token: %w", err)
	}

	displayName := deriveDisplayName(endpoint)
	config := map[string]interface{}{
		"auth_type": "api-key",
		"base_url":  endpoint,
		"name":      displayName,
	}

	// Parse and persist any pre-configured model IDs.
	if options.Models != "" {
		var modelIDs []string
		if jsonErr := json.Unmarshal([]byte(options.Models), &modelIDs); jsonErr == nil && len(modelIDs) > 0 {
			config["models"] = modelIDs
		}
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(instanceID, config); err != nil {
		return "", "", "", nil, fmt.Errorf("openai-compatible: failed to save config: %w", err)
	}

	log.Info().Str("provider", instanceID).Str("base_url", endpoint).Msg("openai-compatible: authenticated")
	return apiKey, endpoint, displayName, config, nil
}

// CanonicalInstanceID derives a stable instance ID from endpoint + key suffix.
func CanonicalInstanceID(endpoint, apiKey string) string {
	slug := urlSlug(endpoint)
	suffix := keySuffix(apiKey)
	if suffix == "" {
		return "openai-compatible-" + slug
	}
	return "openai-compatible-" + slug + "-" + suffix
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// normalizeBaseURL adds https scheme and strips trailing slash.
func normalizeBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		s = "https://" + s
	}
	return strings.TrimRight(s, "/")
}

// chatURL appends "/chat/completions" to baseURL.
func chatURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

// buildHeaders returns the HTTP headers for a request.
// Authorization header is omitted when token is empty (open endpoints).
func buildHeaders(token string, stream bool) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	if stream {
		h["Accept"] = "text/event-stream"
	}
	if token != "" {
		h["Authorization"] = "Bearer " + token
	}
	return h
}

// deriveDisplayName returns a human-readable name from a base URL.
func deriveDisplayName(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "OpenAI-Compatible"
	}
	return "OpenAI-Compatible (" + u.Host + ")"
}

// urlSlug converts a URL to a safe identifier fragment.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func urlSlug(rawURL string) string {
	u, err := url.Parse(normalizeBaseURL(rawURL))
	if err != nil || u.Host == "" {
		return "custom"
	}
	host := strings.ToLower(u.Host)
	// Strip port for well-known local hosts to keep IDs short.
	if h, _, err := splitHostPort(host); err == nil {
		host = h
	}
	return nonAlphanumRe.ReplaceAllString(host, "-")
}

// splitHostPort separates host and port without importing net (to keep deps light).
func splitHostPort(hostport string) (host, port string, err error) {
	i := strings.LastIndex(hostport, ":")
	if i < 0 {
		return hostport, "", nil
	}
	return hostport[:i], hostport[i+1:], nil
}

// keySuffix returns the last 6 chars of the API key for use in instance IDs.
func keySuffix(apiKey string) string {
	k := strings.TrimSpace(apiKey)
	if k == "" {
		return ""
	}
	if len(k) > 6 {
		return k[len(k)-6:]
	}
	return k
}

// fetchModels calls GET <baseURL>/models and returns the model list.
func fetchModels(baseURL, token string) (*types.ModelsResponse, error) {
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible: failed to build models request: %w", err)
	}
	for k, v := range buildHeaders(token, false) {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai-compatible: models fetch returned HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible: failed to read models response: %w", err)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, fmt.Errorf("openai-compatible: failed to decode models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" {
			continue
		}
		models = append(models, types.Model{
			ID:   item.ID,
			Name: item.ID,
		})
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}
