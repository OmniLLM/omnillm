// Package alibaba provides Alibaba DashScope / Qwen provider implementation.
// This package contains auth, header, model, and adapter logic specific to Alibaba.
// The OAuth 2.0 device-code flow is implemented in auth.go (sibling file).
package alibaba

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"omnimodel/internal/cif"
	"omnimodel/internal/database"
	"omnimodel/internal/providers/shared"
	"omnimodel/internal/providers/types"

	"github.com/rs/zerolog/log"
)

const UserAgent = "QwenCode/0.13.2 (darwin; arm64)"

// Model catalog for Alibaba DashScope.
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

// OAuthSupportedModels lists model IDs available to OAuth-authenticated users.
var OAuthSupportedModels = map[string]bool{
	"qwen3-coder-flash": true,
	"qwen3-coder-next":  true,
	"qwen3-coder-plus":  true,
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

// SetupAPIKeyAuth handles the api-key auth method for Alibaba.
// It saves the token and config to the database and updates the in-memory fields.
func SetupAPIKeyAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, config map[string]interface{}, err error) {
	if options.APIKey == "" {
		return "", "", "", nil, fmt.Errorf("alibaba: API key is required")
	}

	// Anthropic-compatible API mode — store api_format, skip plan/region.
	if strings.EqualFold(strings.TrimSpace(options.APIFormat), "anthropic") {
		tokenStore := database.NewTokenStore()
		tokenData := map[string]interface{}{"access_token": options.APIKey}
		if err := tokenStore.Save(instanceID, "alibaba", tokenData); err != nil {
			return "", "", "", nil, fmt.Errorf("failed to save alibaba token: %w", err)
		}
		cfg := map[string]interface{}{
			"auth_type":  "api-key",
			"api_format": "anthropic",
		}
		if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
			cfg["base_url"] = endpoint
		}
		configStore := database.NewProviderConfigStore()
		if err := configStore.Save(instanceID, cfg); err != nil {
			return "", "", "", nil, fmt.Errorf("failed to save alibaba config: %w", err)
		}
		resolvedURL := NormalizeBaseURL(cfg)
		resolvedName := APIKeyProviderName(cfg)
		log.Info().Str("provider", instanceID).Str("api_format", "anthropic").Msg("Alibaba authenticated via API key (Anthropic mode)")
		return options.APIKey, resolvedURL, resolvedName, cfg, nil
	}

	region := strings.TrimSpace(options.Region)
	if region == "" {
		region = "global"
	}
	plan := NormalizeAPIPlan(options.Plan)

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, "alibaba", tokenData); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to save alibaba token: %w", err)
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
		return "", "", "", nil, fmt.Errorf("failed to save alibaba config: %w", err)
	}

	resolvedURL := NormalizeBaseURL(cfg)
	resolvedName := APIKeyProviderName(cfg)

	log.Info().
		Str("provider", instanceID).
		Str("region", region).
		Str("plan", plan).
		Msg("Alibaba authenticated via API key")

	return options.APIKey, resolvedURL, resolvedName, cfg, nil
}

// LoadTokenFromDB reads the persisted Alibaba token from the database.
// Returns the token string, base URL, config, and any error.
func LoadTokenFromDB(instanceID string) (token, baseURL string, config map[string]interface{}, err error) {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to load token from DB: %w", err)
	}
	if record == nil {
		return "", "", nil, nil
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to parse token data: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	resolvedURL := NormalizeBaseURL(cfg)
	return td.AccessToken, resolvedURL, cfg, nil
}

// SaveOAuthToken persists a completed OAuth token for the given provider instance.
// It derives the canonical instance ID from the JWT email or token suffix and
// returns it so the caller can rename the provider in the registry.
func SaveOAuthToken(instanceID string, td *TokenData) (newInstanceID, name, baseURL string, err error) {
	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	resolvedURL := NormalizeBaseURL(cfg)

	// Determine region
	region := "china"
	if td.ResourceURL != "" && strings.Contains(strings.ToLower(td.ResourceURL), "dashscope-intl") {
		region = "global"
	} else if resolvedURL != "" && strings.Contains(strings.ToLower(resolvedURL), "dashscope-intl") {
		region = "global"
	}

	// Derive display name and canonical instance ID
	email := ExtractEmailFromJWT(td.AccessToken)
	if email != "" {
		safe := strings.ReplaceAll(email, "@", "-")
		safe = strings.ReplaceAll(safe, ".", "-")
		newInstanceID = "alibaba-oauth-" + region + "-" + safe
		name = "Alibaba (" + email + ")"
	} else {
		suffix := ShortTokenSuffix(td.AccessToken)
		newInstanceID = "alibaba-oauth-" + region + "-" + suffix
		name = "Alibaba OAuth (" + suffix + ")"
	}

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save(newInstanceID, "alibaba", td); err != nil {
		return "", "", "", fmt.Errorf("alibaba: failed to save OAuth token: %w", err)
	}

	// Remove old temporary record (best-effort)
	if instanceID != newInstanceID {
		_ = tokenStore.Delete(instanceID)
	}

	log.Info().
		Str("instanceID", newInstanceID).
		Str("name", name).
		Str("resource_url", td.ResourceURL).
		Msg("Alibaba OAuth token saved")

	return newInstanceID, name, resolvedURL, nil
}

