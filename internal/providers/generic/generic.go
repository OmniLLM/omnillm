// Package generic provides a generic provider implementation for alibaba, antigravity, azure-openai, and google.
// It acts as a facade that delegates to provider-specific sub-packages while maintaining
// backward compatibility for callers that use *GenericProvider type assertions.
package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	alibabapkg "omnimodel/internal/providers/alibaba"
	antigravitypkg "omnimodel/internal/providers/antigravity"
	azurepkg "omnimodel/internal/providers/azure"
	googlepkg "omnimodel/internal/providers/google"
	kimipkg "omnimodel/internal/providers/kimi"
	"omnimodel/internal/providers/shared"
	"omnimodel/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// ─── Model catalogs (kept for white-box test access) ─────────────────────────

var providerModels = map[string][]types.Model{
	"antigravity": antigravitypkg.Models,
	"alibaba":     alibabapkg.Models,
	"azure-openai": azurepkg.DefaultModels,
	"kimi":        kimipkg.Models,
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
}

// GenericAdapter wraps GenericProvider for the ProviderAdapter interface.
type GenericAdapter struct {
	provider *GenericProvider
}

// ─── Constructor ──────────────────────────────────────────────────────────────

// NewGenericProvider creates a new GenericProvider for the given provider type.
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
	case "api-key":
		token, baseURL, name, config, err := alibabapkg.SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil

	case "oauth":
		return p.loadAlibabaTokenFromDB()

	default:
		return fmt.Errorf("alibaba: unsupported auth method: %s", options.Method)
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

