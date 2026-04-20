// Package copilot provides GitHub Copilot provider implementation
package copilot

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	copilotHTTPClient    = &http.Client{Timeout: 120 * time.Second}
	copilotStreamClient  = &http.Client{} // no timeout for streaming
)

type GitHubCopilotProvider struct {
	id           string
	instanceID   string
	name         string
	token        string // short-lived Copilot API token
	githubToken  string // long-lived GitHub OAuth token
	expiresAt    int64  // Copilot token expiry (unix timestamp)
	baseURL      string
	tokenFetcher func(string) (*ghservice.CopilotTokenResponse, error)
}

type CopilotAdapter struct {
	provider *GitHubCopilotProvider
}

const copilotResponsesMinOutputTokens = 16
const copilotMaxUserIDLength = 64
const copilotMaxToolNameLength = 64

var copilotToolNamePattern = regexp.MustCompile(`[^A-Za-z0-9_-]`)

type copilotAPIError struct {
	statusCode int
	body       []byte
}

func (e *copilotAPIError) Error() string {
	return fmt.Sprintf("API request failed with status %d: %s", e.statusCode, string(e.body))
}

func (e *copilotAPIError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.statusCode
}

func (e *copilotAPIError) IsAuthenticationError() bool {
	if e == nil {
		return false
	}
	if e.statusCode == http.StatusUnauthorized || e.statusCode == http.StatusForbidden {
		return true
	}

	body := strings.ToLower(string(e.body))
	return strings.Contains(body, "token expired") || strings.Contains(body, "unauthorized")
}

type copilotErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type copilotToolNameMapper struct {
	upstreamByCanonical map[string]string
	canonicalByUpstream map[string]string
}

type copilotResponsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details,omitempty"`
}

type copilotResponsesContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type copilotResponsesOutputItem struct {
	Type      string                         `json:"type"`
	ID        string                         `json:"id"`
	CallID    string                         `json:"call_id"`
	Name      string                         `json:"name"`
	Arguments string                         `json:"arguments"`
	Status    string                         `json:"status"`
	Content   []copilotResponsesContentBlock `json:"content"`
}

