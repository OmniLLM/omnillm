// Package generic provides a generic provider implementation for alibaba, antigravity, azure-openai
package generic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	alibabapkg "omnimodel/internal/providers/alibaba"
	"omnimodel/internal/providers/types"

	"github.com/rs/zerolog/log"
)

var providerModels = map[string][]types.Model{
	"antigravity": {
		{ID: "claude-opus-4-6-thinking", Name: "Claude Opus 4.6 (Thinking)", MaxTokens: 64000, Provider: "antigravity"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6 (Thinking)", MaxTokens: 64000, Provider: "antigravity"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-3-flash", Name: "Gemini 3 Flash", MaxTokens: 65536, Provider: "antigravity"},
		{ID: "gemini-3-pro-high", Name: "Gemini 3 Pro (High)", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-3-pro-low", Name: "Gemini 3 Pro (Low)", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-3.1-flash-image", Name: "Gemini 3.1 Flash Image", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-3.1-pro-high", Name: "Gemini 3.1 Pro (High)", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gemini-3.1-pro-low", Name: "Gemini 3.1 Pro (Low)", MaxTokens: 65535, Provider: "antigravity"},
		{ID: "gpt-oss-120b-medium", Name: "GPT-OSS 120B (Medium)", MaxTokens: 32768, Provider: "antigravity"},
	},
	"alibaba": {
		{ID: "qwen3.6-plus", Name: "Qwen3.6 Plus", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3.5-omni-flash", Name: "Qwen3.5 Omni Flash", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3-coder-next", Name: "Qwen3 Coder Next", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3-coder-plus", Name: "Qwen3 Coder Plus", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3-coder-flash", Name: "Qwen3 Coder Flash", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3-max", Name: "Qwen3 Max", MaxTokens: 32768, Provider: "alibaba"},
		{ID: "qwen3-max-preview", Name: "Qwen3 Max Preview", MaxTokens: 32768, Provider: "alibaba"},
		{ID: "qwen3-32b", Name: "Qwen3-32B", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen3-235b-a22b-instruct", Name: "Qwen3-235B-A22B Instruct", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen-plus", Name: "Qwen Plus", MaxTokens: 131072, Provider: "alibaba"},
		{ID: "qwen-turbo", Name: "Qwen Turbo", MaxTokens: 1000000, Provider: "alibaba"},
	},
	"azure-openai": {
		{ID: "gpt-4o", Name: "GPT-4o", MaxTokens: 128000, Provider: "azure-openai"},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", MaxTokens: 128000, Provider: "azure-openai"},
	},
	"google": {
		// Gemini 2.5 series (latest stable + previews)
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-preview-05-20", Name: "Gemini 2.5 Flash Preview 05-20", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-pro-preview-06-05", Name: "Gemini 2.5 Pro Preview 06-05", MaxTokens: 65536, Provider: "google"},
		// Gemini 2.0 series
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash Lite", MaxTokens: 8192, Provider: "google"},
		// Gemini 1.5 series
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash-8b", Name: "Gemini 1.5 Flash-8B", MaxTokens: 8192, Provider: "google"},
	},
}

var providerBaseURLs = map[string]string{
	"antigravity":  "https://daily-cloudcode-pa.googleapis.com",
	"alibaba":      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	"azure-openai": "",
	"google":       "https://generativelanguage.googleapis.com",
}

const alibabaUserAgent = "QwenCode/0.13.2 (darwin; arm64)"

// GenericProvider is a minimal provider implementation for non-copilot providers
type GenericProvider struct {
	id         string
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
}

type GenericAdapter struct {
	provider *GenericProvider
}

func NewGenericProvider(providerType, instanceID, name string) *GenericProvider {
	baseURL := providerBaseURLs[providerType]
	displayName := name
	if displayName == "" {
		displayName = instanceID
	}
	return &GenericProvider{
		id:         providerType,
		instanceID: instanceID,
		name:       displayName,
		baseURL:    baseURL,
	}
}

func (p *GenericProvider) GetID() string         { return p.id }
func (p *GenericProvider) GetInstanceID() string { return p.instanceID }
func (p *GenericProvider) GetName() string       { return p.name }

// SetInstanceID updates the provider's in-memory instance ID after a registry rename.
func (p *GenericProvider) SetInstanceID(newID string) { p.instanceID = newID }

func (p *GenericProvider) SetupAuth(options *types.AuthOptions) error {
	switch p.id {
	case "alibaba":
		return p.setupAlibabaAuth(options)
	case "azure-openai":
		return p.setupAzureAuth(options)
	case "google":
		return p.setupGoogleAuth(options)
	default:
		return fmt.Errorf("use the admin UI to authenticate %s", p.id)
	}
}

func (p *GenericProvider) setupAlibabaAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "api-key":
		if options.APIKey == "" {
			return fmt.Errorf("alibaba: API key is required")
		}
		region := strings.TrimSpace(options.Region)
		if region == "" {
			region = "global"
		}
		plan := normalizeAlibabaAPIPlan(options.Plan)

		// Save token to database
		tokenStore := database.NewTokenStore()
		tokenData := map[string]interface{}{
			"access_token": options.APIKey,
		}
		if err := tokenStore.Save(p.instanceID, p.id, tokenData); err != nil {
			return fmt.Errorf("failed to save alibaba token: %w", err)
		}
		p.token = options.APIKey

		// Save region config
		configStore := database.NewProviderConfigStore()
		config := map[string]interface{}{
			"auth_type": "api-key",
			"region":    region,
			"plan":      plan,
		}
		if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
			config["base_url"] = endpoint
		}
		if err := configStore.Save(p.instanceID, config); err != nil {
			return fmt.Errorf("failed to save alibaba config: %w", err)
		}
		p.config = config
		p.baseURL = normalizeAlibabaBaseURL(config)
		p.name = alibabaAPIKeyProviderName(config)

		log.Info().
			Str("provider", p.instanceID).
			Str("region", region).
			Str("plan", plan).
			Msg("Alibaba authenticated via API key")
		return nil

	case "oauth":
		// The OAuth device-code flow is driven by admin.go (same pattern as GitHub Copilot).
		// This path is reached after the token has already been obtained and persisted;
		// reload the token from the database so the provider is ready to serve requests.
		return p.loadAlibabaTokenFromDB()

	default:
		return fmt.Errorf("alibaba: unsupported auth method: %s", options.Method)
	}
}

// loadAlibabaTokenFromDB reads the persisted Alibaba token and applies it to the provider.
func (p *GenericProvider) loadAlibabaTokenFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("alibaba: failed to load token from DB: %w", err)
	}
	if record == nil {
		return nil
	}

	var td alibabapkg.TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return fmt.Errorf("alibaba: failed to parse token data: %w", err)
	}

	p.token = td.AccessToken

	// Rebuild the base URL from token fields.
	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	p.applyConfig(cfg)
	return nil
}

