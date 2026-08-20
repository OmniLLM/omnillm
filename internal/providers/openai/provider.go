package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

// defaultBaseURL is the Codex backend served to ChatGPT accounts.
//
// Note: this is the endpoint the Codex CLI uses. Models on api.openai.com/v1
// (gpt-5-codex, codex-mini-latest, …) are rejected here with
// "not supported when using Codex with a ChatGPT account".
const defaultBaseURL = "https://chatgpt.com/backend-api/codex"

// tokenRefreshSkew is how long before expiry we proactively refresh.
const tokenRefreshSkew = 5 * time.Minute

// defaultMaxTokens is the context window assumed when upstream does not
// report one.
const defaultMaxTokens = 128000

// Models is the fallback catalog used when the live /models probe is
// unavailable (see models.go). Each entry was verified to stream successfully
// against a live ChatGPT account; *-codex model IDs are deliberately absent
// because the backend rejects them for ChatGPT-account auth.
var Models = []types.Model{
	{ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", MaxTokens: 272000, Provider: "openai"},
	{ID: "gpt-5.6-terra", Name: "GPT-5.6 Terra", MaxTokens: 272000, Provider: "openai"},
	{ID: "gpt-5.6-luna", Name: "GPT-5.6 Luna", MaxTokens: 272000, Provider: "openai"},
	{ID: "gpt-5.5", Name: "GPT-5.5", MaxTokens: 272000, Provider: "openai"},
	{ID: "gpt-5.4", Name: "GPT-5.4", MaxTokens: 272000, Provider: "openai"},
	{ID: "gpt-5.4-mini", Name: "GPT-5.4-Mini", MaxTokens: 272000, Provider: "openai"},
}

// GetModels returns the catalog scoped to a provider instance.
func GetModels(instanceID string) *types.ModelsResponse {
	result := make([]types.Model, len(Models))
	for i, m := range Models {
		result[i] = m
		result[i].Provider = instanceID
		// Fill in output limits / capability metadata from models.dev; the
		// hand-maintained entries only carry ID, name and context window.
		EnrichFromModelsDev(&result[i])
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// RemapModel maps incoming model names onto supported Codex-backend IDs using
// the built-in catalog. Prefer (*Provider).RemapModel, which consults the live
// catalog first.
func RemapModel(model string) string {
	return remapAgainst(model, Models)
}

func remapAgainst(model string, catalog []types.Model) string {
	if len(catalog) == 0 {
		return model
	}
	for _, m := range catalog {
		if model == m.ID {
			return model
		}
	}
	// Codex-suffixed and platform model IDs are rejected upstream for ChatGPT
	// accounts; steer them to the default model rather than 400-ing.
	if strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") {
		return catalog[0].ID
	}
	return model
}

// RemapModel resolves a model against this instance's catalog, preferring the
// live list when one has been fetched.
func (p *Provider) RemapModel(model string) string {
	if resp := FetchModels(p); resp != nil && len(resp.Data) > 0 {
		return remapAgainst(model, resp.Data)
	}
	return RemapModel(model)
}

// Provider is a ChatGPT-account-backed OpenAI provider.
type Provider struct {
	// mu guards the mutable auth fields. One *Provider is shared across all
	// concurrent requests by the registry, so reads in GetToken/GetHeaders race
	// with writes in RefreshToken/LoadFromDB unless synchronized.
	mu           sync.RWMutex
	instanceID   string
	name         string
	accessToken  string
	refreshToken string
	accountID    string
	email        string
	expiresAt    int64

	// refreshGroup collapses concurrent refreshes into a single upstream call.
	refreshGroup singleflight.Group
}

// Adapter bridges the provider into the CIF execution layer.
type Adapter struct {
	provider *Provider
}

// NewProvider constructs a provider instance.
func NewProvider(instanceID, name string) *Provider {
	displayName := name
	if displayName == "" {
		displayName = "OpenAI (ChatGPT)"
	}
	return &Provider{instanceID: instanceID, name: displayName}
}

// ── Identity ─────────────────────────────────────────────────────────────────

func (p *Provider) GetID() string         { return string(types.ProviderOpenAI) }
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

// ── Authentication ───────────────────────────────────────────────────────────

// SetupAuth is not used for this provider: credentials arrive through the
// browser OAuth flow (POST /providers/openai/start-oauth).
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	return fmt.Errorf("openai: use the browser sign-in flow " +
		"(POST /api/admin/providers/openai/start-oauth) — this provider does not accept an API key")
}

// ApplyTokens stores a freshly obtained token set and persists it.
func (p *Provider) ApplyTokens(t *TokenResponse) error {
	if t == nil {
		return fmt.Errorf("openai: nil token response")
	}
	claims := ClaimsFromTokenResponse(t)

	p.mu.Lock()
	p.accessToken = t.AccessToken
	if t.RefreshToken != "" {
		p.refreshToken = t.RefreshToken
	}
	if claims != nil {
		if claims.AccountID != "" {
			p.accountID = claims.AccountID
		}
		if claims.Email != "" {
			p.email = claims.Email
			p.name = "OpenAI (" + claims.Email + ")"
		}
		if claims.ExpiresAt > 0 {
			p.expiresAt = claims.ExpiresAt
		}
	}
	if p.expiresAt == 0 && t.ExpiresIn > 0 {
		p.expiresAt = time.Now().Unix() + t.ExpiresIn
	}
	instanceID := p.instanceID
	p.mu.Unlock()

	// The catalog is account-scoped; a new token set may be a different account.
	InvalidateModelsCache(instanceID)

	return p.SaveToDB()
}

// GetToken returns a valid access token, refreshing it when near expiry.
func (p *Provider) GetToken() string {
	p.mu.RLock()
	token := p.accessToken
	expiresAt := p.expiresAt
	hasRefresh := p.refreshToken != ""
	p.mu.RUnlock()

	needsRefresh := hasRefresh && (token == "" ||
		(expiresAt > 0 && time.Now().Add(tokenRefreshSkew).Unix() > expiresAt))
	if !needsRefresh {
		return token
	}

	if err := p.RefreshToken(); err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("OpenAI: token refresh failed; using existing token")
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.accessToken
}

// RefreshToken exchanges the stored refresh token for a fresh token pair.
// Concurrent callers share a single upstream request.
func (p *Provider) RefreshToken() error {
	_, err, _ := p.refreshGroup.Do("refresh", func() (interface{}, error) {
		p.mu.RLock()
		refresh := p.refreshToken
		p.mu.RUnlock()

		if refresh == "" {
			return nil, fmt.Errorf("openai: no refresh token — re-authenticate via the browser sign-in flow")
		}

		t, err := RefreshAccessToken(refresh)
		if err != nil {
			if isRefreshTokenAlreadyUsed(err) {
				return nil, p.recoverRotatedRefreshToken(refresh)
			}
			return nil, err
		}
		if err := p.ApplyTokens(t); err != nil {
			return nil, err
		}
		log.Info().Str("provider", p.instanceID).Msg("OpenAI: access token refreshed")
		return nil, nil
	})
	return err
}

func (p *Provider) recoverRotatedRefreshToken(rejected string) error {
	stored, err := p.persistedRefreshToken()
	if err != nil {
		return fmt.Errorf("openai: reload rotated refresh token: %w", err)
	}
	if stored != "" && stored != rejected {
		return p.exchangeRecoveredRefreshToken(stored)
	}

	retired, err := database.NewTokenStore().ClearRefreshTokenIfMatches(p.instanceID, rejected)
	if err != nil {
		return fmt.Errorf("openai: retire rejected refresh token: %w", err)
	}
	if !retired {
		stored, err = p.persistedRefreshToken()
		if err != nil {
			return fmt.Errorf("openai: reload refresh token after rotation race: %w", err)
		}
		if stored != "" && stored != rejected {
			return p.exchangeRecoveredRefreshToken(stored)
		}
	}

	p.mu.Lock()
	if p.refreshToken == rejected {
		p.refreshToken = ""
	}
	p.mu.Unlock()
	return fmt.Errorf("openai: refresh token was already used; sign in again with 'omnillm provider login %s'", p.instanceID)
}

func (p *Provider) persistedRefreshToken() (string, error) {
	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil || record == nil {
		return "", err
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &data); err != nil {
		return "", err
	}
	refresh, _ := data["refresh_token"].(string)
	return refresh, nil
}

func (p *Provider) exchangeRecoveredRefreshToken(refresh string) error {
	tokens, err := RefreshAccessToken(refresh)
	if err != nil {
		return fmt.Errorf("openai: retry with newer persisted refresh token: %w", err)
	}
	if err := p.ApplyTokens(tokens); err != nil {
		return err
	}
	log.Info().Str("provider", p.instanceID).Msg("OpenAI: access token refreshed from newer persisted rotation")
	return nil
}

// ── API configuration ────────────────────────────────────────────────────────

func (p *Provider) GetBaseURL() string { return defaultBaseURL }

// GetHeaders returns the headers required by the Codex backend. The
// chatgpt-account-id header is mandatory; requests without it are rejected.
func (p *Provider) GetHeaders(_ bool) map[string]string {
	token := p.GetToken()

	p.mu.RLock()
	accountID := p.accountID
	p.mu.RUnlock()

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"OpenAI-Beta":   "responses=experimental",
		"originator":    "codex_cli_rs",
		"session_id":    shared.RandomID(),
	}
	if accountID != "" {
		headers["chatgpt-account-id"] = accountID
	}
	return headers
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	return FetchModels(p), nil
}

