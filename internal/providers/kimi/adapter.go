package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

type Provider struct {
	// mu guards the mutable auth/config fields (token, baseURL, config, name).
	// A single *Provider is shared across concurrent requests; these fields are
	// read on every request and written by SetupAuth/RefreshToken/LoadFromDB.
	mu         sync.RWMutex
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	configOnce sync.Once
}

type Adapter struct {
	provider *Provider
}

func NewProvider(instanceID, name string) *Provider {
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = "Kimi"
	}
	return &Provider{
		instanceID: instanceID,
		name:       displayName,
		baseURL:    BaseURL,
	}
}

func (p *Provider) GetID() string         { return string(types.ProviderKimi) }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}
func (p *Provider) SetName(name string) {
	p.mu.Lock()
	p.name = name
	p.mu.Unlock()
}
func (p *Provider) SetInstanceID(id string) {
	p.instanceID = id
}

func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "", "api-key":
		token, baseURL, name, config, err := SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.mu.Lock()
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		p.mu.Unlock()
		return nil
	case "oauth":
		return p.LoadFromDB()
	default:
		return fmt.Errorf("kimi: unsupported auth method: %s", options.Method)
	}
}

func (p *Provider) GetToken() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token
}

func (p *Provider) RefreshToken() error {
	p.mu.RLock()
	instanceID := p.instanceID
	current := p.token
	p.mu.RUnlock()

	fresh := EnsureFreshToken(instanceID, current)

	p.mu.Lock()
	p.token = fresh
	p.mu.Unlock()
	return nil
}

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.baseURL
}

func (p *Provider) GetConfig() map[string]interface{} {
	p.ensureConfig()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	p.mu.RLock()
	token, config := p.token, p.config
	p.mu.RUnlock()
	return Headers(token, false, config)
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	p.mu.RLock()
	token, baseURL, config := p.token, p.baseURL, p.config
	p.mu.RUnlock()
	return GetModels(p.instanceID, token, baseURL, config)
}

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: use the adapter for chat completions", p.GetID())
}

func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: embeddings not yet implemented in Go backend", p.GetID())
}

func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

func (p *Provider) LoadFromDB() error {
	token, baseURL, config, err := LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	p.mu.Unlock()
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

func (p *Provider) ensureConfig() {
	p.configOnce.Do(func() {
		configStore := database.NewProviderConfigStore()
		record, err := configStore.Get(p.instanceID)
		if err != nil || record == nil {
			return
		}
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
			return
		}
		p.applyConfig(config)
	})
}

// applyConfig merges config into the provider under the write lock. Callers must
// NOT hold p.mu.
//
// It uses copy-on-write: a brand-new map is built from the existing config plus
// the incoming keys and then swapped in. This guarantees that any reader which
// captured the previous *config pointer under the read lock continues to
// iterate an immutable map — mutating the shared map in place would race with
// those readers (e.g. Headers → FirstString ranging over the same map).
func (p *Provider) applyConfig(config map[string]interface{}) {
	if len(config) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	merged := make(map[string]interface{}, len(p.config)+len(config))
	for key, value := range p.config {
		merged[key] = value
	}
	for key, value := range config {
		merged[key] = value
	}
	p.config = merged
	p.baseURL = NormalizeBaseURL(p.config)
	if p.name == "" || p.name == p.instanceID || p.name == "Kimi" {
		if authType, _ := shared.FirstString(p.config, "auth_type", "authType"); authType == "api-key" {
			p.name = APIKeyProviderName(p.config)
		}
	}
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return strings.TrimSpace(model) }

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false
	a.provider.mu.RLock()
	token, baseURL, config := a.provider.token, a.provider.baseURL, a.provider.config
	a.provider.mu.RUnlock()
	return shared.ExecuteOpenAIWithPayload(ctx, ChatURL(baseURL), Headers(token, false, config), payload)
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	a.provider.mu.RLock()
	token, baseURL, config := a.provider.token, a.provider.baseURL, a.provider.config
	a.provider.mu.RUnlock()
	return shared.StreamOpenAIWithPayload(ctx, ChatURL(baseURL), Headers(token, true, config), payload)
}

func (a *Adapter) buildOpenAIPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if system := cif.PlainSystemText(request.System); strings.TrimSpace(system) != "" {
		messages = append([]map[string]interface{}{{
			"role":    "system",
			"content": system,
		}}, messages...)
	}
	if IsThinkingModel(model) {
		EnsureReasoningContentInMessages(messages)
	}
	return BuildOpenAIPayload(model, messages, request, false)
}