// SaveAlibabaOAuthToken persists a completed OAuth token for this instance,
// updates in-memory state, and returns the new canonical instance ID
// (e.g. "alibaba-oauth-china") so the caller can rename in the registry.
// The token is saved under the new instance ID; the old record is removed.
func (p *GenericProvider) SaveAlibabaOAuthToken(td *alibabapkg.TokenData) (newInstanceID string, err error) {
	p.token = td.AccessToken

	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	p.applyConfig(cfg)

	// Determine region from resource_url or the resolved baseURL.
	region := "china"
	if td.ResourceURL != "" && strings.Contains(strings.ToLower(td.ResourceURL), "dashscope-intl") {
		region = "global"
	} else if p.baseURL != "" && strings.Contains(strings.ToLower(p.baseURL), "dashscope-intl") {
		region = "global"
	}

	// Try to extract email from the JWT for the display name and unique instance ID.
	email := extractEmailFromJWT(td.AccessToken)
	if email != "" {
		// Sanitize email for use in instance ID: replace @ and dots with hyphens.
		safe := strings.ReplaceAll(email, "@", "-")
		safe = strings.ReplaceAll(safe, ".", "-")
		newInstanceID = "alibaba-oauth-" + region + "-" + safe
		p.name = "Alibaba (" + email + ")"
	} else {
		suffix := shortTokenSuffix(td.AccessToken)
		newInstanceID = "alibaba-oauth-" + region + "-" + suffix
		p.name = "Alibaba OAuth (" + suffix + ")"
	}

	// Save the token under the new instance ID.
	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save(newInstanceID, p.id, td); err != nil {
		return "", fmt.Errorf("alibaba: failed to save OAuth token: %w", err)
	}

	// Remove the old temporary record (best-effort).
	if p.instanceID != newInstanceID {
		_ = tokenStore.Delete(p.instanceID)
	}

	// Update in-memory instance ID.
	p.instanceID = newInstanceID

	log.Info().
		Str("instanceID", p.instanceID).
		Str("name", p.name).
		Str("resource_url", td.ResourceURL).
		Msg("Alibaba OAuth token saved")
	return newInstanceID, nil
}

// isJWT checks if a token has the format of a JWT (xxx.yyy.zzz)
func isJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
}

func shortTokenSuffix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) >= 5 {
		return trimmed[len(trimmed)-5:]
	}
	if trimmed == "" {
		return "oauth"
	}
	return trimmed
}

// extractEmailFromJWT decodes the payload of a JWT access token and extracts the email claim.
func extractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payload := parts[1]
	// Add padding if needed.
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	return claims.Email
}

// ensureFreshAlibabaToken refreshes the OAuth token if it is about to expire.
// It returns the current (possibly refreshed) access token.
// Errors are logged but not propagated so callers always get a token to try.
func (p *GenericProvider) ensureFreshAlibabaToken() string {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil || record == nil {
		return p.token
	}

	var td alibabapkg.TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return p.token
	}

	if !alibabapkg.IsExpiringSoon(&td) {
		p.token = td.AccessToken
		return p.token
	}

	// Token is expiring – attempt a refresh.
	if td.RefreshToken == "" {
		log.Warn().Str("provider", p.instanceID).Msg("Alibaba OAuth token expiring but no refresh token available")
		return p.token
	}

	refreshed, err := alibabapkg.RefreshToken(context.Background(), td.RefreshToken)
	if err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("Alibaba OAuth token refresh failed")
		return p.token
	}

	if _, saveErr := p.SaveAlibabaOAuthToken(refreshed); saveErr != nil {
		log.Warn().Err(saveErr).Str("provider", p.instanceID).Msg("Failed to persist refreshed Alibaba token")
	}

	return p.token
}

func (p *GenericProvider) setupAzureAuth(options *types.AuthOptions) error {
	if options.APIKey == "" {
		return fmt.Errorf("azure-openai: API key is required")
	}
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"access_token": options.APIKey,
	}
	if err := tokenStore.Save(p.instanceID, p.id, tokenData); err != nil {
		return fmt.Errorf("failed to save azure token: %w", err)
	}
	p.token = options.APIKey

	// If endpoint provided, save it to config and apply immediately
	if options.Endpoint != "" {
		configStore := database.NewProviderConfigStore()
		cfg := map[string]interface{}{"endpoint": options.Endpoint}
		if err := configStore.Save(p.instanceID, cfg); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Azure: failed to save endpoint config")
		}
		p.applyConfig(cfg)
	}

	log.Info().Str("provider", p.instanceID).Msg("Azure OpenAI authenticated via API key")
	return nil
}

func (p *GenericProvider) setupGoogleAuth(options *types.AuthOptions) error {
	if options.APIKey == "" {
		return fmt.Errorf("google: API key is required")
	}
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"access_token": options.APIKey,
	}
	if err := tokenStore.Save(p.instanceID, p.id, tokenData); err != nil {
		return fmt.Errorf("failed to save google token: %w", err)
	}
	p.token = options.APIKey
	p.baseURL = providerBaseURLs["google"]

	suffix := shortTokenSuffix(options.APIKey)
	p.name = "Google Gemini (" + suffix + ")"

	log.Info().Str("provider", p.instanceID).Msg("Google Gemini authenticated via API key")
	return nil
}

func (p *GenericProvider) GetToken() string { return p.token }

func (p *GenericProvider) RefreshToken() error { return nil }

func (p *GenericProvider) GetBaseURL() string {
	p.loadConfigFromDB()
	return p.baseURL
}

func (p *GenericProvider) GetHeaders(forVision bool) map[string]string {
	p.loadConfigFromDB()

	switch p.id {
	case "alibaba":
		return p.alibabaHeaders(false)
	case "azure-openai":
		return map[string]string{
			"api-key":      p.token,
			"Content-Type": "application/json",
		}
	case "google":
		return map[string]string{
			"x-goog-api-key": p.token,
			"Content-Type":   "application/json",
		}
	}

	return map[string]string{
		"Authorization": "Bearer " + p.token,
		"Content-Type":  "application/json",
	}
}

func (p *GenericProvider) GetModels() (*types.ModelsResponse, error) {
	p.loadConfigFromDB()

	if p.id == "azure-openai" {
		if deployments := stringSliceFromConfig(p.config, "deployments"); len(deployments) > 0 {
			models := make([]types.Model, 0, len(deployments))
			for _, deployment := range deployments {
				models = append(models, types.Model{
					ID:        deployment,
					Name:      deployment,
					MaxTokens: 128000,
					Provider:  p.instanceID,
				})
			}

			return &types.ModelsResponse{Data: models, Object: "list"}, nil
		}
	}

	if p.id == "alibaba" {
		return p.getAlibabaModels()
	}

	if p.id == "google" {
		return p.fetchGoogleModels()
	}

	models := providerModels[p.id]
	if models == nil {
		models = []types.Model{}
	}
	// Set provider field to instanceID
	result := make([]types.Model, len(models))
	for i, m := range models {
		result[i] = m
		result[i].Provider = p.instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}, nil
}

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

