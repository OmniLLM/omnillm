// Package generic provides a generic provider implementation for alibaba, antigravity, azure-openai, and google.
// It acts as a facade that delegates to provider-specific sub-packages while maintaining
// backward compatibility for callers that use *GenericProvider type assertions.
package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"sync"
	"time"

	alibabapkg "omnillm/internal/providers/alibaba"
	antigravitypkg "omnillm/internal/providers/antigravity"
	azurepkg "omnillm/internal/providers/azure"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"

	"github.com/rs/zerolog/log"
)

// ─── Model catalogs (kept for white-box test access) ─────────────────────────

var providerModels = map[string][]types.Model{
	"antigravity":  antigravitypkg.Models,
	"alibaba":      alibabapkg.Models,
	"azure-openai": azurepkg.DefaultModels,
	"kimi":         kimipkg.Models,
	"google": {
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-preview-05-20", Name: "Gemini 2.5 Flash Preview 05-20", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-pro-preview-06-05", Name: "Gemini 2.5 Pro Preview 06-05", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash Lite", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash-8b", Name: "Gemini 1.5 Flash-8B", MaxTokens: 8192, Provider: "google"},
	},
}

var providerBaseURLs = map[string]string{
	"antigravity":  "https://daily-cloudcode-pa.googleapis.com",
	"alibaba":      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	"azure-openai": "",
	"kimi":         "https://api.moonshot.cn/v1",
	"google":       "https://generativelanguage.googleapis.com",
}

const alibabaUserAgent = alibabapkg.UserAgent

// ─── Types ────────────────────────────────────────────────────────────────────

// GenericProvider is a minimal provider implementation for non-copilot providers.
// The struct fields are kept identical to the original to preserve backward compatibility
// for callers that use *GenericProvider type assertions (e.g., admin.go).
type GenericProvider struct {
	id         string
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	// configOnce ensures config is loaded from the database exactly once,
	// even under concurrent requests.  Replaces the racy configLoaded bool.
	configOnce sync.Once
}

// GenericAdapter wraps GenericProvider for the ProviderAdapter interface.
type GenericAdapter struct {
	provider *GenericProvider
}

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

// SetInstanceID updates the provider's in-memory instance ID after a registry rename.
func (p *GenericProvider) SetInstanceID(newID string) { p.instanceID = newID }

// ─── Auth ─────────────────────────────────────────────────────────────────────

// SetupAuth authenticates the provider using the given options.
func (p *GenericProvider) SetupAuth(options *types.AuthOptions) error {
	switch p.id {
	case "alibaba":
		return p.setupAlibabaAuth(options)
	case "azure-openai":
		return p.setupAzureAuth(options)
	case "google":
		return p.setupGoogleAuth(options)
	case "kimi":
		return p.setupKimiAuth(options)
	default:
		return fmt.Errorf("use the admin UI to authenticate %s", p.id)
	}
}

func (p *GenericProvider) setupAlibabaAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "api-key", "":
		token, baseURL, name, config, err := alibabapkg.SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil

	default:
		return fmt.Errorf("alibaba: unsupported auth method: %s (only api-key is supported)", options.Method)
	}
}

// loadAlibabaTokenFromDB reads the persisted Alibaba token and applies it to the provider.
func (p *GenericProvider) loadAlibabaTokenFromDB() error {
	token, baseURL, config, err := alibabapkg.LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

// SaveAlibabaOAuthToken is a stub that returns an error since OAuth is no longer supported
func (p *GenericProvider) SaveAlibabaOAuthToken(td *alibabapkg.TokenData) (newInstanceID string, err error) {
	return "", fmt.Errorf("oauth authentication is no longer supported for Alibaba DashScope - please use API key authentication")
}

func (p *GenericProvider) setupAzureAuth(options *types.AuthOptions) error {
	token, endpoint, cfg, err := azurepkg.SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	if endpoint != "" {
		p.baseURL = endpoint
		if name := deriveAzureName(endpoint); name != "" {
			p.name = name
		}
	}
	if cfg != nil {
		p.applyConfig(cfg)
	}
	return nil
}

func (p *GenericProvider) setupGoogleAuth(options *types.AuthOptions) error {
	token, baseURL, name, err := googlepkg.SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	return nil
}

func (p *GenericProvider) setupKimiAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "", "api-key":
		token, baseURL, name, config, err := kimipkg.SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil

	case "oauth":
		return p.loadKimiTokenFromDB()

	default:
		return fmt.Errorf("kimi: unsupported auth method: %s", options.Method)
	}
}

