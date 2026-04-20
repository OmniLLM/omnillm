// Package alibaba provides Alibaba DashScope / Qwen provider implementation.
//
// Alibaba DashScope speaks the OpenAI-compatible chat completions protocol.
// This package is a thin configuration layer on top of
// internal/providers/openaicompat — providing DashScope-specific:
//   - Base URL constants and normalization
//   - API-key authentication and token persistence
//   - Model catalog and live model discovery
//   - Qwen3 reasoning (enable_thinking) config
//   - Provider name derivation
//
// The Provider struct implements types.Provider.
// The Adapter struct implements types.ProviderAdapter.
package alibaba

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// ─── Constants ────────────────────────────────────────────────────────────────

const UserAgent = "OmniLLM/1.0"

// API mode constant for OpenAI-compatible DashScope endpoints.
const AlibabaAPIModeOpenAICompatible = "openai-compatible"


// ─── Model catalog ────────────────────────────────────────────────────────────

var Models = []types.Model{
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
}

// ─── Provider ─────────────────────────────────────────────────────────────────

// Provider implements types.Provider for Alibaba DashScope.
type Provider struct {
	instanceID   string
	name         string
	token        string
	baseURL      string
	config       map[string]interface{}
	configLoaded bool
}

// NewProvider creates a new Alibaba Provider.
func NewProvider(instanceID, name string) *Provider {
	return &Provider{
		instanceID: instanceID,
		name:       name,
		baseURL:    BaseURLGlobal,
	}
}

// ─── types.Provider identity ─────────────────────────────────────────────────

func (p *Provider) GetID() string         { return "alibaba" }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }

// SetInstanceID updates the in-memory instance ID (called by registry rename).
func (p *Provider) SetInstanceID(id string) { p.instanceID = id }

// ─── types.Provider auth ─────────────────────────────────────────────────────

// SetupAuth handles API-key authentication and persists credentials.
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	token, baseURL, name, config, err := SetupAPIKeyAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	p.config = config
	return nil
}

func (p *Provider) GetToken() string  { return p.token }
func (p *Provider) RefreshToken() error { return nil } // API keys don't expire

// ─── types.Provider config ────────────────────────────────────────────────────

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	return Headers(p.token, false, p.config)
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
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("alibaba: failed to parse config")
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
	p.baseURL = NormalizeBaseURL(p.config)
}

// ─── types.Provider models ────────────────────────────────────────────────────

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	return GetModels(p.instanceID, p.token, p.baseURL, p.config)
}

// ─── types.Provider legacy stubs ─────────────────────────────────────────────

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("alibaba: use the adapter for chat completions")
}
func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("alibaba: embeddings not implemented")
}
func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

// ─── LoadFromDB ───────────────────────────────────────────────────────────────

// LoadFromDB restores persisted credentials and config from the database.
func (p *Provider) LoadFromDB() error {
	token, baseURL, config, err := LoadTokenFromDB(p.instanceID)
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
	p.name = APIKeyProviderName(p.config)
	p.configLoaded = true
	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Msg("Alibaba: loaded from DB")
	return nil
}

// ─── Adapter ──────────────────────────────────────────────────────────────────

// Adapter implements types.ProviderAdapter using openaicompat for HTTP.
type Adapter struct {
	provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return RemapModel(model) }

func (a *Adapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	cr, err := a.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	return openaicompat.Execute(ChatURL(a.provider.baseURL), Headers(a.provider.token, false, a.provider.config), cr)
}

func (a *Adapter) ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	cr, err := a.buildRequest(request, true)
	if err != nil {
		return nil, err
	}
	return openaicompat.Stream(ChatURL(a.provider.baseURL), Headers(a.provider.token, true, a.provider.config), cr)
}

// buildRequest converts a CIF request into an openaicompat.ChatRequest with
// DashScope-specific extras (enable_thinking, stream_options).
func (a *Adapter) buildRequest(request *cif.CanonicalRequest, stream bool) (*openaicompat.ChatRequest, error) {
	model := a.RemapModel(request.Model)

	// LiteLLM-style defaults for Alibaba.
	defTemp := 0.55
	defTopP := 1.0

	extras := map[string]interface{}{}

	// enable_thinking for Qwen3 reasoning models; suppressed when tools present
	// (DashScope rejects that combination).
	if IsReasoningModel(model) && len(request.Tools) == 0 {
		extras["enable_thinking"] = true
	}

	cfg := openaicompat.Config{
		DefaultTemperature:   &defTemp,
		DefaultTopP:          &defTopP,
		IncludeUsageInStream: stream,
		Extras:               extras,
	}
	return openaicompat.BuildChatRequest(model, request, stream, cfg)
}

// ─── Auth helpers ─────────────────────────────────────────────────────────────

// SetupAPIKeyAuth saves credentials and returns resolved values.
func SetupAPIKeyAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, config map[string]interface{}, err error) {
	if options.APIKey == "" {
		return "", "", "", nil, fmt.Errorf("alibaba: API key is required")
	}

	region := strings.TrimSpace(options.Region)
	if region == "" {
		region = "global"
	}
	plan := NormalizeAPIPlan(options.Plan)

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, "alibaba", tokenData); err != nil {
		return "", "", "", nil, fmt.Errorf("alibaba: failed to save token: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type": "api-key",
		"region":    region,
		"plan":      plan,
	}
	if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
		cfg["base_url"] = endpoint
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(instanceID, cfg); err != nil {
		return "", "", "", nil, fmt.Errorf("alibaba: failed to save config: %w", err)
	}

	resolvedURL := NormalizeBaseURL(cfg)
	resolvedName := APIKeyProviderName(cfg)

	log.Info().Str("provider", instanceID).Str("region", region).Str("plan", plan).
		Msg("Alibaba authenticated via API key")

	return options.APIKey, resolvedURL, resolvedName, cfg, nil
}