// LoadFromDB loads the saved token from database
func (p *GenericProvider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if record != nil {
		var tokenData map[string]interface{}
		if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
			// Try plain string token
			p.token = record.TokenData
			return nil
		}

		// Try common token field names
		for _, key := range []string{"token", "api_key", "apiKey", "access_token", "github_token", "copilot_token"} {
			if t, ok := tokenData[key].(string); ok && t != "" {
				p.token = t
				break
			}
		}

		p.applyConfig(tokenData)

		// Load additional config from the config store
		p.loadConfigFromDB()

		// For Alibaba OAuth, refresh the display name from the JWT.
		// This ensures stale records (e.g. "alibaba-oauth-china" from before
		// the email was added to the instance ID) show the correct user email.
		if p.id == "alibaba" && p.token != "" {
			// Only try JWT extraction for OAuth tokens that actually look like JWTs
			authType, _ := firstString(p.config, "auth_type", "authType")

			if authType == "oauth" && isJWT(p.token) {
				if email := extractEmailFromJWT(p.token); email != "" {
					p.name = "Alibaba (" + email + ")"
					log.Info().
						Str("instanceID", p.instanceID).
						Str("extractedEmail", email).
						Str("newName", p.name).
						Msg("Updated Alibaba provider name from JWT email")
				} else {
					suffix := shortTokenSuffix(p.token)
					p.name = "Alibaba OAuth (" + suffix + ")"
					log.Warn().
						Str("instanceID", p.instanceID).
						Str("tokenSuffix", suffix).
						Msg("Failed to extract email from Alibaba JWT token; using token suffix")
				}
			} else if authType == "oauth" {
				// OAuth but not a JWT token (likely opaque OAuth token)
				suffix := shortTokenSuffix(p.token)
				p.name = "Alibaba OAuth (" + suffix + ")"
				log.Info().
					Str("instanceID", p.instanceID).
					Str("tokenSuffix", suffix).
					Msg("Alibaba OAuth provider has non-JWT token; using token suffix")
			} else {
				// API key provider - keep default naming
				p.name = alibabaAPIKeyProviderName(p.config)
				log.Info().
					Str("instanceID", p.instanceID).
					Str("authType", authType).
					Str("newName", p.name).
					Msg("Updated Alibaba API key provider name")
			}
		}
	}

	// For Google, restore the baseURL and name from the token
	if p.id == "google" && p.token != "" {
		p.baseURL = providerBaseURLs["google"]
		suffix := shortTokenSuffix(p.token)
		p.name = "Google Gemini (" + suffix + ")"
	}

	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Msg("Loaded generic provider token")
	return nil
}

func (p *GenericProvider) detectRegion() string {
	if strings.Contains(strings.ToLower(p.instanceID), "global") {
		return "global"
	}
	if p.baseURL != "" && strings.Contains(strings.ToLower(p.baseURL), "dashscope-intl") {
		return "global"
	}
	return "china"
}

func (p *GenericProvider) loadConfigFromDB() {
	configStore := database.NewProviderConfigStore()
	record, err := configStore.Get(p.instanceID)
	if err != nil || record == nil {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to parse provider config")
		return
	}

	p.applyConfig(config)
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
		p.baseURL = normalizeAlibabaBaseURL(p.config)
	case "azure-openai":
		if endpoint, ok := firstString(config, "endpoint"); ok {
			p.baseURL = strings.TrimRight(endpoint, "/")
		}
	case "google":
		if p.baseURL == "" {
			p.baseURL = providerBaseURLs["google"]
		}
	}
}

func firstString(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value, true
		}
	}

	return "", false
}

func stringSliceFromConfig(config map[string]interface{}, key string) []string {
	if config == nil {
		return nil
	}

	raw, exists := config[key]
	if !exists {
		return nil
	}

	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// fetchGoogleModels calls the Google Gemini REST API (GET /v1beta/models)
// and returns all models that support generateContent, which is required for chat.
func (p *GenericProvider) fetchGoogleModels() (*types.ModelsResponse, error) {
	if p.token == "" {
		return nil, fmt.Errorf("google: not authenticated")
	}

	base := p.baseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}

	url := base + "/v1beta/models"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("google: failed to build list-models request: %w", err)
	}
	req.Header.Set("x-goog-api-key", p.token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: list-models request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: list-models failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Google list-models response shape:
	// { "models": [ { "name": "models/gemini-2.5-flash", "displayName": "Gemini 2.5 Flash",
	//                 "outputTokenLimit": 65536, "supportedGenerationMethods": ["generateContent", ...] }, ... ] }
	var raw struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("google: failed to parse list-models response: %w", err)
	}

	var models []types.Model
	for _, m := range raw.Models {
		// Only include models that support chat (generateContent)
		supportsChat := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat {
			continue
		}

		// m.Name is "models/gemini-2.5-flash" — strip the "models/" prefix for the ID
		modelID := strings.TrimPrefix(m.Name, "models/")
		name := m.DisplayName
		if name == "" {
			name = modelID
		}
		maxTokens := m.OutputTokenLimit
		if maxTokens == 0 {
			maxTokens = 8192
		}

		models = append(models, types.Model{
			ID:          modelID,
			Name:        name,
			Description: m.Description,
			MaxTokens:   maxTokens,
			Provider:    p.instanceID,
		})
	}

	log.Info().Str("provider", p.instanceID).Int("count", len(models)).Msg("Fetched Google models from API")
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

func (p *GenericProvider) getAlibabaModels() (*types.ModelsResponse, error) {
	if p.token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated (set access_token via admin UI)")
	}

	authType, _ := firstString(p.config, "auth_type", "authType")
	if authType == "oauth" {
		return p.getAlibabaModelsHardcoded(), nil
	}

	// API-key: try fetching models from DashScope first.
	resp, err := p.fetchAlibabaModelsFromAPI()
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}

	// Fallback to the bundled catalog if live discovery is unavailable.
	log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to fetch models from API, using hardcoded list")
	return p.getAlibabaModelsHardcoded(), nil
}

func (p *GenericProvider) getAlibabaModelsHardcoded() *types.ModelsResponse {
	models := providerModels["alibaba"]
	authType, _ := firstString(p.config, "auth_type", "authType")
	if authType == "oauth" {
		oauthSupported := map[string]bool{
			"qwen3-coder-flash": true,
			"qwen3-coder-plus":  true,
		}
		filtered := make([]types.Model, 0, len(models))
		for _, model := range models {
			if oauthSupported[model.ID] {
				filtered = append(filtered, model)
			}
		}
		models = filtered
	}
	if models == nil {
		models = []types.Model{}
	}
	result := make([]types.Model, len(models))
	for i, m := range models {
		result[i] = m
		result[i].Provider = p.instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

func (p *GenericProvider) fetchAlibabaModelsFromAPI() (*types.ModelsResponse, error) {
	url := strings.TrimRight(p.baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create alibaba models request: %w", err)
	}

	for key, value := range p.alibabaHeaders(false) {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("alibaba models fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode alibaba models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		if !isAlibabaChatCompletionsModel(model.ID) {
			continue
		}

		result := types.Model{
			ID:       model.ID,
			Name:     model.ID,
			Provider: p.instanceID,
		}

		if metadata, ok := alibabaModelMetadata(model.ID); ok {
			if metadata.Name != "" {
				result.Name = metadata.Name
			}
			result.Description = metadata.Description
			result.Capabilities = metadata.Capabilities
			result.MaxTokens = metadata.MaxTokens
		}

		models = append(models, result)
	}

	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

func isAlibabaChatCompletionsModel(modelID string) bool {
	normalized := strings.ToLower(modelID)
	return !strings.Contains(normalized, "realtime")
}

func (p *GenericProvider) alibabaHeaders(stream bool) map[string]string {
	// Proactively refresh if the OAuth token is about to expire.
	token := p.ensureFreshAlibabaToken()

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	if stream {
		headers["Accept"] = "text/event-stream"
	}

	if authType, _ := firstString(p.config, "auth_type", "authType"); authType == "oauth" {
		headers["User-Agent"] = alibabaUserAgent
		headers["X-DashScope-UserAgent"] = alibabaUserAgent
		headers["X-DashScope-AuthType"] = "qwen-oauth"
		headers["X-DashScope-CacheControl"] = "enable"
		headers["X-Stainless-Runtime"] = "node"
		headers["X-Stainless-Runtime-Version"] = "v22.17.0"
		headers["X-Stainless-Lang"] = "js"
		headers["X-Stainless-Arch"] = "arm64"
		headers["X-Stainless-Os"] = "MacOS"
		headers["X-Stainless-Package-Version"] = "5.11.0"
		headers["X-Stainless-Retry-Count"] = "0"
	}

	return headers
}

func normalizeAlibabaBaseURL(config map[string]interface{}) string {
	authType, _ := firstString(config, "auth_type", "authType")

	switch authType {
	case "api-key":
		if baseURL, ok := firstString(config, "base_url", "baseUrl"); ok {
			return ensureAlibabaBaseURL(baseURL, false)
		}
		plan, _ := firstString(config, "plan")
		region, _ := firstString(config, "region")
		return defaultAlibabaAPIBaseURL(plan, region)
	case "oauth":
		if resourceURL, ok := firstString(config, "resource_url", "resourceUrl"); ok {
			return ensureAlibabaBaseURL(resourceURL, true)
		}
		// OAuth default: portal.qwen.ai/v1 (CLIProxyAPI confirmed this)
		return "https://portal.qwen.ai/v1"
	default:
		if baseURL, ok := firstString(config, "base_url", "baseUrl"); ok {
			return ensureAlibabaBaseURL(baseURL, false)
		}
		if resourceURL, ok := firstString(config, "resource_url", "resourceUrl"); ok {
			return ensureAlibabaBaseURL(resourceURL, true)
		}
	}

	return ensureAlibabaBaseURL(providerBaseURLs["alibaba"], false)
}

func normalizeAlibabaAPIPlan(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "coding", "coding-plan", "coding_plan":
		return "coding-plan"
	default:
		return "standard"
	}
}

func defaultAlibabaAPIBaseURL(plan string, region string) string {
	switch normalizeAlibabaAPIPlan(plan) {
	case "coding-plan":
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return alibabapkg.CodingPlanBaseURLChina
		}
		return alibabapkg.CodingPlanBaseURLGlobal
	default:
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return alibabapkg.BaseURLChina
		}
		return alibabapkg.BaseURLGlobal
	}
}