type copilotResponsesResponse struct {
	ID                string                       `json:"id"`
	Model             string                       `json:"model"`
	Status            string                       `json:"status"`
	Output            []copilotResponsesOutputItem `json:"output"`
	OutputText        interface{}                  `json:"output_text"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage *copilotResponsesUsage `json:"usage"`
}

type copilotResponsesStreamState struct {
	streamStarted     bool
	responseID        string
	model             string
	nextContentIndex  int
	textBlockIndices  map[string]int
	textBlockHasDelta map[string]bool
	toolCallIndices   map[int]int
	toolCallHasDelta  map[int]bool
}

func NewGitHubCopilotProvider(instanceID string) *GitHubCopilotProvider {
	return &GitHubCopilotProvider{
		id:           "github-copilot",
		instanceID:   instanceID,
		name:         "GitHub Copilot",
		baseURL:      "https://api.githubcopilot.com",
		tokenFetcher: ghservice.GetCopilotToken,
	}
}

// Provider interface implementation
func (p *GitHubCopilotProvider) GetID() string         { return p.id }
func (p *GitHubCopilotProvider) GetInstanceID() string { return p.instanceID }
func (p *GitHubCopilotProvider) GetName() string       { return p.name }

// SetInstanceID updates the provider's in-memory instance ID.
// Used by auth-and-create flow to assign the canonical ID after successful auth.
func (p *GitHubCopilotProvider) SetInstanceID(newID string) { p.instanceID = newID }

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
			if login, ok := user["login"].(string); ok {
				p.name = fmt.Sprintf("GitHub Copilot (%s)", login)
				log.Info().Str("provider", p.instanceID).Str("login", login).Msg("GitHub Copilot authenticated")
			}
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
		if login, ok := user["login"].(string); ok {
			p.name = fmt.Sprintf("GitHub Copilot (%s)", login)

			log.Info().
				Str("instance_id", p.instanceID).
				Str("login", login).
				Msg("GitHub Copilot authenticated via device code")
		}
	}

	// Exchange GitHub token for Copilot token
	if err := p.RefreshToken(); err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to get Copilot token")
		return err
	}

	return nil
}

// SetGitHubToken sets the long-lived GitHub OAuth token (used for Copilot token refresh)
func (p *GitHubCopilotProvider) SetGitHubToken(token string) {
	p.githubToken = token
}

func (p *GitHubCopilotProvider) GetToken() string {
	if p.githubToken != "" {
		needsRefresh := p.token == "" || p.expiresAt == 0 || time.Now().Unix() > p.expiresAt-300
		if needsRefresh {
			if err := p.RefreshToken(); err != nil {
				log.Warn().Err(err).Msg("Failed to auto-refresh Copilot token")
			}
		}
	}
	return p.token
}

func (p *GitHubCopilotProvider) RefreshToken() error {
	if p.githubToken == "" {
		log.Debug().Str("provider", p.instanceID).Msg("No GitHub token available for refresh")
		return nil
	}

	fetcher := p.tokenFetcher
	if fetcher == nil {
		fetcher = ghservice.GetCopilotToken
	}

	copilotToken, err := fetcher(p.githubToken)
	if err != nil {
		return fmt.Errorf("failed to refresh Copilot token: %w", err)
	}

	p.token = copilotToken.Token
	p.expiresAt = copilotToken.ExpiresAt

	log.Info().Str("provider", p.instanceID).Msg("Copilot token refreshed")
	return nil
}

// LoadFromDB loads saved tokens from the database
func (p *GitHubCopilotProvider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if record == nil {
		return nil // no saved token
	}

	var tokenData map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
		return fmt.Errorf("failed to parse token data: %w", err)
	}

	if gt, ok := tokenData["github_token"].(string); ok {
		p.githubToken = gt
	}
	if ct, ok := tokenData["copilot_token"].(string); ok {
		p.token = ct
	}
	if ea, ok := tokenData["expires_at"].(float64); ok {
		p.expiresAt = int64(ea)
	}
	if name, ok := tokenData["name"].(string); ok && name != "" {
		p.name = name
	}

	// If we have a GitHub token, refresh the Copilot token if expired
	if p.githubToken != "" && (p.token == "" || time.Now().Unix() > p.expiresAt-300) {
		if err := p.RefreshToken(); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to refresh token on load")
		}
	}

	if p.token != "" {
		log.Info().Str("provider", p.instanceID).Msg("Loaded saved token")
	}

	return nil
}

// SaveToDB saves the GitHub OAuth token and Copilot API token to database
func (p *GitHubCopilotProvider) SaveToDB() error {
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"github_token":  p.githubToken,
		"copilot_token": p.token,
		"expires_at":    p.expiresAt,
		"name":          p.name,
	}

	return tokenStore.Save(p.instanceID, p.id, tokenData)
}

func (p *GitHubCopilotProvider) GetBaseURL() string {
	return p.baseURL
}

func (p *GitHubCopilotProvider) GetHeaders(forVision bool) map[string]string {
	token := p.GetToken()
	headers := map[string]string{
		"Authorization":                       fmt.Sprintf("Bearer %s", token),
		"Content-Type":                        "application/json",
		"Accept":                              "application/json",
		"copilot-integration-id":              "vscode-chat",
		"Editor-Version":                      ghservice.EditorVersion,
		"Editor-Plugin-Version":               ghservice.PluginVersion,
		"User-Agent":                          ghservice.UserAgent,
		"OpenAI-Intent":                       "conversation-panel",
		"X-Github-Api-Version":                ghservice.APIVersion,
		"X-Request-Id":                        generateCopilotRequestID(),
		"X-Vscode-User-Agent-Library-Version": "electron-fetch",
	}

	if forVision {
		headers["copilot-vision-request"] = "true"
	}

	return headers
}

func generateCopilotRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}

	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (p *GitHubCopilotProvider) GetModels() (*types.ModelsResponse, error) {
	token := p.GetToken()
	if token == "" {
		return nil, fmt.Errorf("provider not authenticated")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/models", p.baseURL), nil)
	if err == nil {
		for k, v := range p.GetHeaders(false) {
			req.Header.Set(k, v)
		}

			client := copilotHTTPClient
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var payload struct {
					Data []struct {
						ID           string `json:"id"`
						Name         string `json:"name"`
						Capabilities struct {
							Tokenizer string `json:"tokenizer"`
							Limits    struct {
								MaxContextWindowTokens int `json:"max_context_window_tokens"`
								MaxOutputTokens        int `json:"max_output_tokens"`
							} `json:"limits"`
							Supports struct {
								ToolCalls         bool `json:"tool_calls"`
								ParallelToolCalls bool `json:"parallel_tool_calls"`
								Dimensions        bool `json:"dimensions"`
							} `json:"supports"`
						} `json:"capabilities"`
					} `json:"data"`
					Object string `json:"object"`
				}

				decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
				if decodeErr == nil && len(payload.Data) > 0 {
					models := make([]types.Model, 0, len(payload.Data))
					for _, model := range payload.Data {
						capabilities := map[string]interface{}{}
						if model.Capabilities.Tokenizer != "" {
							capabilities["tokenizer"] = model.Capabilities.Tokenizer
						}
						if model.Capabilities.Supports.ToolCalls {
							capabilities["function_calling"] = true
						}
						if model.Capabilities.Supports.ParallelToolCalls {
							capabilities["parallel_tool_calls"] = true
						}
						if model.Capabilities.Supports.Dimensions {
							capabilities["embeddings"] = true
						}

						maxTokens := model.Capabilities.Limits.MaxContextWindowTokens
						if maxTokens == 0 {
							maxTokens = model.Capabilities.Limits.MaxOutputTokens
						}

						models = append(models, types.Model{
							ID:           model.ID,
							Name:         firstNonEmpty(model.Name, model.ID),
							MaxTokens:    maxTokens,
							Provider:     p.instanceID,
							Capabilities: capabilities,
						})
					}

					return &types.ModelsResponse{
						Data:   models,
						Object: firstNonEmpty(payload.Object, "list"),
					}, nil
				}

				log.Warn().Err(decodeErr).Str("provider", p.instanceID).Msg("Failed to decode Copilot models response, falling back to built-in model list")
			} else {
				body, _ := io.ReadAll(resp.Body)
				log.Warn().
					Str("provider", p.instanceID).
					Int("status", resp.StatusCode).
					Str("body", string(body)).
					Msg("Copilot models request failed, falling back to built-in model list")
			}
		} else {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to fetch Copilot models, falling back to built-in model list")
		}
	}

	models := []types.Model{
		{
			ID:          "gpt-4o",
			Name:        "GPT-4o",
			Description: "Most capable GPT-4o model with vision",
			MaxTokens:   128000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
			},
		},
		{
			ID:          "gpt-4o-mini",
			Name:        "GPT-4o Mini",
			Description: "Fast and efficient GPT-4o model",
			MaxTokens:   128000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
			},
		},
		{
			ID:          "o1",
			Name:        "o1",
			Description: "Advanced reasoning model",
			MaxTokens:   200000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"reasoning":        true,
				"function_calling": false,
				"streaming":        false,
			},
		},
		{
			ID:          "o1-mini",
			Name:        "o1-mini",
			Description: "Reasoning model optimized for speed",
			MaxTokens:   65536,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"reasoning":        true,
				"function_calling": false,
				"streaming":        false,
			},
		},
		{
			ID:          "o3-mini",
			Name:        "o3-mini",
			Description: "Fast and efficient o3 reasoning model",
			MaxTokens:   200000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"reasoning":        true,
				"function_calling": true,
				"streaming":        true,
			},
		},
		{
			ID:          "claude-3.5-sonnet",
			Name:        "Claude 3.5 Sonnet",
			Description: "Anthropic Claude 3.5 Sonnet",
			MaxTokens:   200000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "cl100k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
			},
		},
		{
			ID:          "claude-3.7-sonnet",
			Name:        "Claude 3.7 Sonnet",
			Description: "Anthropic Claude 3.7 Sonnet with extended thinking",
			MaxTokens:   200000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "cl100k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
				"reasoning":        true,
			},
		},
		{
			ID:          "gemini-2.0-flash-001",
			Name:        "Gemini 2.0 Flash",
			Description: "Google Gemini 2.0 Flash via Copilot",
			MaxTokens:   1048576,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "cl100k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
			},
		},
	}

	return &types.ModelsResponse{
		Data:   models,
		Object: "list",
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

// Legacy interface methods
func (p *GitHubCopilotProvider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	canonicalReq, err := ingestion.ParseOpenAIChatCompletions(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	adapter := p.GetAdapter().(*CopilotAdapter)
	response, err := adapter.Execute(canonicalReq)
	if err != nil {
		return nil, err
	}

	openaiResp, err := serialization.SerializeToOpenAI(response)
	if err != nil {
		return nil, err
	}

	respBytes, _ := json.Marshal(openaiResp)
	var result map[string]interface{}
	json.Unmarshal(respBytes, &result)

	return result, nil
}

func (p *GitHubCopilotProvider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	if p.token == "" {
		return nil, fmt.Errorf("provider not authenticated")
	}

	// Forward to Copilot embeddings endpoint
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", p.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.GetHeaders(false) {
		req.Header.Set(k, v)
	}

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings response: %w", err)
	}

	return result, nil
}

func (p *GitHubCopilotProvider) GetUsage() (map[string]interface{}, error) {
	if p.githubToken == "" {
		return nil, errors.New("GitHub token not available")
	}
	return ghservice.GetCopilotUsage(p.githubToken)
}

func (p *GitHubCopilotProvider) GetAdapter() types.ProviderAdapter {
	return &CopilotAdapter{provider: p}
}

// CIF Adapter implementation
func (a *CopilotAdapter) GetProvider() types.Provider {
	return a.provider
}

func (a *CopilotAdapter) Execute(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	if !a.forceChatCompletions(request) && a.shouldUseResponsesAPI(request.Model) {
		return a.executeResponses(request)
	}

	response, err := a.executeOpenAI(request)
	if err == nil {
		return response, nil
	}

	var apiErr *copilotAPIError
	if !a.forceChatCompletions(request) && errors.As(err, &apiErr) && a.isUnsupportedChatCompletionsModel(apiErr) {
		log.Info().
			Str("model", request.Model).
			Str("provider", a.provider.GetInstanceID()).
			Msg("Copilot model requires responses API, retrying request")
		return a.executeResponses(request)
	}

	return nil, err
}

func (a *CopilotAdapter) ExecuteStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if !a.forceChatCompletions(request) && a.shouldUseResponsesAPI(request.Model) {
		return a.executeResponsesStream(request)
	}

	eventCh, err := a.executeOpenAIStream(request)
	if err == nil {
		return eventCh, nil
	}

	var apiErr *copilotAPIError
	if !a.forceChatCompletions(request) && errors.As(err, &apiErr) && a.isUnsupportedChatCompletionsModel(apiErr) {
		log.Info().
			Str("model", request.Model).
			Str("provider", a.provider.GetInstanceID()).
			Msg("Copilot model requires responses API for streaming, retrying request")
		return a.executeResponsesStream(request)
	}

	return nil, err
}

func (a *CopilotAdapter) RemapModel(canonicalModel string) string {
	return modelrouting.NormalizeModelName(canonicalModel)
}

func (a *CopilotAdapter) shouldUseResponsesAPI(model string) bool {
	modelLower := strings.ToLower(model)
	return strings.Contains(modelLower, "gpt-5.4")
}

func (a *CopilotAdapter) forceChatCompletions(request *cif.CanonicalRequest) bool {
	return request != nil &&
		request.Extensions != nil &&
		request.Extensions.ForceChatCompletions != nil &&
		*request.Extensions.ForceChatCompletions
}

func (a *CopilotAdapter) isUnsupportedChatCompletionsModel(apiErr *copilotAPIError) bool {
	if apiErr == nil || apiErr.statusCode != http.StatusBadRequest {
		return false
	}

	var payload copilotErrorEnvelope
	if err := json.Unmarshal(apiErr.body, &payload); err == nil {
		if payload.Error.Code == "unsupported_api_for_model" {
			return true
		}

		return strings.Contains(strings.ToLower(payload.Error.Message), "/chat/completions")
	}

	return strings.Contains(strings.ToLower(string(apiErr.body)), "unsupported_api_for_model")
}

func (a *CopilotAdapter) executeOpenAI(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeOpenAIWithRetry(request, true)
}

func (a *CopilotAdapter) executeOpenAIWithRetry(request *cif.CanonicalRequest, allowAuthRetry bool) (*cif.CanonicalResponse, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = false

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions") {
			return a.executeOpenAIWithRetry(request, false)
		}
		return nil, apiErr
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertOpenAIToCIF(openaiResp, toolNameMapper), nil
}

func (a *CopilotAdapter) executeOpenAIStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeOpenAIStreamWithRetry(request, true)
}

func (a *CopilotAdapter) executeOpenAIStreamWithRetry(request *cif.CanonicalRequest, allowAuthRetry bool) (<-chan cif.CIFStreamEvent, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = true

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	resp, err := copilotStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions-stream") {
			return a.executeOpenAIStreamWithRetry(request, false)
		}
		return nil, apiErr
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go a.parseOpenAISSE(resp.Body, eventCh, toolNameMapper)
	return eventCh, nil
}

func (a *CopilotAdapter) executeResponses(request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeResponsesWithRetry(request, true)
}

func (a *CopilotAdapter) executeResponsesWithRetry(request *cif.CanonicalRequest, allowAuthRetry bool) (*cif.CanonicalResponse, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	responsesPayload := a.convertCIFToResponses(request, toolNameMapper)
	responsesPayload["stream"] = false

	reqBody, err := json.Marshal(responsesPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/responses", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("responses request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("responses") {
			return a.executeResponsesWithRetry(request, false)
		}
		return nil, apiErr
	}

	var responsesResp copilotResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		return nil, fmt.Errorf("failed to decode responses payload: %w", err)
	}

	return a.convertResponsesToCIF(&responsesResp, toolNameMapper), nil
}

func (a *CopilotAdapter) executeResponsesStream(request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeResponsesStreamWithRetry(request, true)
}

func (a *CopilotAdapter) executeResponsesStreamWithRetry(request *cif.CanonicalRequest, allowAuthRetry bool) (<-chan cif.CIFStreamEvent, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	responsesPayload := a.convertCIFToResponses(request, toolNameMapper)
	responsesPayload["stream"] = true

	reqBody, err := json.Marshal(responsesPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/responses", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	resp, err := copilotStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("responses streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("responses-stream") {
			return a.executeResponsesStreamWithRetry(request, false)
		}
		return nil, apiErr
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go a.parseResponsesSSE(resp.Body, eventCh, toolNameMapper)
	return eventCh, nil
}

func (a *CopilotAdapter) shouldRetryAfterAuthError(request *cif.CanonicalRequest, apiErr *copilotAPIError) bool {
	if request != nil &&
		request.Extensions != nil &&
		request.Extensions.DisableAuthRetry != nil &&
		*request.Extensions.DisableAuthRetry {
		return false
	}

	return apiErr != nil && apiErr.IsAuthenticationError() && a.provider.githubToken != ""
}

func (a *CopilotAdapter) requestHeaders(request *cif.CanonicalRequest) map[string]string {
	headers := a.provider.GetHeaders(a.requestUsesVision(request))
	headers["X-Initiator"] = a.requestInitiator(request)
	return headers
}

func (a *CopilotAdapter) requestUsesVision(request *cif.CanonicalRequest) bool {
	if request == nil {
		return false
	}

	for _, message := range request.Messages {
		for _, part := range messageContentParts(message) {
			if _, ok := part.(cif.CIFImagePart); ok {
				return true
			}
		}
	}

	return false
}

func (a *CopilotAdapter) requestInitiator(request *cif.CanonicalRequest) string {
	if request == nil {
		return "user"
	}

	for _, message := range request.Messages {
		switch msg := message.(type) {
		case cif.CIFAssistantMessage:
			if len(msg.Content) > 0 {
				return "agent"
			}
		case cif.CIFUserMessage:
			for _, part := range msg.Content {
				if _, ok := part.(cif.CIFToolResultPart); ok {
					return "agent"
				}
			}
		}
	}

	return "user"
}

func messageContentParts(message cif.CIFMessage) []cif.CIFContentPart {
	switch msg := message.(type) {
	case cif.CIFUserMessage:
		return msg.Content
	case cif.CIFAssistantMessage:
		return msg.Content
	default:
		return nil
	}
}

func (a *CopilotAdapter) refreshTokenForRetry(endpoint string) bool {
	if err := a.provider.RefreshToken(); err != nil {
		log.Warn().
			Err(err).
			Str("provider", a.provider.GetInstanceID()).
			Str("endpoint", endpoint).
			Msg("Failed to refresh Copilot token after upstream auth error")
		return false
	}

	log.Info().
		Str("provider", a.provider.GetInstanceID()).
		Str("endpoint", endpoint).
		Msg("Refreshed Copilot token after upstream auth error, retrying request")
	return true
}

func (a *CopilotAdapter) parseOpenAISSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent, toolNameMapper *copilotToolNameMapper) {
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
			eventCh <- cif.CIFStreamEnd{
				Type:       "stream_end",
				StopReason: cif.StopReasonEndTurn,
			}
			return
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Warn().Err(err).Msg("Failed to parse SSE chunk")
			continue
		}

		if !streamStartSent {
			id, _ := chunk["id"].(string)
			model, _ := chunk["model"].(string)
			eventCh <- cif.CIFStreamStart{
				Type:  "stream_start",
				ID:    id,
				Model: model,
			}
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
				StopReason: a.convertOpenAIStopReason(finishReason),
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
				Type:  "content_delta",
				Index: contentBlockIndex,
				ContentBlock: cif.CIFTextPart{
					Type: "text",
					Text: "",
				},
				Delta: cif.TextDelta{
					Type: "text_delta",
					Text: content,
				},
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
							ToolName:      toolNameMapper.fromUpstream(name),
							ToolArguments: map[string]interface{}{},
						},
						Delta: cif.ToolArgumentsDelta{
							Type:        "tool_arguments_delta",
							PartialJSON: "",
						},
					}
				} else if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						eventCh <- cif.CIFContentDelta{
							Type:  "content_delta",
							Index: contentBlockIndex,
							Delta: cif.ToolArgumentsDelta{
								Type:        "tool_arguments_delta",
								PartialJSON: args,
							},
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    "stream_error",
				Message: err.Error(),
			},
		}
	}
}

func (a *CopilotAdapter) parseResponsesSSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent, toolNameMapper *copilotToolNameMapper) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	state := &copilotResponsesStreamState{
		textBlockIndices:  make(map[string]int),
		textBlockHasDelta: make(map[string]bool),
		toolCallIndices:   make(map[int]int),
		toolCallHasDelta:  make(map[int]bool),
	}

	var eventType string
	var dataLines []string

	flushEvent := func() {
		if eventType == "" || len(dataLines) == 0 {
			eventType = ""
			dataLines = nil
			return
		}

		a.handleResponsesSSEEvent(eventType, strings.Join(dataLines, "\n"), state, eventCh, toolNameMapper)
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushEvent()
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}

	flushEvent()

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("Responses SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    "stream_error",
				Message: err.Error(),
			},
		}
	}
}

func (a *CopilotAdapter) handleResponsesSSEEvent(
	eventType string,
	data string,
	state *copilotResponsesStreamState,
	eventCh chan cif.CIFStreamEvent,
	toolNameMapper *copilotToolNameMapper,
) {
	if data == "" {
		return
	}

	switch eventType {
	case "response.created":
		var payload struct {
			Response *copilotResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			if err != nil {
				log.Warn().Err(err).Msg("Failed to parse response.created event")
			}
			return
		}

		state.responseID = payload.Response.ID
		state.model = payload.Response.Model
		state.streamStarted = true
		eventCh <- cif.CIFStreamStart{
			Type:  "stream_start",
			ID:    payload.Response.ID,
			Model: payload.Response.Model,
		}

	case "response.output_item.added":
		var payload struct {
			Item        copilotResponsesOutputItem `json:"item"`
			OutputIndex int                        `json:"output_index"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("Failed to parse response.output_item.added event")
			return
		}

		if payload.Item.Type != "function_call" {
			return
		}

		index := state.nextContentIndex
		state.nextContentIndex++
		state.toolCallIndices[payload.OutputIndex] = index

		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			ContentBlock: cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(payload.Item.CallID, payload.Item.ID),
				ToolName:      toolNameMapper.fromUpstream(payload.Item.Name),
				ToolArguments: map[string]interface{}{},
			},
			Delta: cif.ToolArgumentsDelta{
				Type:        "tool_arguments_delta",
				PartialJSON: "",
			},
		}

		if payload.Item.Arguments != "" {
			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				Delta: cif.ToolArgumentsDelta{
					Type:        "tool_arguments_delta",
					PartialJSON: payload.Item.Arguments,
				},
			}
			state.toolCallHasDelta[payload.OutputIndex] = true
		}

	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		var payload struct {
			OutputIndex int    `json:"output_index"`
			Delta       string `json:"delta"`
			Arguments   string `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("Failed to parse function_call_arguments event")
			return
		}

		index, ok := state.toolCallIndices[payload.OutputIndex]
		if !ok {
			return
		}

		partial := payload.Delta
		if partial == "" {
			partial = payload.Arguments
		}
		if partial == "" {
			return
		}
		if eventType == "response.function_call_arguments.done" && state.toolCallHasDelta[payload.OutputIndex] {
			return
		}

		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.ToolArgumentsDelta{
				Type:        "tool_arguments_delta",
				PartialJSON: partial,
			},
		}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.output_text.delta", "response.output_text.done":
		var payload struct {
			OutputIndex  int    `json:"output_index"`
			ContentIndex int    `json:"content_index"`
			Delta        string `json:"delta"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("Failed to parse output_text event")
			return
		}

		index, isNew := state.ensureTextBlockIndex(payload.OutputIndex, payload.ContentIndex)
		key := state.textBlockKey(payload.OutputIndex, payload.ContentIndex)
		text := payload.Delta
		if text == "" {
			text = payload.Text
		}
		if text == "" {
			return
		}
		if eventType == "response.output_text.done" && state.textBlockHasDelta[key] {
			return
		}

		contentDelta := cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.TextDelta{
				Type: "text_delta",
				Text: text,
			},
		}
		if isNew {
			contentDelta.ContentBlock = cif.CIFTextPart{
				Type: "text",
				Text: "",
			}
		}

		eventCh <- contentDelta
		state.textBlockHasDelta[key] = true

	case "response.completed":
		var payload struct {
			Response *copilotResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			if err != nil {
				log.Warn().Err(err).Msg("Failed to parse response.completed event")
			}
			return
		}

		eventCh <- cif.CIFStreamEnd{
			Type:       "stream_end",
			StopReason: a.convertResponsesStopReason(payload.Response),
			Usage:      a.convertResponsesUsage(payload.Response.Usage),
		}

	case "error", "response.failed":
		var payload struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
			Response *struct {
				Error *struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("Failed to parse responses error event")
			return
		}

		errInfo := payload.Error
		if errInfo == nil && payload.Response != nil {
			errInfo = payload.Response.Error
		}
		if errInfo == nil {
			return
		}

		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    firstNonEmpty(errInfo.Type, "stream_error"),
				Message: errInfo.Message,
			},
		}
	}
}