// SaveAlibabaOAuthToken persists a completed OAuth token and returns the new canonical instance ID.
// This method is called by admin.go after a successful OAuth device code flow.
func (p *GenericProvider) SaveAlibabaOAuthToken(td *alibabapkg.TokenData) (newInstanceID string, err error) {
	newID, name, baseURL, err := alibabapkg.SaveOAuthToken(p.instanceID, td)
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

func (p *GenericProvider) setupAzureAuth(options *types.AuthOptions) error {
	token, endpoint, cfg, err := azurepkg.SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	if endpoint != "" {
		p.baseURL = endpoint
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

// ─── Config management ────────────────────────────────────────────────────────

func (p *GenericProvider) GetBaseURL() string {
	p.loadConfigFromDB()
	return p.baseURL
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
		p.baseURL = alibabapkg.NormalizeBaseURL(p.config)
	case "azure-openai":
		if endpoint, ok := firstString(config, "endpoint"); ok {
			p.baseURL = strings.TrimRight(endpoint, "/")
		}
	case "google":
		if p.baseURL == "" {
			p.baseURL = providerBaseURLs["google"]
		}
	case "kimi":
		p.baseURL = kimipkg.NormalizeBaseURL(p.config)
	}
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

// ensureFreshAlibabaToken refreshes the OAuth token if it is about to expire.
func (p *GenericProvider) ensureFreshAlibabaToken() string {
	refreshed := alibabapkg.EnsureFreshToken(p.instanceID, p.token)
	p.token = refreshed
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

		p.applyConfig(tokenData)
		p.loadConfigFromDB()

		if p.id == "alibaba" && p.token != "" {
			authType, _ := firstString(p.config, "auth_type", "authType")
			if authType == "oauth" && alibabapkg.IsJWT(p.token) {
				if email := alibabapkg.ExtractEmailFromJWT(p.token); email != "" {
					p.name = "Alibaba (" + email + ")"
					log.Info().Str("instanceID", p.instanceID).Str("extractedEmail", email).Str("newName", p.name).Msg("Updated Alibaba provider name from JWT email")
				} else {
					suffix := alibabapkg.ShortTokenSuffix(p.token)
					p.name = "Alibaba OAuth (" + suffix + ")"
					log.Warn().Str("instanceID", p.instanceID).Str("tokenSuffix", suffix).Msg("Failed to extract email from Alibaba JWT token; using token suffix")
				}
			} else if authType == "oauth" {
				suffix := alibabapkg.ShortTokenSuffix(p.token)
				p.name = "Alibaba OAuth (" + suffix + ")"
				log.Info().Str("instanceID", p.instanceID).Str("tokenSuffix", suffix).Msg("Alibaba OAuth provider has non-JWT token; using token suffix")
			} else {
				p.name = alibabapkg.APIKeyProviderName(p.config)
				log.Info().Str("instanceID", p.instanceID).Str("authType", authType).Str("newName", p.name).Msg("Updated Alibaba API key provider name")
			}
		}
	}

	if p.id == "google" && p.token != "" {
		p.baseURL = providerBaseURLs["google"]
		suffix := alibabapkg.ShortTokenSuffix(p.token)
		p.name = "Google Gemini (" + suffix + ")"
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
	return model
}

func (a *GenericAdapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.loadConfigFromDB()
	switch a.provider.id {
	case "alibaba":
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.executeOpenAI(a.alibabaURL(), a.provider.alibabaHeaders(false), request)
	case "azure-openai":
		if azurepkg.IsResponsesAPIModel(a.RemapModel(request.Model)) {
			return a.executeAzureResponses(request)
		}
		url, err := a.azureURL(a.RemapModel(request.Model))
		if err != nil {
			return nil, err
		}
		return a.executeOpenAI(url, a.azureHeaders(), request)
	case "google":
		return googlepkg.Execute(a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.executeOpenAI(a.kimiURL(), a.kimiHeaders(false), request)
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
		if !alibabapkg.IsChatCompletionsModel(a.RemapModel(request.Model)) {
			return nil, fmt.Errorf("alibaba model %s is realtime-only and is not supported by /v1/chat/completions", request.Model)
		}
		return a.streamOpenAI(a.alibabaURL(), a.provider.alibabaHeaders(true), request)
	case "azure-openai":
		if azurepkg.IsResponsesAPIModel(a.RemapModel(request.Model)) {
			return a.streamAzureResponses(request)
		}
		url, err := a.azureURL(a.RemapModel(request.Model))
		if err != nil {
			return nil, err
		}
		return a.streamOpenAI(url, a.azureHeaders(), request)
	case "google":
		return googlepkg.Stream(a.provider.token, a.provider.baseURL, request)
	case "kimi":
		return a.streamOpenAI(a.kimiURL(), a.kimiHeaders(true), request)
	case "antigravity":
		projectID, _ := firstString(a.provider.config, "project_id", "project")
		return antigravitypkg.Stream(a.provider.token, a.provider.baseURL, projectID, request)
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

func (a *GenericAdapter) azureURL(deployment string) (string, error) {
	apiVersion := ""
	if v, ok := firstString(a.provider.config, "api_version", "apiVersion"); ok {
		apiVersion = v
	}
	return azurepkg.ChatURL(a.provider.baseURL, deployment, apiVersion)
}

func (a *GenericAdapter) azureHeaders() map[string]string {
	return azurepkg.Headers(a.provider.token)
}

func (a *GenericAdapter) azureResponsesURL() (string, error) {
	return azurepkg.ResponsesURL(a.provider.baseURL)
}

// ─── Azure Responses API ──────────────────────────────────────────────────────

func (a *GenericAdapter) executeAzureResponses(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.ExecuteResponses(url, a.provider.token, request, model)
}

func (a *GenericAdapter) streamAzureResponses(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	url, err := a.azureResponsesURL()
	if err != nil {
		return nil, err
	}
	model := a.RemapModel(request.Model)
	return azurepkg.StreamResponses(url, a.provider.token, request, model)
}

// ─── OpenAI-compatible execution ─────────────────────────────────────────────

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
		authType, _ := firstString(a.provider.config, "auth_type", "authType")
		return alibabapkg.BuildOpenAIPayload(model, messages, request, authType == "oauth")
	}

	if a.provider.id == "azure-openai" {
		return azurepkg.BuildOpenAIPayload(model, messages, request)
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
					"parameters": tool.ParametersSchema,
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

func (a *GenericAdapter) executeOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	if a.provider.id == "azure-openai" {
		return azurepkg.ExecuteOpenAI(url, headers, request)
	}
	return a.executeOpenAIGeneric(url, headers, request)
}

func (a *GenericAdapter) executeOpenAIGeneric(url string, headers map[string]string, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false
	return executeOpenAIWithPayload(url, headers, payload, request.Model)
}

func (a *GenericAdapter) streamOpenAI(url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.provider.id == "azure-openai" {
		return azurepkg.StreamOpenAI(url, headers, request)
	}
	return a.streamOpenAIGeneric(url, headers, request)
}

func (a *GenericAdapter) streamOpenAIGeneric(url string, headers map[string]string, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	if a.provider.id == "alibaba" {
		payload["stream_options"] = map[string]interface{}{"include_usage": true}
	}
	return streamOpenAIWithPayload(url, headers, payload)
}

// collectStream runs ExecuteStream and assembles a CanonicalResponse.
func (a *GenericAdapter) collectStream(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := a.ExecuteStream(request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}

// collectStreamFromChan assembles a CanonicalResponse from a stream channel.
func (a *GenericAdapter) collectStreamFromChan(ch <-chan cif.CIFStreamEvent) (*cif.CanonicalResponse, error) {
	return shared.CollectStream(ch)
}

// ─── Google payload builder (kept for white-box test access) ──────────────────

func (a *GenericAdapter) buildGooglePayload(request *cif.CanonicalRequest) map[string]interface{} {
	return googlepkg.BuildPayload(a.RemapModel(request.Model), request)
}

func (a *GenericAdapter) googleURL(model string) string {
	return googlepkg.StreamURL(a.provider.baseURL, model)
}

func (a *GenericAdapter) executeGoogle(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return googlepkg.Execute(a.provider.token, a.provider.baseURL, request)
}

func (a *GenericAdapter) streamGoogle(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return googlepkg.Stream(a.provider.token, a.provider.baseURL, request)
}

// ─── Antigravity stream (kept for white-box test access) ─────────────────────

func (a *GenericAdapter) streamAntigravity(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	projectID, _ := firstString(a.provider.config, "project_id", "project")
	return antigravitypkg.Stream(a.provider.token, a.provider.baseURL, projectID, request)
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

// cifMessagesToOpenAI converts CIF messages to OpenAI format.
func cifMessagesToOpenAI(messages []cif.CIFMessage) []map[string]interface{} {
	return shared.CIFMessagesToOpenAI(messages)
}

// openAIRespToCIF converts an OpenAI response to CIF format.
func openAIRespToCIF(resp map[string]interface{}) *cif.CanonicalResponse {
	return shared.OpenAIRespToCIF(resp)
}

// openAIStopReason converts an OpenAI finish_reason to a CIF stop reason.
func openAIStopReason(reason string) cif.CIFStopReason {
	return shared.OpenAIStopReason(reason)
}

// randomID generates a random ID.
func randomID() string {
	return shared.RandomID()
}

// googleStopReason converts a Google finish reason to CIF stop reason.
func googleStopReason(reason string) cif.CIFStopReason {
	return googlepkg.StopReason(reason)
}

// antigravityStopReason converts an Antigravity finish reason to CIF stop reason.
func antigravityStopReason(reason string) cif.CIFStopReason {
	return antigravitypkg.StopReason(reason)
}

// parseOpenAISSE parses an OpenAI SSE stream.
func parseOpenAISSE(body interface{ Read([]byte) (int, error); Close() error }, eventCh chan cif.CIFStreamEvent) {
	shared.ParseOpenAISSE(body, eventCh)
}

// parseGoogleGeminiSSE parses a Google Gemini SSE stream.
func parseGoogleGeminiSSE(body interface{ Read([]byte) (int, error); Close() error }, eventCh chan cif.CIFStreamEvent) {
	googlepkg.ParseGeminiSSE(body, eventCh)
}

// parseAntigravitySSE parses an Antigravity SSE stream.
func parseAntigravitySSE(body interface{ Read([]byte) (int, error); Close() error }, eventCh chan cif.CIFStreamEvent) {
	antigravitypkg.ParseAntigravitySSE(body, eventCh)
}

// ─── Alibaba model helpers (kept for white-box test access) ───────────────────

func (p *GenericProvider) getAlibabaModels() (*types.ModelsResponse, error) {
	return alibabapkg.GetModels(p.instanceID, p.token, p.baseURL, p.config)
}

func (p *GenericProvider) getAlibabaModelsHardcoded() *types.ModelsResponse {
	return alibabapkg.GetModelsHardcoded(p.instanceID, p.config)
}

func (p *GenericProvider) fetchAlibabaModelsFromAPI() (*types.ModelsResponse, error) {
	return alibabapkg.FetchModelsFromAPI(p.instanceID, p.token, p.baseURL)
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

func ensureAlibabaBaseURL(raw string, forOAuth bool) string {
	return alibabapkg.EnsureBaseURL(raw, forOAuth)
}

func isJWT(token string) bool {
	return alibabapkg.IsJWT(token)
}

func shortTokenSuffix(token string) string {
	return alibabapkg.ShortTokenSuffix(token)
}

func extractEmailFromJWT(token string) string {
	return alibabapkg.ExtractEmailFromJWT(token)
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

// ensureAlibabaOAuthSystemMessage is kept as a private shim for the existing tests.
func ensureAlibabaOAuthSystemMessage(messages []map[string]interface{}) []map[string]interface{} {
	return alibabapkg.EnsureOAuthSystemMessage(messages)
}

// alibabaHeaders wraps the alibaba package header builder.
func (p *GenericProvider) alibabaHeaders(stream bool) map[string]string {
	token := p.ensureFreshAlibabaToken()
	return alibabapkg.Headers(token, stream, p.config)
}

// ─── Azure helpers (kept for white-box test access) ───────────────────────────

func isAzureResponsesApiModel(model string) bool {
	return azurepkg.IsResponsesAPIModel(model)
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

// ─── fetchGoogleModels (kept for white-box test access) ───────────────────────

func (p *GenericProvider) fetchGoogleModels() (*types.ModelsResponse, error) {
	return googlepkg.FetchModels(p.instanceID, p.token, p.baseURL)
}

// ─── OpenAI payload helper (kept for white-box test access) ───────────────────

// openAIMessageContentText is kept for the Alibaba OAuth system message merging tests.
func openAIMessageContentText(content interface{}) string {
	// Delegate to alibaba package's internal logic
	return openAIContentTextHelper(content)
}

func openAIContentTextHelper(content interface{}) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]interface{}:
		if text, ok := value["text"].(string); ok {
			return strings.TrimSpace(text)
		}
		if nested, ok := value["content"]; ok {
			return openAIContentTextHelper(nested)
		}
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			text := openAIContentTextHelper(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// convertCanonicalToolChoiceToOpenAI is kept for white-box tests.
func convertCanonicalToolChoiceToOpenAI(toolChoice interface{}) interface{} {
	return shared.ConvertCanonicalToolChoiceToOpenAI(toolChoice)
}

// ─── HTTP execution helpers ───────────────────────────────────────────────────

// executeOpenAIWithPayload performs a non-streaming OpenAI-compatible HTTP request.
func executeOpenAIWithPayload(url string, headers map[string]string, payload map[string]interface{}, originalModel string) (*cif.CanonicalResponse, error) {
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

	return shared.OpenAIRespToCIF(openaiResp), nil
}

// streamOpenAIWithPayload performs a streaming OpenAI-compatible HTTP request.
func streamOpenAIWithPayload(url string, headers map[string]string, payload map[string]interface{}) (<-chan cif.CIFStreamEvent, error) {
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
