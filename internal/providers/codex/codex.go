// Package codex provides the Codex provider implementation.
// Codex authenticates via an OpenAI API key and routes requests to the
// official OpenAI Codex endpoint (or the GitHub Copilot endpoint for legacy
// GitHub OAuth tokens).
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"time"

	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

const (
	// codexBaseURL is the OpenAI endpoint used when authenticating with an API key.
	codexBaseURL = "https://api.openai.com/v1"
	// codexGitHubBaseURL is the GitHub Copilot endpoint for legacy OAuth tokens.
	codexGitHubBaseURL = "https://api.githubcopilot.com"
	codexProviderID    = "codex"
)

var (
	codexHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	codexStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

// CodexProvider authenticates via an OpenAI API key (primary) or a legacy
// GitHub OAuth token and routes requests to the Codex endpoint.
type CodexProvider struct {
	id          string
	instanceID  string
	name        string
	apiKey      string // OpenAI API key (primary auth method)
	token       string // short-lived Copilot/Codex API token (legacy GitHub path)
	githubToken string // long-lived GitHub OAuth token (legacy path)
	expiresAt   int64
	baseURL     string
}

// CodexAdapter bridges the provider to the CIF execution layer.
type CodexAdapter struct {
	provider *CodexProvider
}

// NewCodexProvider creates a new CodexProvider for the given instance ID.
func NewCodexProvider(instanceID string) *CodexProvider {
	return &CodexProvider{
		id:         codexProviderID,
		instanceID: instanceID,
		name:       "Codex",
		baseURL:    codexBaseURL,
	}
}

// ── Provider interface ───────────────────────────────────────────────────────

func (p *CodexProvider) GetID() string         { return p.id }
func (p *CodexProvider) GetInstanceID() string { return p.instanceID }
func (p *CodexProvider) GetName() string       { return p.name }
func (p *CodexProvider) SetName(name string)   { p.name = name }
func (p *CodexProvider) SetInstanceID(id string) { p.instanceID = id }

// SetupAuth configures the provider.  Pass an OpenAI API key via
// options.APIKey (primary), or a GitHub OAuth token via options.GithubToken /
// options.Token for the legacy GitHub Copilot path.
func (p *CodexProvider) SetupAuth(options *types.AuthOptions) error {
	if options == nil {
		return fmt.Errorf("auth options required")
	}

	// Primary: OpenAI API key.
	if strings.TrimSpace(options.APIKey) != "" {
		p.apiKey = strings.TrimSpace(options.APIKey)
		p.baseURL = codexBaseURL
		p.name = "Codex"
		log.Info().Str("provider", p.instanceID).Msg("Codex: configured with OpenAI API key")
		return nil
	}

	// Legacy: GitHub OAuth / personal token.
	token := strings.TrimSpace(options.GithubToken)
	if token == "" {
		token = strings.TrimSpace(options.Token)
	}
	if token == "" {
		return fmt.Errorf("an OpenAI API key (apiKey) is required for Codex")
	}
	p.githubToken = token
	p.baseURL = codexGitHubBaseURL
	if err := p.RefreshToken(); err != nil {
		return fmt.Errorf("failed to exchange GitHub token for Codex token: %w", err)
	}
	user, err := ghservice.GetUser(p.githubToken)
	if err == nil {
		if name := codexProviderName(user); name != "" {
			p.name = name
		}
	}
	return nil
}

// InitiateDeviceCodeFlow starts the GitHub OAuth device-code flow.
func (p *CodexProvider) InitiateDeviceCodeFlow() (*ghservice.DeviceCodeResponse, error) {
	log.Info().Str("provider", p.instanceID).Msg("Codex: initiating GitHub OAuth device-code flow")
	dc, err := ghservice.GetDeviceCode()
	if err != nil {
		return nil, fmt.Errorf("codex: failed to get device code: %w", err)
	}
	log.Info().Str("user_code", dc.UserCode).Str("uri", dc.VerificationURI).Msg("Codex: device code generated")
	return dc, nil
}

// PollAndCompleteDeviceCodeFlow polls GitHub until the user authorises and
// then exchanges the OAuth token for a Codex API token.
func (p *CodexProvider) PollAndCompleteDeviceCodeFlow(dc *ghservice.DeviceCodeResponse) error {
	log.Info().Str("provider", p.instanceID).Msg("Codex: polling for GitHub access token")
	accessToken, err := ghservice.PollAccessToken(dc)
	if err != nil {
		return fmt.Errorf("codex: failed to poll access token: %w", err)
	}
	p.githubToken = accessToken
	user, err := ghservice.GetUser(accessToken)
	if err == nil {
		if name := codexProviderName(user); name != "" {
			p.name = name
			log.Info().Str("name", name).Msg("Codex: authenticated via device code")
		}
	}
	if err := p.RefreshToken(); err != nil {
		return fmt.Errorf("codex: failed to get API token after OAuth: %w", err)
	}
	return nil
}

// RefreshToken exchanges the long-lived GitHub token for a short-lived
// Copilot/Codex API token.
func (p *CodexProvider) RefreshToken() error {
	if p.githubToken == "" {
		return nil
	}
	resp, err := ghservice.GetCopilotToken(p.githubToken)
	if err != nil {
		return fmt.Errorf("codex: refresh failed: %w", err)
	}
	p.token = resp.Token
	p.expiresAt = resp.ExpiresAt
	log.Info().Str("provider", p.instanceID).Msg("Codex: token refreshed")
	return nil
}

// GetToken returns a valid token for API requests.
// For API-key auth this is the key itself; for GitHub OAuth it returns the
// short-lived Copilot token, refreshing it when needed.
func (p *CodexProvider) GetToken() string {
	if p.apiKey != "" {
		return p.apiKey
	}
	if p.githubToken != "" {
		if p.token == "" || p.expiresAt == 0 || time.Now().Unix() > p.expiresAt-300 {
			if err := p.RefreshToken(); err != nil {
				log.Warn().Err(err).Msg("Codex: auto-refresh failed")
			}
		}
	}
	return p.token
}

// SaveToDB persists auth credentials to the database.
func (p *CodexProvider) SaveToDB() error {
	data := map[string]interface{}{
		"name": p.name,
	}
	if p.apiKey != "" {
		data["api_key"]     = p.apiKey
		data["auth_method"] = "api-key"
	} else {
		data["github_token"] = p.githubToken
		data["codex_token"]  = p.token
		data["expires_at"]   = p.expiresAt
		data["auth_method"]  = "github"
	}
	return database.NewTokenStore().Save(p.instanceID, p.id, data)
}

// LoadFromDB restores credentials from the database.
func (p *CodexProvider) LoadFromDB() error {
	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("codex: load from DB failed: %w", err)
	}
	if record == nil {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &data); err != nil {
		return fmt.Errorf("codex: failed to parse token data: %w", err)
	}
	if v, ok := data["name"].(string); ok && v != "" {
		p.name = v
	}
	// API-key path.
	if v, ok := data["api_key"].(string); ok && v != "" {
		p.apiKey  = v
		p.baseURL = codexBaseURL
		return nil
	}
	// Legacy GitHub OAuth path.
	if v, ok := data["github_token"].(string); ok {
		p.githubToken = v
	}
	if v, ok := data["codex_token"].(string); ok {
		p.token = v
	}
	if v, ok := data["expires_at"].(float64); ok {
		p.expiresAt = int64(v)
	}
	if p.githubToken != "" {
		p.baseURL = codexGitHubBaseURL
		if p.token == "" || time.Now().Unix() > p.expiresAt-300 {
			if err := p.RefreshToken(); err != nil {
				log.Warn().Err(err).Str("provider", p.instanceID).Msg("Codex: refresh on load failed")
			}
		}
	}
	return nil
}