func alibabaAPIKeyProviderName(config map[string]interface{}) string {
	region, _ := firstString(config, "region")
	if region == "" {
		region = "global"
	}

	switch normalizeAlibabaAPIPlan(stringValueOrEmpty(config["plan"])) {
	case "coding-plan":
		return "Alibaba Coding Plan (" + region + ")"
	default:
		return "Alibaba DashScope Standard (" + region + ")"
	}
}

func stringValueOrEmpty(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

// ensureAlibabaBaseURL normalizes a base URL. When forOAuth is true, it uses
// portal.qwen.ai/v1 as the fallback (confirmed by CLIProxyAPI test cases).
func ensureAlibabaBaseURL(raw string, forOAuth bool) string {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		if forOAuth {
			baseURL = "https://portal.qwen.ai/v1"
		} else {
			baseURL = providerBaseURLs["alibaba"]
		}
	}

	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}

	return baseURL
}

func alibabaModelMetadata(modelID string) (types.Model, bool) {
	for _, model := range providerModels["alibaba"] {
		if model.ID == modelID {
			return model, true
		}
	}

	return types.Model{}, false
}

// ─── GenericAdapter ────────────────────────────────────────────────────────

func (a *GenericAdapter) GetProvider() types.Provider { return a.provider }

func (a *GenericAdapter) RemapModel(model string) string {
	switch a.provider.id {
	case "antigravity":
		switch {
		case strings.HasPrefix(model, "claude-opus-4"):
			return "claude-opus-4-6-thinking"
		case strings.HasPrefix(model, "claude-sonnet-4"):
			return "claude-sonnet-4-6"
		case strings.HasPrefix(model, "claude-haiku-4"):
			return "gemini-3-flash"
		}
	}
	return model
}

func (a *GenericAdapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !isAlibabaChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.executeOpenAI(a.alibabaURL(), a.provider.alibabaHeaders(false), request)
	case "azure-openai":
		if isAzureResponsesApiModel(a.RemapModel(request.Model)) {
			return a.executeAzureResponses(request)
		}
		url, err := a.azureURL(a.RemapModel(request.Model))
		if err != nil {
			return nil, err
		}
		return a.executeOpenAI(url, a.azureHeaders(), request)
	case "google":
		return a.executeGoogle(request)
	case "antigravity":
		return a.collectStream(request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

func (a *GenericAdapter) ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !isAlibabaChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.streamOpenAI(a.alibabaURL(), a.provider.alibabaHeaders(true), request)
	case "azure-openai":
		if isAzureResponsesApiModel(a.RemapModel(request.Model)) {
			return a.streamAzureResponses(request)
		}
		url, err := a.azureURL(a.RemapModel(request.Model))
		if err != nil {
			return nil, err
		}
		return a.streamOpenAI(url, a.azureHeaders(), request)
	case "google":
		return a.streamGoogle(request)
	case "antigravity":
		return a.streamAntigravity(request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

// ─── URL / header helpers ──────────────────────────────────────────────────

func (a *GenericAdapter) alibabaURL() string {
	base := a.provider.baseURL
	if base == "" {
		base = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	}
	return base + "/chat/completions"
}

func (a *GenericAdapter) openAIHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + a.provider.token,
		"Content-Type":  "application/json",
	}
}

func (a *GenericAdapter) azureURL(deployment string) (string, error) {
	endpoint := a.provider.baseURL
	if endpoint == "" {
		return "", fmt.Errorf("azure-openai endpoint not configured; set it via the admin UI")
	}
	apiVersion := "2024-08-01-preview"
	if v, ok := firstString(a.provider.config, "api_version", "apiVersion"); ok {
		apiVersion = v
	}
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		endpoint, deployment, apiVersion), nil
}

func (a *GenericAdapter) azureHeaders() map[string]string {
	return map[string]string{
		"api-key":      a.provider.token,
		"Content-Type": "application/json",
	}
}

func (a *GenericAdapter) azureResponsesURL() (string, error) {
	endpoint := a.provider.baseURL
	if endpoint == "" {
		return "", fmt.Errorf("azure-openai endpoint not configured; set it via the admin UI")
	}
	return endpoint + "/openai/v1/responses", nil
}

// isAzureResponsesApiModel mirrors the TypeScript isResponsesApiModel logic.
// All GPT-5.x Azure models use /openai/v1/responses; chat/completions returns 400 for most.
func isAzureResponsesApiModel(model string) bool {
	modelLower := strings.ToLower(model)
	patterns := []string{
		"gpt-5.1-codex",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5-codex",
		"gpt-5.4",
	}
	for _, p := range patterns {
		if strings.Contains(modelLower, p) {
			return true
		}
	}
	return false
}