func (s *copilotResponsesStreamState) ensureTextBlockIndex(outputIndex, contentIndex int) (int, bool) {
	key := s.textBlockKey(outputIndex, contentIndex)
	if index, ok := s.textBlockIndices[key]; ok {
		return index, false
	}

	index := s.nextContentIndex
	s.nextContentIndex++
	s.textBlockIndices[key] = index
	return index, true
}

func (s *copilotResponsesStreamState) textBlockKey(outputIndex, contentIndex int) string {
	return fmt.Sprintf("%d:%d", outputIndex, contentIndex)
}

// Helper methods
func (a *CopilotAdapter) convertCIFToOpenAI(request *cif.CanonicalRequest, toolNameMapper *copilotToolNameMapper) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    a.RemapModel(request.Model),
		"messages": a.convertCIFMessagesToOpenAI(request.Messages, toolNameMapper),
		"stream":   request.Stream,
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
		payload["user"] = sanitizeCopilotUserID(*request.UserID)
	}

	if len(request.Tools) > 0 {
		var tools []map[string]interface{}
		for _, tool := range request.Tools {
			openaiTool := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":       toolNameMapper.toUpstream(tool.Name),
					"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
				},
			}
			if tool.Description != nil {
				openaiTool["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, openaiTool)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		if toolChoice := convertCanonicalToolChoiceToOpenAI(request.ToolChoice, toolNameMapper); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	return payload
}

func (a *CopilotAdapter) convertCIFToResponses(request *cif.CanonicalRequest, toolNameMapper *copilotToolNameMapper) map[string]interface{} {
	input := make([]map[string]interface{}, 0)
	instructions := make([]string, 0)

	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		instructions = append(instructions, *request.SystemPrompt)
	}

	for _, msg := range request.Messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			if strings.TrimSpace(m.Content) != "" {
				instructions = append(instructions, m.Content)
			}
		default:
			input = append(input, a.convertCIFMessageToResponses(msg, toolNameMapper)...)
		}
	}

	payload := map[string]interface{}{
		"model": a.RemapModel(request.Model),
		"input": input,
		"store": false,
	}

	if len(instructions) > 0 {
		payload["instructions"] = strings.Join(instructions, "\n\n")
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		maxOutputTokens := *request.MaxTokens
		if maxOutputTokens < copilotResponsesMinOutputTokens {
			maxOutputTokens = copilotResponsesMinOutputTokens
		}
		payload["max_output_tokens"] = maxOutputTokens
	}
	if request.UserID != nil {
		payload["user"] = sanitizeCopilotUserID(*request.UserID)
	}

	if tools := a.convertCIFToolsToResponses(request.Tools, toolNameMapper); len(tools) > 0 {
		payload["tools"] = tools
	}

	if toolChoice := a.convertCIFToolChoiceToResponses(request.ToolChoice, toolNameMapper); toolChoice != nil {
		payload["tool_choice"] = toolChoice
	}

	return payload
}