// EnsureFreshToken refreshes the OAuth token if it is about to expire.
// Returns the current (possibly refreshed) access token.
// Errors are logged but not propagated so callers always get a token to try.
func EnsureFreshToken(instanceID, currentToken string) string {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil || record == nil {
		return currentToken
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return currentToken
	}

	if !IsExpiringSoon(&td) {
		return td.AccessToken
	}

	if td.RefreshToken == "" {
		log.Warn().Str("provider", instanceID).Msg("Alibaba OAuth token expiring but no refresh token available")
		return currentToken
	}

	refreshed, err := RefreshToken(context.Background(), td.RefreshToken)
	if err != nil {
		log.Warn().Err(err).Str("provider", instanceID).Msg("Alibaba OAuth token refresh failed")
		return currentToken
	}

	// Persist refreshed token (best-effort)
	_, _, _, saveErr := SaveOAuthToken(instanceID, refreshed)
	if saveErr != nil {
		log.Warn().Err(saveErr).Str("provider", instanceID).Msg("Failed to persist refreshed Alibaba token")
	}

	return refreshed.AccessToken
}

// ─── Headers ──────────────────────────────────────────────────────────────────

// Headers returns the HTTP headers required for Alibaba API requests.
// When stream is true, Accept is set to text/event-stream.
// config must contain auth_type to determine OAuth vs API-key header sets.
func Headers(token string, stream bool, config map[string]interface{}) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	if stream {
		headers["Accept"] = "text/event-stream"
	}

	if authType, _ := shared.FirstString(config, "auth_type", "authType"); authType == "oauth" {
		headers["User-Agent"] = UserAgent
		headers["X-DashScope-UserAgent"] = UserAgent
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

// ChatURL returns the chat completions URL for the given base URL.
func ChatURL(baseURL string) string {
	base := baseURL
	if base == "" {
		base = BaseURLGlobal
	}
	return base + "/chat/completions"
}

// IsAnthropicMode reports whether the provider config selects the Anthropic-compatible API.
// This is enabled by setting api_format = "anthropic" in the provider config.
func IsAnthropicMode(config map[string]interface{}) bool {
	apiFormat, _ := shared.FirstString(config, "api_format", "apiFormat")
	return strings.EqualFold(strings.TrimSpace(apiFormat), "anthropic")
}

// AnthropicMessagesURL returns the messages endpoint for the Alibaba Anthropic-compatible API.
func AnthropicMessagesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = AnthropicBaseURL
	}
	return base + "/messages"
}

// ─── Models ───────────────────────────────────────────────────────────────────

// AnthropicModels lists the Claude models available via the Alibaba Anthropic-compatible API
// (https://dashscope.aliyuncs.com/apps/anthropic). Used when api_format=anthropic.
var AnthropicModels = []types.Model{
	{ID: "claude-opus-4.5", Name: "Claude Opus 4.5", MaxTokens: 32000, Provider: "alibaba"},
	{ID: "claude-sonnet-4.5", Name: "Claude Sonnet 4.5", MaxTokens: 64000, Provider: "alibaba"},
	{ID: "claude-haiku-4.5", Name: "Claude Haiku 4.5", MaxTokens: 32000, Provider: "alibaba"},
}

var anthropicAliasTargets = map[string][]string{
	"opus": {
		"qwen3-max",
		"qwen3-max-preview",
		"qwen3.6-plus",
	},
	"sonnet": {
		"qwen3.6-plus",
		"qwen3-coder-plus",
		"qwen3-coder-next",
		"qwen-plus",
	},
	"haiku": {
		"qwen3-coder-flash",
		"qwen3.6-plus",
		"qwen3-coder-next",
		"qwen-turbo",
	},
}

