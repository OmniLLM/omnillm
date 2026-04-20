package modelrouting

import (
	"os"
	"testing"

	alibabapkg "omnillm/internal/providers/alibaba"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

// TestMain initializes a temp SQLite database so the functions that call
// database.NewModelStateStore() / database.NewProviderInstanceStore() don't panic.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "modelrouting-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := database.InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

// mockProvider is a minimal types.Provider implementation for unit testing.
type mockProvider struct {
	instanceID string
	name       string
	models     *types.ModelsResponse
	fetchErr   error
	fetchCount int
}

func (p *mockProvider) GetID() string                        { return "mock" }
func (p *mockProvider) GetInstanceID() string                { return p.instanceID }
func (p *mockProvider) GetName() string                      { return p.name }
func (p *mockProvider) SetupAuth(_ *types.AuthOptions) error { return nil }
func (p *mockProvider) GetToken() string                     { return "" }
func (p *mockProvider) RefreshToken() error                  { return nil }
func (p *mockProvider) GetBaseURL() string                   { return "" }
func (p *mockProvider) GetHeaders(_ bool) map[string]string  { return nil }
func (p *mockProvider) GetModels() (*types.ModelsResponse, error) {
	p.fetchCount++
	return p.models, p.fetchErr
}
func (p *mockProvider) CreateChatCompletions(_ map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (p *mockProvider) CreateEmbeddings(_ map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (p *mockProvider) GetUsage() (map[string]interface{}, error) { return nil, nil }
func (p *mockProvider) GetAdapter() types.ProviderAdapter         { return nil }

// ─── NormalizeModelName ───

func TestNormalizeModelName_GPT4(t *testing.T) {
	if got := NormalizeModelName("gpt-4"); got != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", got)
	}
}

func TestNormalizeModelName_GPT35Turbo(t *testing.T) {
	if got := NormalizeModelName("gpt-3.5-turbo"); got != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeSonnet(t *testing.T) {
	if got := NormalizeModelName("claude-3-sonnet"); got != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected claude-3-5-sonnet-20241022, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeHaiku45DotForm(t *testing.T) {
	if got := NormalizeModelName("claude-haiku-4.5"); got != "claude-haiku-4.5" {
		t.Errorf("expected claude-haiku-4.5, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeHaiku45DashForm(t *testing.T) {
	if got := NormalizeModelName("claude-haiku-4-5-20251001"); got != "claude-haiku-4.5" {
		t.Errorf("expected claude-haiku-4.5, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeSonnet46DotForm(t *testing.T) {
	if got := NormalizeModelName("claude-sonnet-4.6"); got != "claude-sonnet-4.6" {
		t.Errorf("expected claude-sonnet-4.6, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeSonnet46DashForm(t *testing.T) {
	if got := NormalizeModelName("claude-sonnet-4-6"); got != "claude-sonnet-4.6" {
		t.Errorf("expected claude-sonnet-4.6, got %q", got)
	}
}

func TestNormalizeModelName_ClaudeSonnet46DatedForm(t *testing.T) {
	if got := NormalizeModelName("claude-sonnet-4-6-20241022"); got != "claude-sonnet-4.6" {
		t.Errorf("expected claude-sonnet-4.6, got %q", got)
	}
}

func TestNormalizeModelName_HaikuShorthand(t *testing.T) {
	if got := NormalizeModelName("haiku"); got != "claude-haiku-4.5" {
		t.Errorf("expected claude-haiku-4.5, got %q", got)
	}
}

func TestNormalizeModelName_Sonnet46Shorthand(t *testing.T) {
	if got := NormalizeModelName("sonnet-4.6"); got != "claude-sonnet-4.6" {
		t.Errorf("expected claude-sonnet-4.6, got %q", got)
	}
}

func TestNormalizeModelName_Passthrough(t *testing.T) {
	// Unknown model names should pass through unchanged
	cases := []string{"gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet-20241022", "some-custom-model"}
	for _, name := range cases {
		if got := NormalizeModelName(name); got != name {
			t.Errorf("NormalizeModelName(%q) = %q, expected passthrough %q", name, got, name)
		}
	}
}

// ─── GetCachedOrFetchModels ───

func TestGetCachedOrFetchModels_CacheMiss(t *testing.T) {
	provider := &mockProvider{
		instanceID: "mock-1",
		name:       "Mock",
		models: &types.ModelsResponse{
			Data: []types.Model{{ID: "model-a"}, {ID: "model-b"}},
		},
	}
	cache := NewModelCache()

	result, err := GetCachedOrFetchModels(provider, cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 models, got %d", len(result.Data))
	}
	if provider.fetchCount != 1 {
		t.Errorf("expected 1 fetch, got %d", provider.fetchCount)
	}
}

func TestGetCachedOrFetchModels_CacheHit(t *testing.T) {
	provider := &mockProvider{
		instanceID: "mock-2",
		name:       "Mock2",
		models: &types.ModelsResponse{
			Data: []types.Model{{ID: "model-x"}},
		},
	}
	cache := NewModelCache()

	// First call to populate cache
	_, _ = GetCachedOrFetchModels(provider, cache)

	// Second call should use cache
	result, err := GetCachedOrFetchModels(provider, cache)
	if err != nil {
		t.Fatalf("unexpected error on cached call: %v", err)
	}
	if provider.fetchCount != 1 {
		t.Errorf("cache hit: expected still 1 fetch, got %d", provider.fetchCount)
	}
	if len(result.Data) != 1 {
		t.Errorf("expected 1 model from cache, got %d", len(result.Data))
	}
}

func TestGetCachedOrFetchModels_FetchError(t *testing.T) {
	provider := &mockProvider{
		instanceID: "mock-err",
		name:       "MockErr",
		fetchErr:   os.ErrNotExist,
	}
	cache := NewModelCache()

	_, err := GetCachedOrFetchModels(provider, cache)
	if err == nil {
		t.Error("expected error when provider returns error")
	}
}

// ─── SortProvidersByPriority ───

func TestSortProvidersByPriority_AlphabeticalFallback(t *testing.T) {
	// With no DB records, all priorities are 0 → fall back to alphabetical by instanceID
	providers := []types.Provider{
		&mockProvider{instanceID: "zzz", name: "Z"},
		&mockProvider{instanceID: "aaa", name: "A"},
		&mockProvider{instanceID: "mmm", name: "M"},
	}

	sorted := SortProvidersByPriority(providers)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(sorted))
	}
	if sorted[0].GetInstanceID() != "aaa" {
		t.Errorf("expected first = aaa, got %q", sorted[0].GetInstanceID())
	}
	if sorted[1].GetInstanceID() != "mmm" {
		t.Errorf("expected second = mmm, got %q", sorted[1].GetInstanceID())
	}
	if sorted[2].GetInstanceID() != "zzz" {
		t.Errorf("expected third = zzz, got %q", sorted[2].GetInstanceID())
	}
}

func TestSortProvidersByPriority_DoesNotMutateOriginal(t *testing.T) {
	providers := []types.Provider{
		&mockProvider{instanceID: "zzz"},
		&mockProvider{instanceID: "aaa"},
	}
	original := make([]types.Provider, len(providers))
	copy(original, providers)

	_ = SortProvidersByPriority(providers)

	// Original slice should be unchanged
	if providers[0].GetInstanceID() != original[0].GetInstanceID() {
		t.Errorf("original slice was mutated: expected %q, got %q",
			original[0].GetInstanceID(), providers[0].GetInstanceID())
	}
}

// ─── GetEnabledModelsByProvider ───

func TestGetEnabledModelsByProvider_ReturnsAllModels(t *testing.T) {
	provider := &mockProvider{
		instanceID: "test-provider",
		name:       "Test",
		models: &types.ModelsResponse{
			Data: []types.Model{
				{ID: "model-1"},
				{ID: "model-2"},
				{ID: "model-3"},
			},
		},
	}

	cache := NewModelCache()
	modelsByProvider, err := GetEnabledModelsByProvider([]types.Provider{provider}, cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	models, ok := modelsByProvider["test-provider"]
	if !ok {
		t.Fatal("expected models for test-provider")
	}
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
}

func TestResolveProvidersForModel_AlibabaOpenAICompatibleIncluded(t *testing.T) {
	reg := registry.GetProviderRegistry()
	for _, provider := range reg.ListProviders() {
		_ = reg.RemoveActive(provider.GetInstanceID())
	}

	// Both providers are OpenAI-compatible — both should be candidates.
	provider1 := newAlibabaMockProvider("alibaba-standard", map[string]interface{}{"auth_type": "api-key", "plan": "standard"}, []types.Model{{
		ID: "qwen3.6-plus", Capabilities: map[string]interface{}{"api_modes": []string{alibabapkg.AlibabaAPIModeOpenAICompatible}},
	}})
	provider2 := newAlibabaMockProvider("alibaba-coding", map[string]interface{}{"auth_type": "api-key", "plan": "coding-plan"}, []types.Model{{
		ID: "qwen3.6-plus", Capabilities: map[string]interface{}{"api_modes": []string{alibabapkg.AlibabaAPIModeOpenAICompatible}},
	}})

	if err := reg.Register(provider1, false); err != nil {
		t.Fatalf("register provider1: %v", err)
	}
	if err := reg.Register(provider2, false); err != nil {
		t.Fatalf("register provider2: %v", err)
	}
	if _, err := reg.AddActive(provider1.GetInstanceID()); err != nil {
		t.Fatalf("activate provider1: %v", err)
	}
	if _, err := reg.AddActive(provider2.GetInstanceID()); err != nil {
		t.Fatalf("activate provider2: %v", err)
	}

	cache := NewModelCache()
	route, err := ResolveProvidersForModel("qwen3.6-plus", "qwen3.6-plus", cache)
	if err != nil {
		t.Fatalf("ResolveProvidersForModel() error = %v", err)
	}
	if len(route.CandidateProviders) != 2 {
		t.Fatalf("candidate providers = %d, want 2", len(route.CandidateProviders))
	}
}

type alibabaMockProvider struct {
	mockProvider
	config map[string]interface{}
}

func newAlibabaMockProvider(instanceID string, config map[string]interface{}, models []types.Model) *alibabaMockProvider {
	return &alibabaMockProvider{
		mockProvider: mockProvider{
			instanceID: instanceID,
			name:       instanceID,
			models:     &types.ModelsResponse{Data: models},
		},
		config: config,
	}
}

func (p *alibabaMockProvider) GetID() string { return string(types.ProviderAlibaba) }
func (p *alibabaMockProvider) GetConfig() map[string]interface{} {
	return p.config
}