// ── Legacy interface methods ─────────────────────────────────────────────────

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: use the adapter for chat completions", p.GetID())
}

func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: embeddings are not supported by the Codex backend", p.GetID())
}

func (p *Provider) GetUsage() (map[string]interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	usage := map[string]interface{}{}
	if p.email != "" {
		usage["email"] = p.email
	}
	return usage, nil
}

func (p *Provider) GetAdapter() types.ProviderAdapter { return &Adapter{provider: p} }

// ── Persistence ──────────────────────────────────────────────────────────────

// SaveToDB persists the token set.
func (p *Provider) SaveToDB() error {
	p.mu.RLock()
	data := map[string]interface{}{
		"access_token":  p.accessToken,
		"refresh_token": p.refreshToken,
		"account_id":    p.accountID,
		"email":         p.email,
		"expires_at":    p.expiresAt,
		"name":          p.name,
		"auth_method":   "chatgpt-oauth",
	}
	instanceID := p.instanceID
	p.mu.RUnlock()

	return database.NewTokenStore().Save(instanceID, data)
}

// LoadFromDB restores the token set, refreshing it if already expired.
func (p *Provider) LoadFromDB() error {
	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("openai: load from DB failed: %w", err)
	}
	if record == nil {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &data); err != nil {
		return fmt.Errorf("openai: failed to parse token data: %w", err)
	}

	p.mu.Lock()
	if v, ok := data["access_token"].(string); ok {
		p.accessToken = v
	}
	if v, ok := data["refresh_token"].(string); ok {
		p.refreshToken = v
	}
	if v, ok := data["account_id"].(string); ok {
		p.accountID = v
	}
	if v, ok := data["email"].(string); ok {
		p.email = v
	}
	if v, ok := data["expires_at"].(float64); ok {
		p.expiresAt = int64(v)
	}
	if v, ok := data["name"].(string); ok && v != "" {
		p.name = v
	}
	expired := p.expiresAt > 0 && time.Now().Add(tokenRefreshSkew).Unix() > p.expiresAt
	hasRefresh := p.refreshToken != ""
	p.mu.Unlock()

	if expired && hasRefresh {
		if err := p.RefreshToken(); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("OpenAI: refresh on load failed")
		}
	}
	return nil
}

// ApplyTokenFromDB reloads credentials, ignoring errors (used post-OAuth).
func (p *Provider) ApplyTokenFromDB() { _ = p.LoadFromDB() }

// ── Adapter ──────────────────────────────────────────────────────────────────

func (a *Adapter) GetProvider() types.Provider    { return a.provider }
func (a *Adapter) RemapModel(model string) string { return a.provider.RemapModel(model) }

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return Stream(ctx, a.provider, request)
}

// Execute satisfies the non-streaming path by collecting a stream: the Codex
// backend rejects stream:false with "Stream must be set to true".
func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	ch, err := Stream(ctx, a.provider, request)
	if err != nil {
		return nil, err
	}
	return shared.CollectStream(ch)
}
