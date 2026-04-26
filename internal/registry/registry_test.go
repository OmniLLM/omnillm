package registry

import (
	"omnillm/internal/database"
	"os"
	"testing"

	providertypes "omnillm/internal/providers/types"
)

type testProvider struct {
	id         string
	instanceID string
	name       string
}

func (p *testProvider) GetID() string                                { return p.id }
func (p *testProvider) GetInstanceID() string                        { return p.instanceID }
func (p *testProvider) GetName() string                              { return p.name }
func (p *testProvider) SetName(name string)                          { p.name = name }
func (p *testProvider) SetupAuth(_ *providertypes.AuthOptions) error { return nil }
func (p *testProvider) GetToken() string                             { return "" }
func (p *testProvider) RefreshToken() error                          { return nil }
func (p *testProvider) GetBaseURL() string                           { return "" }
func (p *testProvider) GetHeaders(_ bool) map[string]string          { return nil }
func (p *testProvider) GetModels() (*providertypes.ModelsResponse, error) {
	return &providertypes.ModelsResponse{}, nil
}

func (p *testProvider) CreateChatCompletions(_ map[string]any) (map[string]any, error) {
	return nil, nil
}
func (p *testProvider) CreateEmbeddings(_ map[string]any) (map[string]any, error) { return nil, nil }
func (p *testProvider) GetUsage() (map[string]any, error)                         { return nil, nil }
func (p *testProvider) GetAdapter() providertypes.ProviderAdapter                 { return nil }

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := database.InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func newTestRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:       make(map[string]providertypes.Provider),
		activeProviders: make(map[string]struct{}),
		configStore:     database.NewConfigStore(),
		instanceStore:   database.NewProviderInstanceStore(),
	}
}

func TestRegisterAndGetProvider(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "mock-1", name: "Mock 1"}

	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	got, err := reg.GetProvider("mock-1")
	if err != nil {
		t.Fatalf("get provider failed: %v", err)
	}
	if got.GetInstanceID() != "mock-1" {
		t.Fatalf("expected mock-1, got %s", got.GetInstanceID())
	}
}

func TestSetActiveAndRemoveActivePromotesAnotherProvider(t *testing.T) {
	reg := newTestRegistry()
	first := &testProvider{id: "mock", instanceID: "mock-1", name: "Mock 1"}
	second := &testProvider{id: "mock", instanceID: "mock-2", name: "Mock 2"}
	if err := reg.Register(first, false); err != nil {
		t.Fatalf("register first failed: %v", err)
	}
	if err := reg.Register(second, false); err != nil {
		t.Fatalf("register second failed: %v", err)
	}

	if _, err := reg.AddActive("mock-1"); err != nil {
		t.Fatalf("add active first failed: %v", err)
	}
	if _, err := reg.AddActive("mock-2"); err != nil {
		t.Fatalf("add active second failed: %v", err)
	}
	if err := reg.RemoveActive("mock-1"); err != nil {
		t.Fatalf("remove active failed: %v", err)
	}

	active, err := reg.GetActive()
	if err != nil {
		t.Fatalf("get active failed: %v", err)
	}
	if active.GetInstanceID() != "mock-2" {
		t.Fatalf("expected mock-2 to become active, got %s", active.GetInstanceID())
	}
}

func TestRenameMovesProviderAndActiveState(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "mock-1", name: "Mock 1"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := reg.AddActive("mock-1"); err != nil {
		t.Fatalf("add active failed: %v", err)
	}
	provider.instanceID = "mock-renamed"

	if err := reg.Rename("mock-1", "mock-renamed"); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if reg.IsRegistered("mock-1") {
		t.Fatal("expected old instance id to be removed")
	}
	if !reg.IsRegistered("mock-renamed") {
		t.Fatal("expected new instance id to be registered")
	}
	if !reg.IsActiveProvider("mock-renamed") {
		t.Fatal("expected renamed provider to remain active")
	}
}

func TestRemoveDeletesProviderAndToken(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "mock-1", name: "Mock 1"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := reg.AddActive("mock-1"); err != nil {
		t.Fatalf("add active failed: %v", err)
	}

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save("mock-1", "mock", map[string]string{"token": "secret"}); err != nil {
		t.Fatalf("save token failed: %v", err)
	}

	if err := reg.Remove("mock-1"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if reg.IsRegistered("mock-1") {
		t.Fatal("expected provider to be removed")
	}
	if reg.IsActiveProvider("mock-1") {
		t.Fatal("expected provider to be removed from active set")
	}
	token, err := tokenStore.Get("mock-1")
	if err != nil {
		t.Fatalf("get token failed: %v", err)
	}
	if token != nil {
		t.Fatal("expected token to be deleted")
	}
}

func TestNextInstanceIDSkipsExistingSuffixes(t *testing.T) {
	reg := newTestRegistry()
	if err := reg.Register(&testProvider{id: "mock", instanceID: "mock", name: "Mock"}, false); err != nil {
		t.Fatalf("register base instance failed: %v", err)
	}
	if err := reg.Register(&testProvider{id: "mock", instanceID: "mock-2", name: "Mock 2"}, false); err != nil {
		t.Fatalf("register suffixed instance failed: %v", err)
	}

	if got := reg.NextInstanceID("mock"); got != "mock-3" {
		t.Fatalf("expected mock-3, got %s", got)
	}
}

func TestGetProviderMapReturnsCopy(t *testing.T) {
	reg := newTestRegistry()
	if err := reg.Register(&testProvider{id: "mock", instanceID: "mock-1", name: "Mock"}, false); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	providers := reg.GetProviderMap()
	delete(providers, "mock-1")

	if !reg.IsRegistered("mock-1") {
		t.Fatal("expected registry state to be isolated from returned map")
	}
}
