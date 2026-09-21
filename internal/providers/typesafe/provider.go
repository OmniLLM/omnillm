// Package typesafe implements native TypeSafe System One evaluations.
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/systemone"
)

const BaseURL = "https://api.typesafe.ai/v1"
const maxResponseBytes = 16 << 20

type Provider struct {
	mu         sync.RWMutex
	instanceID string
	name       string
	token      string
	baseURL    string
	client     *http.Client
}

func NewProvider(instanceID, name string) *Provider {
	if strings.TrimSpace(name) == "" {
		name = "TypeSafe"
	}
	return &Provider{instanceID: instanceID, name: name, baseURL: BaseURL, client: &http.Client{
		Timeout:       120 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func (p *Provider) GetID() string         { return string(types.ProviderTypeSafe) }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}
func (p *Provider) SetName(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.name = name
}
func (p *Provider) GetToken() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token
}
func (p *Provider) GetBaseURL() string        { return p.baseURL }
func (p *Provider) GetConfig() map[string]any { return map[string]any{"auth_type": "api-key"} }
func (p *Provider) GetHeaders(bool) map[string]string {
	return map[string]string{"Authorization": "Bearer " + p.GetToken(), "Content-Type": "application/json"}
}
func (p *Provider) RefreshToken() error { return nil }
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	if options == nil || strings.TrimSpace(options.APIKey) == "" {
		return errors.New("typesafe API key is required")
	}
	if options.Method != "" && options.Method != "api-key" {
		return errors.New("typesafe supports API-key authentication only")
	}
	key := strings.TrimSpace(options.APIKey)
	if _, err := p.fetchModels(context.Background(), key); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := database.NewTokenStore().Save(p.instanceID, map[string]string{"access_token": key}); err != nil {
		return errors.New("failed to persist typesafe credentials")
	}
	p.token = key
	return nil
}
func (p *Provider) LoadFromDB() error {
	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil {
		return errors.New("failed to load typesafe credentials")
	}
	if record == nil {
		return nil
	}
	var stored struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal([]byte(record.TokenData), &stored) != nil {
		return errors.New("invalid stored typesafe credentials")
	}
	p.mu.Lock()
	p.token = stored.AccessToken
	p.mu.Unlock()
	return nil
}
func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	return p.fetchModels(context.Background(), p.GetToken())
}
func (p *Provider) fetchModels(ctx context.Context, key string) (*types.ModelsResponse, error) {
	raw, err := p.request(ctx, http.MethodGet, "/models", key, nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Models []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Models) == 0 {
		return nil, errors.New("typesafe returned an invalid model catalog")
	}
	models := make([]types.Model, 0, len(response.Models))
	for _, m := range response.Models {
		if strings.TrimSpace(m.Name) == "" {
			return nil, errors.New("typesafe returned an invalid model name")
		}
		models = append(models, types.Model{ID: m.Name, Name: m.Name, Description: m.Description, Provider: p.instanceID,
			Capabilities: map[string]any{
				"evaluation": true, "generation": false, "streaming": false,
				"tools": false, "vision": false, "embeddings": false,
				"api_modes": []string{"systemone"},
			},
		})
	}
	return &types.ModelsResponse{Object: "list", Data: models, Source: "live"}, nil
}
func (p *Provider) Evaluate(ctx context.Context, req *systemone.Request) (*systemone.Response, error) {
	if req == nil {
		return nil, errors.New("evaluation request is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, errors.New("invalid evaluation request")
	}
	if _, err := systemone.ParseRequest(payload); err != nil {
		return nil, err
	}
	raw, err := p.request(ctx, http.MethodPost, "/systemone", p.GetToken(), payload)
	if err != nil {
		return nil, err
	}
	return systemone.ValidateResponse(raw, req)
}
func (p *Provider) request(ctx context.Context, method, path, key string, body []byte) ([]byte, error) {
	if key == "" {
		return nil, errors.New("typesafe is not authenticated")
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to construct typesafe request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("typesafe request failed or timed out")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &systemone.UpstreamError{
			Status:     response.StatusCode,
			RetryAfter: validRetryAfter(response.Header.Get("Retry-After")),
		}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("failed to read typesafe response")
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("typesafe response exceeds size limit")
	}
	return raw, nil
}
func validRetryAfter(value string) string {
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return strconv.FormatUint(seconds, 10)
	}
	if _, err := http.ParseTime(value); err == nil {
		return value
	}
	return ""
}
func (p *Provider) CreateChatCompletions(map[string]any) (map[string]any, error) {
	return nil, systemone.ErrUnsupported
}
func (p *Provider) CreateEmbeddings(map[string]any) (map[string]any, error) {
	return nil, systemone.ErrUnsupported
}
func (p *Provider) GetUsage() (map[string]any, error) {
	return nil, errors.New("typesafe does not expose account usage")
}
func (p *Provider) GetAdapter() types.ProviderAdapter { return &adapter{provider: p} }

type adapter struct{ provider *Provider }

func (a *adapter) GetProvider() types.Provider    { return a.provider }
func (a *adapter) RemapModel(model string) string { return model }
func (a *adapter) Execute(context.Context, *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return nil, systemone.ErrUnsupported
}
func (a *adapter) ExecuteStream(context.Context, *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return nil, systemone.ErrUnsupported
}