// loadKimiTokenFromDB reads the persisted Kimi token and applies it to the provider.
func (p *GenericProvider) loadKimiTokenFromDB() error {
	token, baseURL, config, err := kimipkg.LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

// SaveKimiOAuthToken persists a completed OAuth token and returns the new canonical instance ID.
func (p *GenericProvider) SaveKimiOAuthToken(td *kimipkg.TokenData) (newInstanceID string, err error) {
	newID, name, baseURL, err := kimipkg.SaveOAuthToken(p.instanceID, td)
	if err != nil {
		return "", err
	}
	p.token = td.AccessToken
	p.name = name
	p.baseURL = baseURL
	p.instanceID = newID
	p.applyConfig(map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	})
	return newID, nil
}

// ─── Token management ─────────────────────────────────────────────────────────

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

// ─── Config management ────────────────────────────────────────────────────────

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

// ─── LoadFromDB ───────────────────────────────────────────────────────────────

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
	}

	if p.id == "google" && p.token != "" {
		p.baseURL = providerBaseURLs["google"]
		p.name = "Google Gemini"
	}

	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Msg("Loaded generic provider token")
	return nil
}

// ─── GenericAdapter ───────────────────────────────────────────────────────────

func (a *GenericAdapter) GetProvider() types.Provider { return a.provider }

func (a *GenericAdapter) RemapModel(model string) string {
	if a.provider.id == "antigravity" {
		return antigravitypkg.RemapModel(model)
	}
	if a.provider.id == "alibaba" {
		return alibabapkg.RemapModel(model)
	}
	if a.provider.id == "azure-openai" {
		return azurepkg.RemapModel(a.provider.config, strings.TrimSpace(model))
	}
	return strings.TrimSpace(model)
}

func (a *GenericAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.executeOpenAI(ctx, a.alibabaURL(), a.provider.alibabaHeaders(false), request)
	case "azure-openai":
		return a.executeAzureResponses(ctx, request)
	case "google":
		return googlepkg.Execute(ctx, a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.executeOpenAI(ctx, a.kimiURL(), a.kimiHeaders(false), request)
	case "antigravity":
		return a.collectStream(ctx, request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

func (a *GenericAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.streamOpenAI(ctx, a.alibabaURL(), a.provider.alibabaHeaders(true), request)
	case "azure-openai":
		return a.streamAzureResponses(ctx, request)
	case "google":
		return googlepkg.Stream(ctx, a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.streamOpenAI(ctx, a.kimiURL(), a.kimiHeaders(true), request)
	case "antigravity":
		projectID, _ := firstString(a.provider.config, "project_id", "project")
		return antigravitypkg.Stream(ctx, a.provider.token, a.provider.baseURL, projectID, request)
	default:
		return nil, fmt.Errorf("provider %s not yet implemented", a.provider.id)
	}
}

// ─── URL / header helpers ─────────────────────────────────────────────────────

func (a *GenericAdapter) alibabaURL() string {
	return alibabapkg.ChatURL(a.provider.baseURL)
}

func (a *GenericAdapter) alibabaHeaders(stream bool) map[string]string {
	token := a.provider.ensureFreshAlibabaToken()
	return alibabapkg.Headers(token, stream, a.provider.config)
}

func (a *GenericAdapter) kimiURL() string {
	return kimipkg.ChatURL(a.provider.baseURL)
}

func (a *GenericAdapter) kimiHeaders(stream bool) map[string]string {
	return kimipkg.Headers(a.provider.token, stream, a.provider.config)
}

func (a *GenericAdapter) azureResponsesURL() (string, error) {
	return azurepkg.ResponsesURL(a.provider.baseURL)
}

// ─── Azure Responses API ──────────────────────────────────────────────────────

func (a *GenericAdapter) executeAzureResponses(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.ExecuteResponses(ctx, url, a.provider.token, request, model)
}

func (a *GenericAdapter) streamAzureResponses(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.StreamResponses(ctx, url, a.provider.token, request, model)
}

func (a *GenericAdapter) buildOpenAIPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}

	if a.provider.id == "alibaba" {
		// Alibaba uses its own adapter; this path is a fallback for direct
		// GenericProvider usage with API-key auth.
		payload := map[string]interface{}{
			"model":    model,
			"messages": messages,
		}
		defTemp := 0.55
		defTopP := 1.0
		if request.Temperature != nil {
			payload["temperature"] = *request.Temperature
		} else {
			payload["temperature"] = defTemp
		}
		if request.TopP != nil {
			payload["top_p"] = *request.TopP
		} else {
			payload["top_p"] = defTopP
		}
		if request.MaxTokens != nil {
			payload["max_tokens"] = *request.MaxTokens
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
						"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
					},
				}
				if tool.Description != nil {
					t["function"].(map[string]interface{})["description"] = *tool.Description
				}
				tools = append(tools, t)
			}
			payload["tools"] = tools
		}
		if request.ToolChoice != nil {
			if tc := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); tc != nil {
				payload["tool_choice"] = tc
			}
		}
		if alibabapkg.IsReasoningModel(model) && len(request.Tools) == 0 {
			payload["enable_thinking"] = true
		}
		return payload
	}

	// Default OpenAI-compatible payload
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
		payload["max_tokens"] = *request.MaxTokens
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
					"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
				},
			}
			if tool.Description != nil {
				t["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
	}
	if request.ToolChoice != nil {
		if toolChoice := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}
	return payload
}