// GetModelsAnthropicMode returns the hardcoded Claude model list for Anthropic-mode providers.
func GetModelsAnthropicMode(instanceID string) *types.ModelsResponse {
	result := make([]types.Model, len(AnthropicModels))
	for i, m := range AnthropicModels {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// RemapModel translates Anthropic Claude aliases to supported Alibaba upstream
// models. Non-Claude model IDs are returned unchanged.
func RemapModel(modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return modelID
	}

	normalized := strings.ToLower(trimmed)
	if !strings.HasPrefix(normalized, "claude-") {
		return trimmed
	}

	switch {
	case strings.Contains(normalized, "opus"):
		if mapped, ok := firstAvailableModel(anthropicAliasTargets["opus"]); ok {
			return mapped
		}
	case strings.Contains(normalized, "sonnet"):
		if mapped, ok := firstAvailableModel(anthropicAliasTargets["sonnet"]); ok {
			return mapped
		}
	case strings.Contains(normalized, "haiku"):
		if mapped, ok := firstAvailableModel(anthropicAliasTargets["haiku"]); ok {
			return mapped
		}
	}

	return trimmed
}

func firstAvailableModel(candidates []string) (string, bool) {
	for _, candidate := range candidates {
		if _, ok := ModelMetadata(candidate); ok {
			return candidate, true
		}
	}
	return "", false
}

// GetModels returns the available models for this Alibaba instance.
// For OAuth providers it returns a filtered hardcoded list; for API-key it tries
// live discovery then falls back to the hardcoded catalog.
func GetModels(instanceID, token, baseURL string, config map[string]interface{}) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated (set access_token via admin UI)")
	}

	// Anthropic-compatible mode: return Claude model catalog directly.
	if IsAnthropicMode(config) {
		return GetModelsAnthropicMode(instanceID), nil
	}

	authType, _ := shared.FirstString(config, "auth_type", "authType")
	if authType == "oauth" {
		return GetModelsHardcoded(instanceID, config), nil
	}

	resp, err := FetchModelsFromAPI(instanceID, token, baseURL)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}

	log.Warn().Err(err).Str("provider", instanceID).Msg("Failed to fetch models from API, using hardcoded list")
	return GetModelsHardcoded(instanceID, config), nil
}