func (a *CopilotAdapter) convertCIFMessageToResponses(message cif.CIFMessage, toolNameMapper *copilotToolNameMapper) []map[string]interface{} {
	switch m := message.(type) {
	case cif.CIFSystemMessage:
		return []map[string]interface{}{{
			"type": "message",
			"role": "system",
			"content": []map[string]interface{}{{
				"type": "input_text",
				"text": m.Content,
			}},
		}}
	case cif.CIFUserMessage:
		return a.convertResponsesMessageParts("user", m.Content, toolNameMapper)
	case cif.CIFAssistantMessage:
		return a.convertResponsesMessageParts("assistant", m.Content, toolNameMapper)
	default:
		return nil
	}
}

func normalizeResponsesToolCallID(toolCallID string) string {
	if toolCallID == "" {
		return ""
	}

	if strings.HasPrefix(toolCallID, "fc") {
		return toolCallID
	}

	trimmedID := toolCallID
	if underscoreIdx := strings.Index(trimmedID, "_"); underscoreIdx >= 0 && underscoreIdx < len(trimmedID)-1 {
		trimmedID = trimmedID[underscoreIdx+1:]
	}

	return "fc_" + trimmedID
}

func sanitizeCopilotUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if len(trimmed) <= copilotMaxUserIDLength {
		return trimmed
	}

	return trimmed[:copilotMaxUserIDLength]
}

