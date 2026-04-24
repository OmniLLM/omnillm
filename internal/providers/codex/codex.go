// Package codex provides the Codex provider implementation.
// Codex authenticates via GitHub OAuth (device-code flow) and exposes
// Codex-flavoured OpenAI-compatible completions.
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
	codexBaseURL    = "https://api.githubcopilot.com"
	codexProviderID = "codex"
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

// CodexProvider authenticates via GitHub OAuth and routes requests to the
// Codex endpoint.
type CodexProvider struct {
	id          string
	instanceID  string
	name        string
	token       string // short-lived Copilot/Codex API token
	githubToken string // long-lived GitHub OAuth token
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
func (p *CodexProvider) SetInstanceID(id string) { p.instanceID = id }

// SetupAuth accepts a direct GitHub token.  When no token is provided the
// caller should start the device-code flow via InitiateDeviceCodeFlow.
func (p *CodexProvider) SetupAuth(options *types.AuthOptions) error {
	token := ""
	if options != nil {
		token = options.GithubToken
		if token == "" {
			token = options.Token
		}
		if token == "" {
			token = options.APIKey
		}
	}
	if token == "" {
		return fmt.Errorf("GitHub token required; use the OAuth device-code flow")
	}
	p.githubToken = token
	if err := p.RefreshToken(); err != nil {
		return fmt.Errorf("failed to exchange GitHub token for Codex token: %w", err)
	}
	user, err := ghservice.GetUser(p.githubToken)
	if err == nil {
		if login, ok := user["login"].(string); ok {
			p.name = fmt.Sprintf("Codex (%s)", login)
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
		if login, ok := user["login"].(string); ok {
			p.name = fmt.Sprintf("Codex (%s)", login)
			log.Info().Str("login", login).Msg("Codex: authenticated via device code")
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

// GetToken returns a valid short-lived token, refreshing if necessary.
func (p *CodexProvider) GetToken() string {
	if p.githubToken != "" {
		if p.token == "" || p.expiresAt == 0 || time.Now().Unix() > p.expiresAt-300 {
			if err := p.RefreshToken(); err != nil {
				log.Warn().Err(err).Msg("Codex: auto-refresh failed")
			}
		}
	}
	return p.token
}

// SaveToDB persists GitHub and API tokens to the database.
func (p *CodexProvider) SaveToDB() error {
	return database.NewTokenStore().Save(p.instanceID, p.id, map[string]interface{}{
		"github_token": p.githubToken,
		"codex_token":  p.token,
		"expires_at":   p.expiresAt,
		"name":         p.name,
	})
}

// LoadFromDB restores tokens from the database.
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
	if v, ok := data["github_token"].(string); ok {
		p.githubToken = v
	}
	if v, ok := data["codex_token"].(string); ok {
		p.token = v
	}
	if v, ok := data["expires_at"].(float64); ok {
		p.expiresAt = int64(v)
	}
	if v, ok := data["name"].(string); ok && v != "" {
		p.name = v
	}
	if p.githubToken != "" && (p.token == "" || time.Now().Unix() > p.expiresAt-300) {
		if err := p.RefreshToken(); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Codex: refresh on load failed")
		}
	}
	return nil
}

func (p *CodexProvider) GetBaseURL() string { return p.baseURL }

// GetHeaders returns the HTTP headers to use for Codex API requests.
func (p *CodexProvider) GetHeaders(_ bool) map[string]string {
	return map[string]string{
		"Authorization":          fmt.Sprintf("Bearer %s", p.GetToken()),
		"Content-Type":           "application/json",
		"Accept":                 "application/json",
		"Editor-Version":         ghservice.EditorVersion,
		"Editor-Plugin-Version":  ghservice.PluginVersion,
		"User-Agent":             ghservice.UserAgent,
		"X-Github-Api-Version":   ghservice.APIVersion,
		"copilot-integration-id": "vscode-chat",
	}
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