// LoadTokenFromDB reads the persisted Alibaba token from the database.
func LoadTokenFromDB(instanceID string) (token, baseURL string, config map[string]interface{}, err error) {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to load token: %w", err)
	}
	if record == nil {
		return "", "", nil, nil
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to parse token data: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type": td.AuthType,
		"base_url":  td.BaseURL,
	}
	return td.AccessToken, NormalizeBaseURL(cfg), cfg, nil
}

// ─── Headers ─────────────────────────────────────────────────────────────────

// Headers returns HTTP headers for DashScope requests.
func Headers(token string, stream bool, config map[string]interface{}) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	if stream {
		h["Accept"] = "text/event-stream"
	}
	return h
}

// ChatURL returns the chat completions endpoint for the given base URL.
func ChatURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURLGlobal
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

// ─── URL helpers ──────────────────────────────────────────────────────────────

// NormalizeBaseURL derives the base URL from a provider config map.
func NormalizeBaseURL(config map[string]interface{}) string {
	if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok {
		return EnsureBaseURL(baseURL)
	}
	plan, _ := shared.FirstString(config, "plan")
	region, _ := shared.FirstString(config, "region")
	return DefaultAPIBaseURL(plan, region)
}

// EnsureBaseURL normalizes a base URL to have https scheme and /v1 suffix.
func EnsureBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return BaseURLGlobal
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

// NormalizeAPIPlan normalizes a plan string to "coding-plan" or "standard".
func NormalizeAPIPlan(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "coding", "coding-plan", "coding_plan":
		return "coding-plan"
	default:
		return "standard"
	}
}

// DefaultAPIBaseURL returns the default DashScope base URL for plan+region.
func DefaultAPIBaseURL(plan, region string) string {
	switch NormalizeAPIPlan(plan) {
	case "coding-plan":
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return CodingPlanBaseURLChina
		}
		return CodingPlanBaseURLGlobal
	default:
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return BaseURLChina
		}
		return BaseURLGlobal
	}
}

// APIKeyProviderName returns the display name for this provider.
func APIKeyProviderName(config map[string]interface{}) string {
	region, _ := shared.FirstString(config, "region")
	if region == "" {
		region = "global"
	}
	planRaw, _ := config["plan"].(string)
	switch NormalizeAPIPlan(planRaw) {
	case "coding-plan":
		return "Alibaba Coding Plan (" + region + ")"
	default:
		return "Alibaba DashScope Standard (" + region + ")"
	}
}

// ─── Model helpers ────────────────────────────────────────────────────────────

// RemapModel is a no-op for Alibaba — model IDs are used as-is.
func RemapModel(modelID string) string { return strings.TrimSpace(modelID) }

// IsChatCompletionsModel returns true if the model is not realtime-only.
func IsChatCompletionsModel(modelID string) bool {
	return !strings.Contains(strings.ToLower(modelID), "realtime")
}

// IsReasoningModel returns true for Qwen3/QwQ models that support enable_thinking.
func IsReasoningModel(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.Contains(lower, "qwen3") ||
		strings.Contains(lower, "qwq") ||
		strings.Contains(lower, "qwen-plus") ||
		strings.Contains(lower, "qwen3.5") ||
		strings.Contains(lower, "qwen3.6")
}

// ModelMetadata returns hardcoded metadata for a known model ID.
func ModelMetadata(modelID string) (types.Model, bool) {
	for _, m := range Models {
		if m.ID == modelID {
			return m, true
		}
	}
	return types.Model{}, false
}

// GetModels returns the available models for this Alibaba instance.
func GetModels(instanceID, token, baseURL string, config map[string]interface{}) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated")
	}
	resp, err := FetchModelsFromAPI(instanceID, token, baseURL, config)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}
	log.Warn().Err(err).Str("provider", instanceID).Msg("alibaba: falling back to hardcoded model list")
	return GetModelsHardcoded(instanceID), nil
}

// GetModelsHardcoded returns the hardcoded model catalog.
func GetModelsHardcoded(instanceID string) *types.ModelsResponse {
	result := make([]types.Model, len(Models))
	for i, m := range Models {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// FetchModelsFromAPI fetches available models from the DashScope API.
func FetchModelsFromAPI(instanceID, token, baseURL string, _ map[string]interface{}) (*types.ModelsResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to create models request: %w", err)
	}
	for k, v := range Headers(token, false, nil) {
		req.Header.Set(k, v)
	}

	resp, err := alibabaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alibaba: models fetch failed (%d)", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to read models response: %w", err)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("alibaba: failed to decode models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" || !IsChatCompletionsModel(item.ID) {
			continue
		}
		m := types.Model{ID: item.ID, Name: item.ID, Provider: instanceID}
		if meta, ok := ModelMetadata(item.ID); ok {
			if meta.Name != "" {
				m.Name = meta.Name
			}
			m.Description = meta.Description
			m.Capabilities = meta.Capabilities
			m.MaxTokens = meta.MaxTokens
		}
		models = append(models, m)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}