func convertCanonicalToolChoiceToOpenAI(toolChoice interface{}, toolNameMapper *copilotToolNameMapper) interface{} {
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
				"name": toolNameMapper.toUpstream(functionName),
			},
		}
	default:
		return nil
	}
}

func (a *CopilotAdapter) convertResponsesMessageParts(role string, parts []cif.CIFContentPart, toolNameMapper *copilotToolNameMapper) []map[string]interface{} {
	items := make([]map[string]interface{}, 0)
	contentBlocks := make([]map[string]interface{}, 0)

	flushMessage := func() {
		if len(contentBlocks) == 0 {
			return
		}

		messageContent := make([]map[string]interface{}, len(contentBlocks))
		copy(messageContent, contentBlocks)
		items = append(items, map[string]interface{}{
			"type":    "message",
			"role":    role,
			"content": messageContent,
		})
		contentBlocks = contentBlocks[:0]
	}

	blockType := "input_text"
	if role == "assistant" {
		blockType = "output_text"
	}

	for _, part := range parts {
		switch p := part.(type) {
		case cif.CIFTextPart:
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": blockType,
				"text": p.Text,
			})
		case cif.CIFThinkingPart:
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": blockType,
				"text": p.Thinking,
			})
		case cif.CIFImagePart:
			imageURL := a.responsesImageURL(p)
			if imageURL == "" {
				continue
			}
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":      "input_image",
				"image_url": imageURL,
			})
		case cif.CIFToolCallPart:
			if role != "assistant" {
				continue
			}
			flushMessage()

			args, _ := json.Marshal(p.ToolArguments)
			responsesToolCallID := normalizeResponsesToolCallID(p.ToolCallID)
			item := map[string]interface{}{
				"type":      "function_call",
				"name":      toolNameMapper.toUpstream(p.ToolName),
				"arguments": string(args),
			}
			if responsesToolCallID != "" {
				item["id"] = responsesToolCallID
				item["call_id"] = responsesToolCallID
			}
			items = append(items, item)
		case cif.CIFToolResultPart:
			if role != "user" {
				continue
			}
			flushMessage()
			items = append(items, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": normalizeResponsesToolCallID(p.ToolCallID),
				"output":  p.Content,
			})
		}
	}

	flushMessage()
	return items
}

