package routes

import (
	"omnillm/internal/database"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"testing"
)

type refreshCatalogProvider struct {
	types.Provider
	id     string
	models *types.ModelsResponse
	calls  int
}

func (p *refreshCatalogProvider) GetInstanceID() string { return p.id }
func (p *refreshCatalogProvider) GetName() string       { return p.id }
func (p *refreshCatalogProvider) GetID() string         { return "catalog-test" }
func (p *refreshCatalogProvider) GetModels() (*types.ModelsResponse, error) {
	p.calls++
	return p.models, nil
}

func TestAdminRefreshRepairsGenerationCatalog(t *testing.T) {
	p := &refreshCatalogProvider{id: "catalog-admin", models: &types.ModelsResponse{Data: []types.Model{{ID: "old"}}}}
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.id, ProviderID: p.GetID(), Name: p.id}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.NewProviderInstanceStore().Delete(p.id) })
	_, _ = modelrouting.GetCachedOrFetchModels(p, modelCache)
	p.models = &types.ModelsResponse{Data: []types.Model{{ID: "gpt-6-astra"}}}
	models, err := loadProviderModels(p, true)
	if err != nil || len(models) != 1 || models[0].ID != "gpt-6-astra" {
		t.Fatalf("refresh: %+v %v", models, err)
	}
	got, err := modelrouting.GetCachedOrFetchModels(p, modelCache)
	if err != nil || got.Data[0].ID != "gpt-6-astra" || p.calls != 2 {
		t.Fatalf("routing: %+v %v calls=%d", got, err, p.calls)
	}
	before, _ := database.NewProviderModelsCacheStore().Get(p.id, p.GetID(), database.DefaultCacheTTL)
	p.models = &types.ModelsResponse{Data: []types.Model{{ID: "fallback"}}, Degraded: true}
	if _, err := loadProviderModels(p, true); err == nil {
		t.Fatal("degraded refresh claimed fresh discovery")
	}
	after, _ := database.NewProviderModelsCacheStore().Get(p.id, p.GetID(), database.DefaultCacheTTL)
	if before == nil || after == nil || before.ModelsData != after.ModelsData {
		t.Fatal("degraded discovery overwrote persisted success")
	}
}

func TestCatalogLifecycleConfigAndTokens(t *testing.T) {
	t.Cleanup(func() {
		reg := registry.GetProviderRegistry()
		_ = reg.Remove("catalog-mutations")
		reg.WaitForPendingSaves()
	})
	p := &refreshCatalogProvider{id: "catalog-mutations", models: &types.ModelsResponse{Data: []types.Model{{ID: "old"}}}}
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.id, ProviderID: p.GetID(), Name: p.id}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.NewProviderInstanceStore().Delete(p.id) })
	for _, mutate := range []func() error{
		func() error {
			return database.NewProviderConfigStore().Save(p.id, map[string]interface{}{"models": []string{"new"}})
		},
		func() error {
			return database.NewTokenStore().Save(p.id, map[string]interface{}{"token": "new-account"})
		},
		func() error { return registry.GetProviderRegistry().Register(p, false) },
	} {
		cache := modelrouting.NewModelCache()
		p.models = &types.ModelsResponse{Data: []types.Model{{ID: "old"}}}
		_, _ = modelrouting.GetCachedOrFetchModels(p, cache)
		p.models = &types.ModelsResponse{Data: []types.Model{{ID: "new"}}}
		if err := mutate(); err != nil {
			t.Fatal(err)
		}
		got, err := modelrouting.GetCachedOrFetchModels(p, cache)
		if err != nil || got.Data[0].ID != "new" {
			t.Fatalf("mutation retained old: %+v %v", got, err)
		}
	}
}