func (a *GenericAdapter) executeOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false
	return executeOpenAIWithPayload(ctx, url, headers, payload, request.Model)
}

func (a *GenericAdapter) streamOpenAI(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.streamOpenAIGeneric(ctx, url, headers, request)
}

func (a *GenericAdapter) streamOpenAIGeneric(ctx context.Context, url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	if a.provider.id == "alibaba" {
		payload["stream_options"] = map[string]interface{}{"include_usage": true}
	}
	return streamOpenAIWithPayload(ctx, url, headers, payload)
}

// collectStream runs ExecuteStream and assembles a CanonicalResponse.
func (a *GenericAdapter) collectStream(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := a.ExecuteStream(ctx, request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}

// ─── Google payload builder (kept for white-box test access) ──────────────────

func (a *GenericAdapter) buildGooglePayload(request *cif.CanonicalRequest) map[string]interface{} {
	return googlepkg.BuildPayload(a.RemapModel(request.Model), request)
}

func (a *GenericAdapter) googleURL(model string) string {
	return googlepkg.StreamURL(a.provider.baseURL, model)
}

func (a *GenericAdapter) executeGoogle(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return googlepkg.Execute(ctx, a.provider.token, a.provider.baseURL, request)
}

func (a *GenericAdapter) streamGoogle(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return googlepkg.Stream(ctx, a.provider.token, a.provider.baseURL, request)
}

// ─── Antigravity stream (kept for white-box test access) ─────────────────────

func (a *GenericAdapter) streamAntigravity(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	projectID, _ := firstString(a.provider.config, "project_id", "project")
	return antigravitypkg.Stream(ctx, a.provider.token, a.provider.baseURL, projectID, request)
}

// ─── Shared helpers (kept for white-box test access) ─────────────────────────

// normalizeToolArguments converts arbitrary raw tool args to map[string]interface{}.
func normalizeToolArguments(raw interface{}) map[string]interface{} {
	return shared.NormalizeToolArguments(raw)
}

// cifMessagesToGemini converts CIF messages to the Gemini format.
func cifMessagesToGemini(messages []cif.CIFMessage) []map[string]interface{} {
	return shared.CIFMessagesToGemini(messages)
}

// sanitizeGeminiSchema removes blocked fields from Gemini JSON Schema objects.
func sanitizeGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	return shared.SanitizeGeminiSchema(schema)
}

// googleStopReason converts a Google finish reason to CIF stop reason.
func googleStopReason(reason string) cif.CIFStopReason {
	return googlepkg.StopReason(reason)
}

// antigravityStopReason converts an Antigravity finish reason to CIF stop reason.
func antigravityStopReason(reason string) cif.CIFStopReason {
	return antigravitypkg.StopReason(reason)
}

// parseGoogleGeminiSSE parses a Google Gemini SSE stream.
func parseGoogleGeminiSSE(body interface {
	Read([]byte) (int, error)
	Close() error
}, eventCh chan cif.CIFStreamEvent,
) {
	googlepkg.ParseGeminiSSE(body, eventCh)
}