func (a *CopilotAdapter) convertCIFToolsToResponses(tools []cif.CIFTool, toolNameMapper *copilotToolNameMapper) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		responseTool := map[string]interface{}{
			"type":       "function",
			"name":       toolNameMapper.toUpstream(tool.Name),
			"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
		}
		if tool.Description != nil && *tool.Description != "" {
			responseTool["description"] = *tool.Description
		}
		result = append(result, responseTool)
	}
	return result
}

func (a *CopilotAdapter) convertCIFToolChoiceToResponses(toolChoice interface{}, toolNameMapper *copilotToolNameMapper) interface{} {
	if toolChoice == nil {
		return nil
	}

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
				"name": toolNameMapper.toUpstream(functionName),
			},
		}
	default:
		return nil
	}
}

func (a *CopilotAdapter) responsesImageURL(part cif.CIFImagePart) string {
	if part.Data != nil && *part.Data != "" {
		return fmt.Sprintf("data:%s;base64,%s", part.MediaType, *part.Data)
	}
	if part.URL != nil {
		return *part.URL
	}
	return ""
}

func (a *CopilotAdapter) convertResponsesToCIF(responsesResp *copilotResponsesResponse, toolNameMapper *copilotToolNameMapper) *cif.CanonicalResponse {
	response := &cif.CanonicalResponse{
		ID:         responsesResp.ID,
		Model:      responsesResp.Model,
		StopReason: a.convertResponsesStopReason(responsesResp),
		Usage:      a.convertResponsesUsage(responsesResp.Usage),
	}

	for _, item := range responsesResp.Output {
		switch item.Type {
		case "message":
			for _, block := range item.Content {
				if block.Type == "output_text" && block.Text != "" {
					response.Content = append(response.Content, cif.CIFTextPart{
						Type: "text",
						Text: block.Text,
					})
				}
			}
		case "function_call":
			response.Content = append(response.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(item.CallID, item.ID),
				ToolName:      toolNameMapper.fromUpstream(item.Name),
				ToolArguments: a.parseToolArguments(item.Arguments),
			})
		}
	}

	if len(response.Content) == 0 {
		for _, text := range extractCopilotResponsesOutputTexts(responsesResp) {
			response.Content = append(response.Content, cif.CIFTextPart{
				Type: "text",
				Text: text,
			})
		}
	}

	return response
}