// buildAzureResponsesPayload converts a CIF request to the Azure Responses API format.
// This mirrors the TypeScript canonicalRequestToResponsesPayload in adapter.ts.
func (a *GenericAdapter) buildAzureResponsesPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)

	input := cifMessagesToResponsesInput(request.Messages)

	maxOutputTokens := 4000
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		maxOutputTokens = *request.MaxTokens
		if maxOutputTokens < 16 {
			maxOutputTokens = 16
		}
	}

	payload := map[string]interface{}{
		"model":             model,
		"input":             input,
		"max_output_tokens": maxOutputTokens,
		"generate":          true,
		"store":             false,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		payload["instructions"] = *request.SystemPrompt
	}

	// gpt-5.4-pro and gpt-5.1-codex-max do not support the temperature parameter
	modelLower := strings.ToLower(model)
	supportsTemperature := !strings.Contains(modelLower, "gpt-5.4-pro") &&
		!strings.Contains(modelLower, "gpt-5.1-codex-max")
	if supportsTemperature {
		if request.Temperature != nil {
			payload["temperature"] = *request.Temperature
		} else {
			payload["temperature"] = 0.1
		}
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			// Responses API uses flat format: {type, name, description, parameters}
			// unlike Chat Completions: {type: "function", function: {name, description, parameters}}
			t := map[string]interface{}{
				"type":       "function",
				"name":       tool.Name,
				"parameters": tool.ParametersSchema,
			}
			if tool.Description != nil {
				t["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	return payload
}

// cifMessagesToResponsesInput converts CIF messages to the Responses API input array format.
func cifMessagesToResponsesInput(messages []cif.CIFMessage) []map[string]interface{} {
	var input []map[string]interface{}

	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			input = append(input, map[string]interface{}{
				"type": "message",
				"role": "system",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": m.Content},
				},
			})
		case cif.CIFUserMessage:
			var textBlocks []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBlocks = append(textBlocks, map[string]interface{}{
						"type": "input_text",
						"text": p.Text,
					})
				case cif.CIFToolResultPart:
					if len(textBlocks) > 0 {
						input = append(input, map[string]interface{}{
							"type":    "message",
							"role":    "user",
							"content": textBlocks,
						})
						textBlocks = nil
					}
					callID := azureToolCallID(p.ToolCallID)
					input = append(input, map[string]interface{}{
						"type":    "function_call_output",
						"call_id": callID,
						"output":  p.Content,
					})
				}
			}
			if len(textBlocks) > 0 {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "user",
					"content": textBlocks,
				})
			}
		case cif.CIFAssistantMessage:
			var textBlocks []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBlocks = append(textBlocks, map[string]interface{}{
						"type": "output_text",
						"text": p.Text,
					})
				case cif.CIFToolCallPart:
					if len(textBlocks) > 0 {
						input = append(input, map[string]interface{}{
							"type":    "message",
							"role":    "assistant",
							"content": textBlocks,
						})
						textBlocks = nil
					}
					callID := azureToolCallID(p.ToolCallID)
					argsBytes, _ := json.Marshal(p.ToolArguments)
					input = append(input, map[string]interface{}{
						"type":      "function_call",
						"id":        callID,
						"call_id":   callID,
						"name":      p.ToolName,
						"arguments": string(argsBytes),
					})
				}
			}
			if len(textBlocks) > 0 {
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "assistant",
					"content": textBlocks,
				})
			}
		}
	}

	return input
}

// azureToolCallID ensures tool call IDs start with "fc" as required by Azure.
func azureToolCallID(id string) string {
	if strings.HasPrefix(id, "fc") {
		return id
	}
	// Strip common prefixes like "call_" and replace with "fc_"
	stripped := strings.TrimPrefix(id, "call_")
	return "fc_" + stripped
}

// executeAzureResponses calls /openai/v1/responses and converts the response to CIF.
func (a *GenericAdapter) executeAzureResponses(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}

	payload := a.buildAzureResponsesPayload(request)
	payload["stream"] = false

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range a.azureHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var respMap map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respMap); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return azureResponsesRespToCIF(respMap, request.Model), nil
}

// streamAzureResponses uses executeAzureResponses and wraps the result as stream events.
// The Responses API SSE format is complex; non-streaming is simpler and sufficient.
func (a *GenericAdapter) streamAzureResponses(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	cifResp, err := a.executeAzureResponses(request)
	if err != nil {
		return nil, err
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go func() {
		defer close(eventCh)
		eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: cifResp.ID, Model: cifResp.Model}
		for i, part := range cifResp.Content {
			switch p := part.(type) {
			case cif.CIFTextPart:
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        i,
					ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
					Delta:        cif.TextDelta{Type: "text_delta", Text: p.Text},
				}
			case cif.CIFToolCallPart:
				argsBytes, _ := json.Marshal(p.ToolArguments)
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: i,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    p.ToolCallID,
						ToolName:      p.ToolName,
						ToolArguments: map[string]interface{}{},
					},
					Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: string(argsBytes)},
				}
			}
		}
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cifResp.StopReason, Usage: cifResp.Usage}
	}()
	return eventCh, nil
}

// azureResponsesRespToCIF converts an Azure Responses API response to CIF format.
func azureResponsesRespToCIF(resp map[string]interface{}, originalModel string) *cif.CanonicalResponse {
	id, _ := resp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixMilli())
	}

	var content []cif.CIFContentPart
	stopReason := cif.StopReasonEndTurn

	output, _ := resp["output"].([]interface{})
	for _, item := range output {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		switch itemType {
		case "message":
			contentItems, _ := itemMap["content"].([]interface{})
			for _, block := range contentItems {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				if blockType, _ := blockMap["type"].(string); blockType == "output_text" {
					text, _ := blockMap["text"].(string)
					content = append(content, cif.CIFTextPart{Type: "text", Text: text})
				}
			}
		case "function_call":
			callID, _ := itemMap["id"].(string)
			if callID == "" {
				callID, _ = itemMap["call_id"].(string)
			}
			name, _ := itemMap["name"].(string)
			argsStr, _ := itemMap["arguments"].(string)
			var args map[string]interface{}
			json.Unmarshal([]byte(argsStr), &args) //nolint:errcheck
			if args == nil {
				args = map[string]interface{}{}
			}
			content = append(content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    callID,
				ToolName:      name,
				ToolArguments: args,
			})
			stopReason = cif.StopReasonToolUse
		}
	}

	if status, _ := resp["status"].(string); status == "incomplete" {
		stopReason = cif.StopReasonMaxTokens
	}

	var usage *cif.CIFUsage
	if usageMap, ok := resp["usage"].(map[string]interface{}); ok {
		inputTokens, _ := usageMap["input_tokens"].(float64)
		outputTokens, _ := usageMap["output_tokens"].(float64)
		usage = &cif.CIFUsage{InputTokens: int(inputTokens), OutputTokens: int(outputTokens)}
	}

	return &cif.CanonicalResponse{
		ID:         id,
		Model:      originalModel,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// ─── OpenAI-compatible execution (Alibaba + Azure) ────────────────────────

func (a *GenericAdapter) buildOpenAIPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	messages := cifMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}
	if a.provider.id == "alibaba" {
		if authType, _ := firstString(a.provider.config, "auth_type", "authType"); authType == "oauth" {
			messages = ensureAlibabaOAuthSystemMessage(messages)
		}
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		// Azure OpenAI: newer models (gpt-5.3+, gpt-5.4, gpt-6) require
		// max_completion_tokens instead of max_tokens
		modelLower := strings.ToLower(request.Model)
		if a.provider.id == "azure-openai" &&
			(strings.Contains(modelLower, "gpt-5.3") ||
				strings.Contains(modelLower, "gpt-5.4") ||
				strings.Contains(modelLower, "gpt-6")) {
			payload["max_completion_tokens"] = *request.MaxTokens
		} else {
			payload["max_tokens"] = *request.MaxTokens
		}
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}
	if request.UserID != nil {
		payload["user"] = *request.UserID
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			t := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":       tool.Name,
					"parameters": tool.ParametersSchema,
				},
			}
			if tool.Description != nil {
				t["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
	} else if a.provider.id == "alibaba" {
		// Qwen3 requires at least one tool (dummy injection workaround)
		payload["tools"] = []map[string]interface{}{{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "do_not_call_me",
				"description": "Do not call this tool",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		}}
	}

	if request.ToolChoice != nil {
		if toolChoice := convertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	// Azure: model is in the URL (deployment), not the body
	if a.provider.id == "azure-openai" {
		delete(payload, "model")
	}

	return payload
}

// portal.qwen.ai rejects OAuth chat-completions requests unless a leading system
// message is present, so we collapse any existing system text into one message.
func ensureAlibabaOAuthSystemMessage(messages []map[string]interface{}) []map[string]interface{} {
	systemParts := []string{"You are Qwen Code."}
	nonSystemMessages := make([]map[string]interface{}, 0, len(messages))

	for _, message := range messages {
		role, _ := message["role"].(string)
		if !strings.EqualFold(role, "system") {
			nonSystemMessages = append(nonSystemMessages, message)
			continue
		}

		text := strings.TrimSpace(openAIMessageContentText(message["content"]))
		if text == "" || text == "You are Qwen Code." {
			continue
		}
		systemParts = append(systemParts, text)
	}

	result := make([]map[string]interface{}, 0, len(nonSystemMessages)+1)
	result = append(result, map[string]interface{}{
		"role":    "system",
		"content": strings.Join(systemParts, "\n\n"),
	})
	result = append(result, nonSystemMessages...)
	return result
}

func openAIMessageContentText(content interface{}) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]interface{}:
		if text, ok := value["text"].(string); ok {
			return strings.TrimSpace(text)
		}
		if nested, ok := value["content"]; ok {
			return openAIMessageContentText(nested)
		}
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			text := openAIMessageContentText(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	}

	return ""
}