// parseAntigravitySSE parses an Antigravity SSE stream.
func parseAntigravitySSE(body interface {
	Read([]byte) (int, error)
	Close() error
}, eventCh chan cif.CIFStreamEvent,
) {
	antigravitypkg.ParseAntigravitySSE(body, eventCh)
}

// ─── Alibaba model helpers (kept for white-box test access) ───────────────────

func (p *GenericProvider) getAlibabaModels() (*types.ModelsResponse, error) {
	return alibabapkg.GetModels(p.instanceID, p.token, p.baseURL, p.config)
}

func (p *GenericProvider) getAlibabaModelsHardcoded() *types.ModelsResponse {
	return alibabapkg.GetModelsHardcoded(p.instanceID)
}

func (p *GenericProvider) fetchAlibabaModelsFromAPI() (*types.ModelsResponse, error) {
	return alibabapkg.FetchModelsFromAPI(p.instanceID, p.token, p.baseURL, p.config)
}

func isAlibabaChatCompletionsModel(modelID string) bool {
	return alibabapkg.IsChatCompletionsModel(modelID)
}

func alibabaModelMetadata(modelID string) (types.Model, bool) {
	return alibabapkg.ModelMetadata(modelID)
}

func normalizeAlibabaBaseURL(config map[string]interface{}) string {
	return alibabapkg.NormalizeBaseURL(config)
}

func normalizeAlibabaAPIPlan(plan string) string {
	return alibabapkg.NormalizeAPIPlan(plan)
}

func defaultAlibabaAPIBaseURL(plan, region string) string {
	return alibabapkg.DefaultAPIBaseURL(plan, region)
}

func alibabaAPIKeyProviderName(config map[string]interface{}) string {
	return alibabapkg.APIKeyProviderName(config)
}

// deriveAzureName extracts a human-friendly resource name from an Azure OpenAI endpoint URL.
// e.g. "https://my-resource.openai.azure.com" → "my-resource"
// Falls back to empty string if the endpoint is not a recognizable Azure URL.
func deriveAzureName(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	// Standard Azure OpenAI hostname: <resource-name>.openai.azure.com
	if strings.HasSuffix(host, ".openai.azure.com") {
		resource := strings.TrimSuffix(host, ".openai.azure.com")
		if resource != "" {
			return resource
		}
	}
	// Cognitive services endpoint: <resource-name>.cognitiveservices.azure.com
	if strings.HasSuffix(host, ".cognitiveservices.azure.com") {
		resource := strings.TrimSuffix(host, ".cognitiveservices.azure.com")
		if resource != "" {
			return resource
		}
	}
	return ""
}

func ensureAlibabaBaseURL(raw string) string {
	return alibabapkg.EnsureBaseURL(raw)
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

// alibabaHeaders wraps the alibaba package header builder.
func (p *GenericProvider) alibabaHeaders(stream bool) map[string]string {
	token := p.ensureFreshAlibabaToken()
	return alibabapkg.Headers(token, stream, p.config)
}

// ─── Azure helpers ────────────────────────────────────────────────────────────

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

// ─── fetchGoogleModels (kept for white-box test access) ───────────────────────

func (p *GenericProvider) fetchGoogleModels() (*types.ModelsResponse, error) {
	return googlepkg.FetchModels(p.instanceID, p.token, p.baseURL)
}

// ─── HTTP execution helpers ───────────────────────────────────────────────────

var (
	genericHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	genericStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
)

// executeOpenAIWithPayload performs a non-streaming OpenAI-compatible HTTP request.
func executeOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}, originalModel string) (*cif.CanonicalResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := genericHTTPClient
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

	return shared.OpenAIRespToCIF(openaiResp), nil
}

// streamOpenAIWithPayload performs a streaming OpenAI-compatible HTTP request.
func streamOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (<-chan cif.CIFStreamEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := genericStreamClient
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
	go shared.ParseOpenAISSE(resp.Body, eventCh)
	return eventCh, nil
}

// ─── Utility ──────────────────────────────────────────────────────────────────

func firstString(values map[string]interface{}, keys ...string) (string, bool) {
	return shared.FirstString(values, keys...)
}

func stringValueOrEmpty(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