func (a *CopilotAdapter) convertResponsesStopReason(responsesResp *copilotResponsesResponse) cif.CIFStopReason {
	for _, item := range responsesResp.Output {
		if item.Type == "function_call" {
			return cif.StopReasonToolUse
		}
	}

	if responsesResp.IncompleteDetails != nil {
		switch responsesResp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return cif.StopReasonMaxTokens
		case "content_filter":
			return cif.StopReasonContentFilter
		}
	}

	return cif.StopReasonEndTurn
}

func (a *CopilotAdapter) convertResponsesUsage(usage *copilotResponsesUsage) *cif.CIFUsage {
	if usage == nil {
		return nil
	}

	result := &cif.CIFUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	}

	if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
		cachedTokens := usage.InputTokensDetails.CachedTokens
		result.CacheReadInputTokens = &cachedTokens
	}

	return result
}

func extractCopilotResponsesOutputTexts(responsesResp *copilotResponsesResponse) []string {
	var texts []string

	switch outputText := responsesResp.OutputText.(type) {
	case string:
		if outputText != "" {
			texts = append(texts, outputText)
		}
	case []interface{}:
		for _, item := range outputText {
			if textMap, ok := item.(map[string]interface{}); ok {
				text, _ := textMap["text"].(string)
				itemType, _ := textMap["type"].(string)
				if (itemType == "output_text" || itemType == "text") && text != "" {
					texts = append(texts, text)
				}
			}
		}
	}

	if len(texts) > 0 {
		return texts
	}

	for _, item := range responsesResp.Output {
		if item.Type != "message" {
			continue
		}
		for _, block := range item.Content {
			if block.Type == "output_text" && block.Text != "" {
				texts = append(texts, block.Text)
			}
		}
	}

	return texts
}

func (a *CopilotAdapter) parseToolArguments(arguments string) map[string]interface{} {
	if arguments == "" {
		return map[string]interface{}{}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &parsed); err == nil {
		return parsed
	}

	return map[string]interface{}{
		"_unparsable_arguments": arguments,
	}
}

func (a *CopilotAdapter) convertCIFMessagesToOpenAI(messages []cif.CIFMessage, toolNameMapper *copilotToolNameMapper) []map[string]interface{} {
	var openaiMessages []map[string]interface{}

	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":    "system",
				"content": m.Content,
			})

		case cif.CIFUserMessage:
			openaiMsg := map[string]interface{}{
				"role": "user",
			}

			if len(m.Content) == 1 {
				if textPart, ok := m.Content[0].(cif.CIFTextPart); ok {
					openaiMsg["content"] = textPart.Text
					openaiMessages = append(openaiMessages, openaiMsg)
					continue
				}
			}

			var contentParts []map[string]interface{}
			for _, part := range m.Content {
				if toolResult, ok := part.(cif.CIFToolResultPart); ok {
					openaiMessages = append(openaiMessages, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": toolResult.ToolCallID,
						"content":      toolResult.Content,
					})
					continue
				}

				contentParts = append(contentParts, a.convertCIFPartToOpenAI(part))
			}

			if len(contentParts) > 0 {
				openaiMsg["content"] = contentParts
				openaiMessages = append(openaiMessages, openaiMsg)
			}

		case cif.CIFAssistantMessage:
			openaiMsg := map[string]interface{}{
				"role": "assistant",
			}

			var textBuf strings.Builder
			var toolCalls []map[string]interface{}

			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBuf.WriteString(p.Text)
				case cif.CIFThinkingPart:
					textBuf.WriteString(p.Thinking)
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCall := map[string]interface{}{
						"id":   p.ToolCallID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      toolNameMapper.toUpstream(p.ToolName),
							"arguments": string(args),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				}
			}

			if textBuf.Len() > 0 {
				openaiMsg["content"] = textBuf.String()
			}
			if len(toolCalls) > 0 {
				openaiMsg["tool_calls"] = toolCalls
			}

			openaiMessages = append(openaiMessages, openaiMsg)
		}
	}

	return openaiMessages
}

func (a *CopilotAdapter) convertCIFPartToOpenAI(part cif.CIFContentPart) map[string]interface{} {
	switch p := part.(type) {
	case cif.CIFTextPart:
		return map[string]interface{}{
			"type": "text",
			"text": p.Text,
		}
	case cif.CIFThinkingPart:
		return map[string]interface{}{
			"type": "text",
			"text": p.Thinking,
		}
	case cif.CIFImagePart:
		imageURL := map[string]interface{}{}
		if p.Data != nil {
			imageURL["url"] = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
		} else if p.URL != nil {
			imageURL["url"] = *p.URL
		}
		return map[string]interface{}{
			"type":      "image_url",
			"image_url": imageURL,
		}
	default:
		return map[string]interface{}{
			"type": "text",
			"text": "[Unsupported content type]",
		}
	}
}