func convertCanonicalToolChoiceToOpenAI(toolChoice interface{}) interface{} {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "none", "auto", "required":
			return choice
		default:
			return nil
		}
	case map[string]interface{}:
		functionName, _ := choice["functionName"].(string)
		if functionName == "" {
			if function, ok := choice["function"].(map[string]interface{}); ok {
				functionName, _ = function["name"].(string)
			}
		}
		if functionName == "" {
			return nil
		}

		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": functionName,
			},
		}
	default:
		return nil
	}
}

func (a *GenericAdapter) executeOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return openAIRespToCIF(openaiResp), nil
}

func (a *GenericAdapter) streamOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	if a.provider.id == "alibaba" {
		payload["stream_options"] = map[string]interface{}{"include_usage": true}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("streaming API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseOpenAISSE(resp.Body, eventCh)
	return eventCh, nil
}

// collectStream runs ExecuteStream and assembles a CanonicalResponse (used by antigravity Execute)
func (a *GenericAdapter) collectStream(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := a.ExecuteStream(request)
	if err != nil {
		return nil, err
	}

	response := &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	var textBuf strings.Builder
	toolCalls := make(map[int]*cif.CIFToolCallPart)
	toolArgBufs := make(map[int]*strings.Builder)

	for event := range ch {
		switch e := event.(type) {
		case cif.CIFStreamStart:
			response.ID = e.ID
			response.Model = e.Model
		case cif.CIFContentDelta:
			switch d := e.Delta.(type) {
			case cif.TextDelta:
				textBuf.WriteString(d.Text)
			case cif.ToolArgumentsDelta:
				if toolArgBufs[e.Index] == nil {
					toolArgBufs[e.Index] = &strings.Builder{}
				}
				toolArgBufs[e.Index].WriteString(d.PartialJSON)
				if e.ContentBlock != nil {
					if tc, ok := e.ContentBlock.(cif.CIFToolCallPart); ok {
						toolCalls[e.Index] = &tc
					}
				}
			}
		case cif.CIFStreamEnd:
			response.StopReason = e.StopReason
			response.Usage = e.Usage
		case cif.CIFStreamError:
			return nil, fmt.Errorf("stream error: %s", e.Error.Message)
		}
	}

	if textBuf.Len() > 0 {
		response.Content = append(response.Content, cif.CIFTextPart{Type: "text", Text: textBuf.String()})
	}
	for idx, tc := range toolCalls {
		finalTC := *tc
		if buf, ok := toolArgBufs[idx]; ok {
			json.Unmarshal([]byte(buf.String()), &finalTC.ToolArguments)
		}
		response.Content = append(response.Content, finalTC)
	}

	return response, nil
}

// ─── Google Gemini API ─────────────────────────────────────────────────────

func (a *GenericAdapter) buildGooglePayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	contents := cifMessagesToGemini(request.Messages)

	payload := map[string]interface{}{
		"model":    model,
		"contents": contents,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		payload["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": *request.SystemPrompt},
			},
		}
	}

	if len(request.Tools) > 0 {
		decls := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			decl := map[string]interface{}{
				"name":       tool.Name,
				"parameters": sanitizeGeminiSchema(tool.ParametersSchema),
			}
			if tool.Description != nil {
				decl["description"] = *tool.Description
			}
			decls = append(decls, decl)
		}
		payload["tools"] = []map[string]interface{}{
			{"functionDeclarations": decls},
		}
	}

	genConfig := map[string]interface{}{}
	if request.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		genConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		genConfig["topP"] = *request.TopP
	}
	if request.Stop != nil {
		genConfig["stopSequences"] = request.Stop
	}
	if len(genConfig) > 0 {
		payload["generationConfig"] = genConfig
	}

	return payload
}

func (a *GenericAdapter) googleURL(model string) string {
	base := a.provider.baseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	return base + "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
}

func (a *GenericAdapter) executeGoogle(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := a.streamGoogle(request)
	if err != nil {
		return nil, err
	}
	return a.collectStreamFromChan(ch)
}

func (a *GenericAdapter) streamGoogle(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.provider.token == "" {
		return nil, fmt.Errorf("google: not authenticated (set API key via admin UI)")
	}

	model := a.RemapModel(request.Model)

	contents := cifMessagesToGemini(request.Messages)

	geminiReq := map[string]interface{}{
		"contents": contents,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		geminiReq["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": *request.SystemPrompt},
			},
		}
	}

	if len(request.Tools) > 0 {
		decls := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			decl := map[string]interface{}{
				"name":       tool.Name,
				"parameters": sanitizeGeminiSchema(tool.ParametersSchema),
			}
			if tool.Description != nil {
				decl["description"] = *tool.Description
			}
			decls = append(decls, decl)
		}
		geminiReq["tools"] = []map[string]interface{}{
			{"functionDeclarations": decls},
		}
	}

	genConfig := map[string]interface{}{}
	if request.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		genConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		genConfig["topP"] = *request.TopP
	}
	if request.Stop != nil {
		genConfig["stopSequences"] = request.Stop
	}
	if len(genConfig) > 0 {
		geminiReq["generationConfig"] = genConfig
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal google request: %w", err)
	}

	base := a.provider.baseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	// Use v1beta which supports the latest Gemini models
	url := base + "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range a.provider.GetHeaders(false) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("google API failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseGoogleGeminiSSE(resp.Body, eventCh)
	return eventCh, nil
}