// GetModelsHardcoded returns the hardcoded model catalog for this Alibaba instance.
func GetModelsHardcoded(instanceID string, config map[string]interface{}) *types.ModelsResponse {
	models := Models
	authType, _ := shared.FirstString(config, "auth_type", "authType")
	if authType == "oauth" {
		filtered := make([]types.Model, 0, len(models))
		for _, model := range models {
			if OAuthSupportedModels[model.ID] {
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
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// FetchModelsFromAPI fetches available models from the Alibaba DashScope API.
func FetchModelsFromAPI(instanceID, token, baseURL string) (*types.ModelsResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create alibaba models request: %w", err)
	}

	for key, value := range Headers(token, false, map[string]interface{}{}) {
		req.Header.Set(key, value)
	}

	client := alibabaHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("alibaba models fetch failed (%d)", resp.StatusCode)
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
		if !IsChatCompletionsModel(model.ID) {
			continue
		}
		result := types.Model{
			ID:       model.ID,
			Name:     model.ID,
			Provider: instanceID,
		}
		if metadata, ok := ModelMetadata(model.ID); ok {
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

// IsChatCompletionsModel returns true if the model supports chat completions
// (i.e., is not a realtime-only model).
func IsChatCompletionsModel(modelID string) bool {
	return !strings.Contains(strings.ToLower(modelID), "realtime")
}

// IsReasoningModel returns true if the model ID is a known Qwen reasoning model
// (Qwen3/Qwen3.5/Qwen3.6/QwQ/Qwen-Plus family) that supports the enable_thinking flag
// on the DashScope China endpoint.
func IsReasoningModel(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.Contains(lower, "qwen3") ||
		strings.Contains(lower, "qwq") ||
		strings.Contains(lower, "qwen-plus") ||
		strings.Contains(lower, "qwen3.5") ||
		strings.Contains(lower, "qwen3.6")
}

// ModelMetadata returns the hardcoded metadata for the given model ID if known.
func ModelMetadata(modelID string) (types.Model, bool) {
	for _, m := range Models {
		if m.ID == modelID {
			return m, true
		}
	}
	return types.Model{}, false
}

// ─── URL helpers ──────────────────────────────────────────────────────────────

// NormalizeBaseURL derives the base URL from a provider config map.
func NormalizeBaseURL(config map[string]interface{}) string {
	authType, _ := shared.FirstString(config, "auth_type", "authType")

	switch authType {
	case "api-key":
		// Anthropic-compatible mode: use the Anthropic base URL.
		if IsAnthropicMode(config) {
			if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok && baseURL != "" {
				return EnsureBaseURL(baseURL, false)
			}
			return AnthropicBaseURL
		}
		if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok {
			return EnsureBaseURL(baseURL, false)
		}
		plan, _ := shared.FirstString(config, "plan")
		region, _ := shared.FirstString(config, "region")
		return DefaultAPIBaseURL(plan, region)
	case "oauth":
		if resourceURL, ok := shared.FirstString(config, "resource_url", "resourceUrl"); ok {
			return EnsureBaseURL(resourceURL, true)
		}
		return "https://portal.qwen.ai/v1"
	default:
		if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok {
			return EnsureBaseURL(baseURL, false)
		}
		if resourceURL, ok := shared.FirstString(config, "resource_url", "resourceUrl"); ok {
			return EnsureBaseURL(resourceURL, true)
		}
	}
	return EnsureBaseURL(BaseURLGlobal, false)
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

// DefaultAPIBaseURL returns the default DashScope base URL for the given plan and region.
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

// APIKeyProviderName returns the display name for an API-key authenticated Alibaba provider.
func APIKeyProviderName(config map[string]interface{}) string {
	if IsAnthropicMode(config) {
		return "Alibaba Anthropic API"
	}
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

// EnsureBaseURL normalizes a base URL to have https scheme and /v1 suffix.
// When forOAuth is true, portal.qwen.ai/v1 is used as the fallback.
func EnsureBaseURL(raw string, forOAuth bool) string {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		if forOAuth {
			baseURL = "https://portal.qwen.ai/v1"
		} else {
			baseURL = BaseURLGlobal
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

// ─── JWT / token helpers ──────────────────────────────────────────────────────

// IsJWT checks if a token has the format of a JWT (xxx.yyy.zzz).
func IsJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
}

// ShortTokenSuffix returns the last 5 characters of a token for display purposes.
func ShortTokenSuffix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) >= 5 {
		return trimmed[len(trimmed)-5:]
	}
	if trimmed == "" {
		return "oauth"
	}
	return trimmed
}

// ExtractEmailFromJWT decodes the payload of a JWT and extracts the email claim.
func ExtractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
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

// ─── Payload helpers ──────────────────────────────────────────────────────────

// EnsureOAuthSystemMessage collapses all system messages into one starting with
// "You are Qwen Code." — required by portal.qwen.ai OAuth chat completions.
func EnsureOAuthSystemMessage(messages []map[string]interface{}) []map[string]interface{} {
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

// BuildOpenAIPayload builds the OpenAI-compatible request payload for Alibaba.
// enableThinking should be true for DashScope China reasoning models (dashscope.aliyuncs.com)
// to receive reasoning_content in the response. It has no effect on the international endpoint
// or OAuth-based portal.qwen.ai requests.
//
// Important: DashScope China does not return tool_calls SSE deltas when enable_thinking=true
// is active. When the request carries real tools, enable_thinking is omitted entirely so
// the model can emit tool calls normally.
func BuildOpenAIPayload(model string, messages []map[string]interface{}, request *cif.CanonicalRequest, isOAuth bool, enableThinking bool) map[string]interface{} {
	if isOAuth {
		messages = EnsureOAuthSystemMessage(messages)
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	// Apply Qwen-recommended sampling defaults when the caller hasn't provided values.
	// opencode uses temperature=0.55 and top_p=1 for all Qwen models.
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	} else {
		payload["temperature"] = 0.55
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	} else {
		payload["top_p"] = 1.0
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
	} else {
		// Qwen3 requires at least one tool to be present in the payload, otherwise
		// it returns an error.  Inject a dummy no-op tool and force tool_choice to
		// "none" so the model never actually calls it.
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
		payload["tool_choice"] = "none"
	}

	if request.ToolChoice != nil {
		if toolChoice := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	// DashScope China reasoning models require enable_thinking=true to return
	// reasoning_content in the response.  This flag is a no-op on the
	// international endpoint and OAuth portal.qwen.ai requests.
	//
	// However, DashScope China does not emit tool_calls SSE deltas when
	// enable_thinking=true is set.  When the request carries real tools we
	// omit enable_thinking entirely so the model can emit tool calls normally.
	if enableThinking && len(request.Tools) == 0 {
		payload["enable_thinking"] = true
	}

	return payload
}
