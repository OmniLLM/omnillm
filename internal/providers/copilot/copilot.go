// Package copilot provides GitHub Copilot provider implementation
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	"regexp"
	"strings"
	"time"

	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	copilotHTTPClient = &http.Client{
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
	copilotStreamClient = &http.Client{
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

const (
	copilotMaxUserIDLength          = 64
	copilotMaxToolNameLength        = 64
)

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
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	canonicalReq, err := ingestion.ParseOpenAIChatCompletions(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	adapter := p.GetAdapter().(*CopilotAdapter)
	response, err := adapter.Execute(context.Background(), canonicalReq)
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

func (a *CopilotAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeOpenAI(ctx, request)
}

func (a *CopilotAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeOpenAIStream(ctx, request)
}

func (a *CopilotAdapter) RemapModel(canonicalModel string) string {
	return modelrouting.NormalizeModelName(canonicalModel)
}

func (a *CopilotAdapter) executeOpenAI(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeOpenAIWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeOpenAIWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (*cif.CanonicalResponse, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = false

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
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
			return a.executeOpenAIWithRetry(ctx, request, false)
		}
		return nil, apiErr
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertOpenAIToCIF(openaiResp, toolNameMapper), nil
}

func (a *CopilotAdapter) executeOpenAIStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeOpenAIStreamWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeOpenAIStreamWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (<-chan cif.CIFStreamEvent, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = true

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
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
			return a.executeOpenAIStreamWithRetry(ctx, request, false)
		}
		return nil, apiErr
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go a.parseOpenAISSE(resp.Body, eventCh, toolNameMapper)
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
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)
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

// copilotModelUsesMaxCompletionTokens returns true for models that require
// "max_completion_tokens" instead of the legacy "max_tokens" parameter.
// This includes o-series reasoning models and the gpt-5 family.
func copilotModelUsesMaxCompletionTokens(model string) bool {
	lower := strings.ToLower(model)
	// o-series reasoning models (o1, o1-mini, o3, o3-mini, o4-mini, …)
	if strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") {
		return true
	}
	// gpt-5 and later generations
	if strings.HasPrefix(lower, "gpt-5") {
		return true
	}
	return false
}

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
		if copilotModelUsesMaxCompletionTokens(payload["model"].(string)) {
			payload["max_completion_tokens"] = *request.MaxTokens
		} else {
			payload["max_tokens"] = *request.MaxTokens
		}
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