// parseGoogleGeminiSSE parses Google Gemini streaming responses (SSE format).
// The response format is identical to the antigravity SSE format.
func parseGoogleGeminiSSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)

	var streamStartSent bool
	var textIndex int
	toolCallIndex := 1000

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var envelope struct {
			Candidates []struct {
				Content struct {
					Parts []map[string]interface{} `json:"parts"`
					Role  string                   `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}

		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			log.Warn().Err(err).Msg("Failed to parse Google Gemini SSE line")
			continue
		}

		if len(envelope.Candidates) == 0 {
			continue
		}

		if !streamStartSent {
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: randomID(), Model: "google"}
			streamStartSent = true
		}

		candidate := envelope.Candidates[0]

		for _, part := range candidate.Content.Parts {
			if text, ok := part["text"].(string); ok && text != "" {
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        textIndex,
					ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
					Delta:        cif.TextDelta{Type: "text_delta", Text: text},
				}
			} else if fcMap, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fcMap["name"].(string)
				args := normalizeToolArguments(fcMap["args"])
				argsJSON, _ := json.Marshal(args)
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: toolCallIndex,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    fmt.Sprintf("call_%s", randomID()),
						ToolName:      name,
						ToolArguments: args,
					},
					Delta: cif.ToolArgumentsDelta{
						Type:        "tool_arguments_delta",
						PartialJSON: string(argsJSON),
					},
				}
				toolCallIndex++
			}
		}

		if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" {
			usage := envelope.UsageMetadata
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: googleStopReason(candidate.FinishReason),
				Usage: &cif.CIFUsage{
					InputTokens:  usage.PromptTokenCount,
					OutputTokens: usage.CandidatesTokenCount,
				},
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
		return
	}

	// End of stream without explicit finish reason
	if streamStartSent {
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
	}
}

func googleStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "STOP":
		return cif.StopReasonEndTurn
	case "MAX_TOKENS":
		return cif.StopReasonMaxTokens
	case "FUNCTION_CALL":
		return cif.StopReasonToolUse
	case "SAFETY", "RECITATION", "LANGUAGE":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}

// collectStreamFromChan assembles a CanonicalResponse from a stream channel
func (a *GenericAdapter) collectStreamFromChan(ch <-chan cif.CIFStreamEvent) (*cif.CanonicalResponse, error) {
	response := &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	var textBuf strings.Builder
	toolCalls := make(map[int]*cif.CIFToolCallPart)
	toolArgBufs := make(map[int]*strings.Builder)

	for event := range ch {
		switch e := event.(type) {
		case cif.CIFStreamStart:
			response.ID = e.ID
			response.Model = e.Model
		case cif.CIFContentDelta:
			switch d := e.Delta.(type) {
			case cif.TextDelta:
				textBuf.WriteString(d.Text)
			case cif.ToolArgumentsDelta:
				if toolArgBufs[e.Index] == nil {
					toolArgBufs[e.Index] = &strings.Builder{}
				}
				toolArgBufs[e.Index].WriteString(d.PartialJSON)
				if e.ContentBlock != nil {
					if tc, ok := e.ContentBlock.(cif.CIFToolCallPart); ok {
						toolCalls[e.Index] = &tc
					}
				}
			}
		case cif.CIFStreamEnd:
			response.StopReason = e.StopReason
			response.Usage = e.Usage
		case cif.CIFStreamError:
			return nil, fmt.Errorf("stream error: %s", e.Error.Message)
		}
	}

	if textBuf.Len() > 0 {
		response.Content = append(response.Content, cif.CIFTextPart{Type: "text", Text: textBuf.String()})
	}
	for idx, tc := range toolCalls {
		finalTC := *tc
		if buf, ok := toolArgBufs[idx]; ok {
			json.Unmarshal([]byte(buf.String()), &finalTC.ToolArguments)
		}
		response.Content = append(response.Content, finalTC)
	}

	return response, nil
}

// ─── Antigravity (Gemini Cloud Code API) ─────────────────────────────────

func (a *GenericAdapter) streamAntigravity(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.provider.token == "" {
		return nil, fmt.Errorf("antigravity: not authenticated (set access_token via admin UI)")
	}

	model := a.RemapModel(request.Model)
	project, _ := firstString(a.provider.config, "project_id", "project")

	contents := cifMessagesToGemini(request.Messages)

	geminiReq := map[string]interface{}{
		"sessionId": randomID(),
		"contents":  contents,
	}

	if request.SystemPrompt != nil && *request.SystemPrompt != "" {
		geminiReq["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": *request.SystemPrompt},
			},
		}
	}

	if len(request.Tools) > 0 {
		decls := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			decl := map[string]interface{}{
				"name":       tool.Name,
				"parameters": sanitizeGeminiSchema(tool.ParametersSchema),
			}
			if tool.Description != nil {
				decl["description"] = *tool.Description
			}
			decls = append(decls, decl)
		}
		geminiReq["tools"] = []map[string]interface{}{
			{"functionDeclarations": decls},
		}
	}

	genConfig := map[string]interface{}{}
	if request.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		genConfig["temperature"] = *request.Temperature
	}
	if len(genConfig) > 0 {
		geminiReq["generationConfig"] = genConfig
	}

	payload := map[string]interface{}{
		"model":       model,
		"userAgent":   "antigravity/1.19.6",
		"requestType": "agent",
		"requestId":   randomID(),
		"request":     geminiReq,
	}
	if project != "" {
		payload["project"] = project
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal antigravity request: %w", err)
	}

	base := a.provider.baseURL
	if base == "" {
		base = "https://daily-cloudcode-pa.googleapis.com"
	}
	url := base + "/v1internal:streamGenerateContent?alt=sse"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.provider.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/1.19.6")
	req.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	req.Header.Set("Client-Metadata", `{"ideType":"IDE_UNSPECIFIED","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`)

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("antigravity request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("antigravity API failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go parseAntigravitySSE(resp.Body, eventCh)
	return eventCh, nil
}

// ─── SSE/stream parsers ───────────────────────────────────────────────────

func parseOpenAISSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var streamStartSent bool
	var contentBlockIndex int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
			return
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Warn().Err(err).Msg("Failed to parse OpenAI SSE chunk")
			continue
		}

		if !streamStartSent {
			id, _ := chunk["id"].(string)
			model, _ := chunk["model"].(string)
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: id, Model: model}
			streamStartSent = true
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}
		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
			var usage *cif.CIFUsage
			if usageMap, ok := chunk["usage"].(map[string]interface{}); ok {
				promptTokens, _ := usageMap["prompt_tokens"].(float64)
				completionTokens, _ := usageMap["completion_tokens"].(float64)
				usage = &cif.CIFUsage{
					InputTokens:  int(promptTokens),
					OutputTokens: int(completionTokens),
				}
			}
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: openAIStopReason(finishReason),
				Usage:      usage,
			}
			return
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			eventCh <- cif.CIFContentDelta{
				Type:         "content_delta",
				Index:        contentBlockIndex,
				ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
				Delta:        cif.TextDelta{Type: "text_delta", Text: content},
			}
		}

		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}
				if id, ok := tcMap["id"].(string); ok && id != "" {
					contentBlockIndex++
					funcMap, _ := tcMap["function"].(map[string]interface{})
					name, _ := funcMap["name"].(string)
					eventCh <- cif.CIFContentDelta{
						Type:  "content_delta",
						Index: contentBlockIndex,
						ContentBlock: cif.CIFToolCallPart{
							Type:          "tool_call",
							ToolCallID:    id,
							ToolName:      name,
							ToolArguments: map[string]interface{}{},
						},
						Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""},
					}
				} else if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						eventCh <- cif.CIFContentDelta{
							Type:  "content_delta",
							Index: contentBlockIndex,
							Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: args},
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("provider", "openai-compat").Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
	}
}

func parseAntigravitySSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)

	var streamStartSent bool
	var textIndex int
	toolCallIndex := 1000 // start high to not collide with text block

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var envelope struct {
			Response struct {
				Candidates []struct {
					Content struct {
						Parts []map[string]interface{} `json:"parts"`
						Role  string                   `json:"role"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
				UsageMetadata struct {
					PromptTokenCount     int `json:"promptTokenCount"`
					CandidatesTokenCount int `json:"candidatesTokenCount"`
				} `json:"usageMetadata"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			log.Warn().Err(err).Msg("Failed to parse antigravity SSE line")
			continue
		}

		if len(envelope.Response.Candidates) == 0 {
			continue
		}

		if !streamStartSent {
			eventCh <- cif.CIFStreamStart{Type: "stream_start", ID: randomID(), Model: "antigravity"}
			streamStartSent = true
		}

		candidate := envelope.Response.Candidates[0]

		for _, part := range candidate.Content.Parts {
			if text, ok := part["text"].(string); ok && text != "" {
				eventCh <- cif.CIFContentDelta{
					Type:         "content_delta",
					Index:        textIndex,
					ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
					Delta:        cif.TextDelta{Type: "text_delta", Text: text},
				}
			} else if fcMap, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fcMap["name"].(string)
				args := normalizeToolArguments(fcMap["args"])
				argsJSON, _ := json.Marshal(args)
				eventCh <- cif.CIFContentDelta{
					Type:  "content_delta",
					Index: toolCallIndex,
					ContentBlock: cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    fmt.Sprintf("call_%s", randomID()),
						ToolName:      name,
						ToolArguments: args,
					},
					Delta: cif.ToolArgumentsDelta{
						Type:        "tool_arguments_delta",
						PartialJSON: string(argsJSON),
					},
				}
				toolCallIndex++
			}
		}

		if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" {
			usage := envelope.Response.UsageMetadata
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: antigravityStopReason(candidate.FinishReason),
				Usage: &cif.CIFUsage{
					InputTokens:  usage.PromptTokenCount,
					OutputTokens: usage.CandidatesTokenCount,
				},
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
		return
	}

	// End of stream without explicit finish reason
	if streamStartSent {
		eventCh <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
	}
}

func normalizeToolArguments(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case nil:
		return map[string]interface{}{}
	case map[string]interface{}:
		if value == nil {
			return map[string]interface{}{}
		}
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return map[string]interface{}{}
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed != nil {
			return parsed
		}

		return map[string]interface{}{"value": value}
	case []interface{}:
		return map[string]interface{}{"items": value}
	default:
		return map[string]interface{}{"value": value}
	}
}

// ─── CIF → OpenAI message conversion ─────────────────────────────────────

func cifMessagesToOpenAI(messages []cif.CIFMessage) []map[string]interface{} {
	var result []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			result = append(result, map[string]interface{}{
				"role":    "system",
				"content": m.Content,
			})
		case cif.CIFUserMessage:
			openaiMsg := map[string]interface{}{"role": "user"}
			if len(m.Content) == 1 {
				if textPart, ok := m.Content[0].(cif.CIFTextPart); ok {
					openaiMsg["content"] = textPart.Text
					result = append(result, openaiMsg)
					continue
				}
			}
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"type": "text", "text": p.Text})
				case cif.CIFToolResultPart:
					result = append(result, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": p.ToolCallID,
						"content":      p.Content,
					})
					continue
				case cif.CIFImagePart:
					imgURL := map[string]interface{}{}
					if p.Data != nil {
						imgURL["url"] = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
					} else if p.URL != nil {
						imgURL["url"] = *p.URL
					}
					parts = append(parts, map[string]interface{}{"type": "image_url", "image_url": imgURL})
				}
			}
			if len(parts) > 0 {
				openaiMsg["content"] = parts
				result = append(result, openaiMsg)
			}
		case cif.CIFAssistantMessage:
			openaiMsg := map[string]interface{}{"role": "assistant"}
			var textContent string
			var toolCalls []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textContent += p.Text
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   p.ToolCallID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      p.ToolName,
							"arguments": string(args),
						},
					})
				}
			}
			if textContent != "" {
				openaiMsg["content"] = textContent
			}
			if len(toolCalls) > 0 {
				openaiMsg["tool_calls"] = toolCalls
			}
			result = append(result, openaiMsg)
		}
	}
	return result
}

// ─── CIF → Gemini message conversion ─────────────────────────────────────

func cifMessagesToGemini(messages []cif.CIFMessage) []map[string]interface{} {
	var contents []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			// System messages are handled via systemInstruction; skip here
			_ = m
		case cif.CIFUserMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolResultPart:
					parts = append(parts, map[string]interface{}{
						"functionResponse": map[string]interface{}{
							"name":     p.ToolName,
							"response": map[string]interface{}{"output": p.Content},
						},
					})
				case cif.CIFImagePart:
					if p.Data != nil {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": p.MediaType,
								"data":     *p.Data,
							},
						})
					}
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "user", "parts": parts})
			}
		case cif.CIFAssistantMessage:
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"text": p.Text})
				case cif.CIFToolCallPart:
					parts = append(parts, map[string]interface{}{
						"functionCall": map[string]interface{}{
							"name": p.ToolName,
							"args": p.ToolArguments,
						},
					})
				case cif.CIFThinkingPart:
					parts = append(parts, map[string]interface{}{"text": p.Thinking})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]interface{}{"role": "model", "parts": parts})
			}
		}
	}
	return contents
}

// sanitizeGeminiSchema removes fields that Gemini rejects from JSON Schema objects
func sanitizeGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	blocked := map[string]bool{
		"$schema": true, "$id": true, "patternProperties": true, "prefill": true,
		"enumTitles": true, "deprecated": true, "propertyNames": true,
		"exclusiveMinimum": true, "exclusiveMaximum": true, "const": true,
	}
	clean := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if blocked[k] {
			continue
		}
		// Recurse into nested objects/arrays
		switch nested := v.(type) {
		case map[string]interface{}:
			clean[k] = sanitizeGeminiSchema(nested)
		case []interface{}:
			cleaned := make([]interface{}, 0, len(nested))
			for _, item := range nested {
				if m, ok := item.(map[string]interface{}); ok {
					cleaned = append(cleaned, sanitizeGeminiSchema(m))
				} else {
					cleaned = append(cleaned, item)
				}
			}
			clean[k] = cleaned
		default:
			clean[k] = v
		}
	}
	return clean
}

// ─── Response / stop reason helpers ──────────────────────────────────────

func openAIRespToCIF(resp map[string]interface{}) *cif.CanonicalResponse {
	id, _ := resp["id"].(string)
	model, _ := resp["model"].(string)
	result := &cif.CanonicalResponse{ID: id, Model: model, StopReason: cif.StopReasonEndTurn}

	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if fr, ok := choice["finish_reason"].(string); ok {
				result.StopReason = openAIStopReason(fr)
			}
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok && content != "" {
					result.Content = append(result.Content, cif.CIFTextPart{Type: "text", Text: content})
				}
				if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
					for _, tc := range toolCalls {
						tcMap, ok := tc.(map[string]interface{})
						if !ok {
							continue
						}
						if function, ok := tcMap["function"].(map[string]interface{}); ok {
							id, _ := tcMap["id"].(string)
							name, _ := function["name"].(string)
							args, _ := function["arguments"].(string)
							var toolArgs map[string]interface{}
							json.Unmarshal([]byte(args), &toolArgs)
							result.Content = append(result.Content, cif.CIFToolCallPart{
								Type:          "tool_call",
								ToolCallID:    id,
								ToolName:      name,
								ToolArguments: toolArgs,
							})
						}
					}
				}
			}
		}
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		pt, _ := usage["prompt_tokens"].(float64)
		ct, _ := usage["completion_tokens"].(float64)
		result.Usage = &cif.CIFUsage{InputTokens: int(pt), OutputTokens: int(ct)}
	}

	return result
}

func openAIStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "stop":
		return cif.StopReasonEndTurn
	case "length":
		return cif.StopReasonMaxTokens
	case "tool_calls":
		return cif.StopReasonToolUse
	case "content_filter":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}

func antigravityStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "STOP":
		return cif.StopReasonEndTurn
	case "MAX_TOKENS":
		return cif.StopReasonMaxTokens
	case "FUNCTION_CALL":
		return cif.StopReasonToolUse
	case "SAFETY", "RECITATION":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}

func randomID() string {
	return fmt.Sprintf("%x%x", time.Now().UnixNano(), rand.Int63())
}