func (a *CopilotAdapter) convertOpenAIToCIF(openaiResp map[string]interface{}, toolNameMapper *copilotToolNameMapper) *cif.CanonicalResponse {
	id, _ := openaiResp["id"].(string)
	model, _ := openaiResp["model"].(string)

	response := &cif.CanonicalResponse{
		ID:         id,
		Model:      model,
		StopReason: cif.StopReasonEndTurn,
	}

	if choices, ok := openaiResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if finishReason, ok := choice["finish_reason"].(string); ok {
				response.StopReason = a.convertOpenAIStopReason(finishReason)
			}

			if message, ok := choice["message"].(map[string]interface{}); ok {
				response.Content = a.convertOpenAIMessageToCIF(message, toolNameMapper)
			}
		}
	}

	if usage, ok := openaiResp["usage"].(map[string]interface{}); ok {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			if completionTokens, ok := usage["completion_tokens"].(float64); ok {
				response.Usage = &cif.CIFUsage{
					InputTokens:  int(promptTokens),
					OutputTokens: int(completionTokens),
				}
			}
		}
	}

	return response
}

func (a *CopilotAdapter) convertOpenAIMessageToCIF(message map[string]interface{}, toolNameMapper *copilotToolNameMapper) []cif.CIFContentPart {
	var parts []cif.CIFContentPart

	if content, ok := message["content"].(string); ok && content != "" {
		parts = append(parts, cif.CIFTextPart{
			Type: "text",
			Text: content,
		})
	}

	if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
		for _, tc := range toolCalls {
			if toolCall, ok := tc.(map[string]interface{}); ok {
				if function, ok := toolCall["function"].(map[string]interface{}); ok {
					id, _ := toolCall["id"].(string)
					name, _ := function["name"].(string)
					args, _ := function["arguments"].(string)

					var toolArgs map[string]interface{}
					json.Unmarshal([]byte(args), &toolArgs)

					parts = append(parts, cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    id,
						ToolName:      toolNameMapper.fromUpstream(name),
						ToolArguments: toolArgs,
					})
				}
			}
		}
	}

	return parts
}

func newCopilotToolNameMapper(request *cif.CanonicalRequest) *copilotToolNameMapper {
	mapper := &copilotToolNameMapper{
		upstreamByCanonical: make(map[string]string),
		canonicalByUpstream: make(map[string]string),
	}
	if request == nil {
		return mapper
	}

	register := func(name string) {
		if name == "" {
			return
		}
		if _, exists := mapper.upstreamByCanonical[name]; exists {
			return
		}

		alias := normalizeCopilotToolName(name)
		if alias == name {
			return
		}

		mapper.upstreamByCanonical[name] = alias
		mapper.canonicalByUpstream[alias] = name
	}

	for _, tool := range request.Tools {
		register(tool.Name)
	}

	switch choice := request.ToolChoice.(type) {
	case map[string]interface{}:
		if functionName, _ := choice["functionName"].(string); functionName != "" {
			register(functionName)
		}
		if function, ok := choice["function"].(map[string]interface{}); ok {
			if name, _ := function["name"].(string); name != "" {
				register(name)
			}
		}
	}

	for _, message := range request.Messages {
		switch msg := message.(type) {
		case cif.CIFUserMessage:
			for _, part := range msg.Content {
				if toolResult, ok := part.(cif.CIFToolResultPart); ok {
					register(toolResult.ToolName)
				}
			}
		case cif.CIFAssistantMessage:
			for _, part := range msg.Content {
				if toolCall, ok := part.(cif.CIFToolCallPart); ok {
					register(toolCall.ToolName)
				}
			}
		}
	}

	if len(mapper.upstreamByCanonical) > 0 {
		log.Debug().
			Int("aliased_tools", len(mapper.upstreamByCanonical)).
			Msg("Aliased Copilot tool names to satisfy upstream limits")
	}

	return mapper
}

func (m *copilotToolNameMapper) toUpstream(name string) string {
	if m == nil {
		return name
	}
	if aliased, ok := m.upstreamByCanonical[name]; ok {
		return aliased
	}
	return name
}

func (m *copilotToolNameMapper) fromUpstream(name string) string {
	if m == nil {
		return name
	}
	if original, ok := m.canonicalByUpstream[name]; ok {
		return original
	}
	return name
}

func normalizeCopilotToolName(name string) string {
	if name == "" {
		return ""
	}

	sanitized := copilotToolNamePattern.ReplaceAllString(name, "_")
	sanitized = strings.Trim(sanitized, "_-")
	if sanitized == "" {
		sanitized = "tool"
	}

	if sanitized == name && len(sanitized) <= copilotMaxToolNameLength {
		return name
	}

	sum := sha1.Sum([]byte(name))
	hashSuffix := fmt.Sprintf("%x", sum[:])[:12]
	maxPrefixLength := copilotMaxToolNameLength - len(hashSuffix) - 1
	if maxPrefixLength < 1 {
		maxPrefixLength = 1
	}
	if len(sanitized) > maxPrefixLength {
		sanitized = sanitized[:maxPrefixLength]
	}
	sanitized = strings.TrimRight(sanitized, "_-")
	if sanitized == "" {
		sanitized = "tool"
	}
	if len(sanitized) > maxPrefixLength {
		sanitized = sanitized[:maxPrefixLength]
	}

	return fmt.Sprintf("%s_%s", sanitized, hashSuffix)
}

func (a *CopilotAdapter) convertOpenAIStopReason(reason string) cif.CIFStopReason {
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