func (p *CodexProvider) GetBaseURL() string { return p.baseURL }

// GetHeaders returns the HTTP headers to use for Codex API requests.
// For API-key auth only standard OpenAI headers are sent; GitHub-specific
// headers are added only for the legacy OAuth path.
func (p *CodexProvider) GetHeaders(_ bool) map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", p.GetToken()),
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	// Add GitHub Copilot headers only for legacy OAuth path.
	if p.githubToken != "" {
		headers["Editor-Version"]         = ghservice.EditorVersion
		headers["Editor-Plugin-Version"]  = ghservice.PluginVersion
		headers["User-Agent"]             = ghservice.UserAgent
		headers["X-Github-Api-Version"]   = ghservice.APIVersion
		headers["copilot-integration-id"] = "vscode-chat"
	}
	return headers
}

// GetModels returns the available Codex models.
func (p *CodexProvider) GetModels() (*types.ModelsResponse, error) {
	token := p.GetToken()
	if token == "" {
		return nil, fmt.Errorf("codex: provider not authenticated")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/models", p.baseURL), nil)
	if err == nil {
		for k, v := range p.GetHeaders(false) {
			req.Header.Set(k, v)
		}
		if resp, err := codexHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var payload struct {
					Data []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"data"`
				}
				if json.NewDecoder(resp.Body).Decode(&payload) == nil && len(payload.Data) > 0 {
					models := make([]types.Model, 0, len(payload.Data))
					for _, m := range payload.Data {
						if !strings.Contains(strings.ToLower(m.ID), "codex") {
							continue
						}
						name := m.Name
						if name == "" {
							name = m.ID
						}
						models = append(models, types.Model{
							ID:        m.ID,
							Name:      name,
							MaxTokens: 8096,
							Provider:  p.instanceID,
						})
					}
					if len(models) > 0 {
						return &types.ModelsResponse{Data: models, Object: "list"}, nil
					}
				}
			}
		}
	}

	// Fall back to known Codex models.
	return &types.ModelsResponse{
		Object: "list",
		Data: []types.Model{
			{ID: "code-davinci-002", Name: "Codex (code-davinci-002)", MaxTokens: 8001, Provider: p.instanceID},
			{ID: "code-cushman-001", Name: "Codex (code-cushman-001)", MaxTokens: 2048, Provider: p.instanceID},
		},
	}, nil
}

// GetUsage is not available for Codex; returns an empty map.
func (p *CodexProvider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// GetAdapter returns the CIF adapter for this provider.
func (p *CodexProvider) GetAdapter() types.ProviderAdapter {
	return &CodexAdapter{provider: p}
}

// ── CIF Adapter ──────────────────────────────────────────────────────────────

func (a *CodexAdapter) GetProvider() types.Provider { return a.provider }

func (a *CodexAdapter) RemapModel(model string) string { return model }

// ── Provider legacy interface methods ────────────────────────────────────────

// CreateChatCompletions implements the legacy types.Provider interface.
func (p *CodexProvider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("codex: marshal error: %w", err)
	}
	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("codex: create request error: %w", err)
	}
	for k, v := range p.GetHeaders(false) {
		req.Header.Set(k, v)
	}
	resp, err := codexHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex: API error %d: %s", resp.StatusCode, b)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("codex: decode error: %w", err)
	}
	return result, nil
}

// CreateEmbeddings is not supported by Codex.
func (p *CodexProvider) CreateEmbeddings(_ map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("codex: embeddings are not supported")
}

func (a *CodexAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := a.buildPayload(request, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("codex: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("codex: create request error: %w", err)
	}
	for k, v := range a.provider.GetHeaders(false) {
		req.Header.Set(k, v)
	}

	resp, err := codexHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex: API error %d: %s", resp.StatusCode, b)
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("codex: decode response error: %w", err)
	}
	return a.convertResponseToCIF(openaiResp), nil
}

func (a *CodexAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	payload := a.buildPayload(request, true)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("codex: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("codex: create request error: %w", err)
	}
	for k, v := range a.provider.GetHeaders(false) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := codexStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: stream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("codex: API error %d: %s", resp.StatusCode, b)
	}

	ch := make(chan cif.CIFStreamEvent, 64)
	go shared.ParseOpenAISSE(resp.Body, ch)
	return ch, nil
}

func (a *CodexAdapter) buildPayload(request *cif.CanonicalRequest, stream bool) map[string]interface{} {
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": messages,
		"stream":   stream,
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if len(request.Tools) > 0 {
		var tools []map[string]interface{}
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
	return payload
}

func (a *CodexAdapter) convertResponseToCIF(resp map[string]interface{}) *cif.CanonicalResponse {
	return shared.OpenAIRespToCIF(resp)
}

// codexProviderName builds a human-friendly display name from a GitHub /user response.
// Priority: "name · email" > "name · login" > "email" > "login".
func codexProviderName(user map[string]interface{}) string {
	login, _ := user["login"].(string)
	email, _ := user["email"].(string)
	if email == "" {
		email, _ = user["notification_email"].(string)
	}
	realName, _ := user["name"].(string)

	switch {
	case realName != "" && email != "":
		return fmt.Sprintf("Codex (%s · %s)", realName, email)
	case realName != "" && login != "":
		return fmt.Sprintf("Codex (%s · %s)", realName, login)
	case email != "":
		return fmt.Sprintf("Codex (%s)", email)
	case login != "":
		return fmt.Sprintf("Codex (%s)", login)
	default:
		return ""
	}
}
