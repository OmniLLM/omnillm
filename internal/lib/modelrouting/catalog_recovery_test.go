package modelrouting

import (
	"context"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"testing"
)

func TestCatalogReplacementDoesNotInheritModels(t *testing.T) {
	id := "catalog-replacement"
	old := &mockProvider{instanceID: id, models: &types.ModelsResponse{Data: []types.Model{{ID: "old"}}}}
	current := &mockProvider{instanceID: id, models: &types.ModelsResponse{Data: []types.Model{{ID: "gpt-6-astra"}}}}
	reg := registry.GetProviderRegistry()
	cache := NewModelCache()
	_ = reg.Register(old, false)
	_, _ = GetCachedOrFetchModels(old, cache)
	_ = reg.Register(current, false)
	got, err := GetCachedOrFetchModels(current, cache)
	if err != nil || len(got.Data) != 1 || got.Data[0].ID != "gpt-6-astra" {
		t.Fatalf("replacement catalog = %+v, error = %v", got, err)
	}
}

func TestIdentityTransferIsolatesOldAndNewCatalogOwners(t *testing.T) {
	const oldID = "identity-catalog-old"
	const newID = "identity-catalog-new"
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	old := &mockProvider{instanceID: oldID, name: "Mock", models: &types.ModelsResponse{Data: []types.Model{{ID: "old-model"}}}}
	if err := reg.Register(old, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	t.Cleanup(func() {
		_ = reg.Remove(oldID)
		_ = reg.Remove(newID)
		reg.WaitForPendingSaves()
		_ = store.Delete(oldID)
		_ = store.Delete(newID)
	})
	cache := NewModelCache()
	if _, err := GetCachedOrFetchModels(old, cache); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	lease, err := reg.BeginIdentityChange(oldID)
	if err != nil {
		t.Fatalf("begin identity transfer: %v", err)
	}
	replacement := &mockProvider{instanceID: newID, name: "Mock", models: old.models}
	if err := lease.Commit(context.Background(), replacement); err != nil {
		lease.Release()
		t.Fatalf("commit identity transfer: %v", err)
	}
	lease.Release()
	newOwner := &mockProvider{instanceID: oldID, name: "New owner", models: &types.ModelsResponse{Data: []types.Model{{ID: "new-model"}}}}
	if err := reg.Register(newOwner, false); err != nil {
		t.Fatalf("register old ID owner: %v", err)
	}
	got, err := GetCachedOrFetchModels(newOwner, cache)
	if err != nil || len(got.Data) != 1 || got.Data[0].ID != "new-model" {
		t.Fatalf("old ID inherited stale catalog: %#v, %v", got, err)
	}
	got, err = GetCachedOrFetchModels(replacement, cache)
	if err != nil || len(got.Data) != 1 || got.Data[0].ID != "old-model" || replacement.fetchCount != 1 {
		t.Fatalf("new ID reused stale cache: %#v, %v", got, err)
	}
}

func TestIdentityTransferPreservesDisabledModelRouting(t *testing.T) {
	const oldID = "identity-disabled-old"
	const newID = "identity-disabled-new"
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := database.NewModelStateStore().SetEnabled(oldID, "disabled-model", false); err != nil {
		t.Fatalf("disable model: %v", err)
	}
	old := &mockProvider{instanceID: oldID, name: "Mock", models: &types.ModelsResponse{Data: []types.Model{{ID: "disabled-model"}}}}
	if err := reg.Register(old, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.AddActive(oldID); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(oldID)
		_ = reg.Remove(newID)
		reg.WaitForPendingSaves()
		_ = store.Delete(oldID)
		_ = store.Delete(newID)
	})
	lease, err := reg.BeginIdentityChange(oldID)
	if err != nil {
		t.Fatalf("begin identity transfer: %v", err)
	}
	replacement := &mockProvider{instanceID: newID, name: "Mock", models: old.models}
	if err := lease.Commit(context.Background(), replacement); err != nil {
		lease.Release()
		t.Fatalf("commit identity transfer: %v", err)
	}
	lease.Release()
	providers, release, err := reg.BorrowActiveProviders()
	if err != nil {
		t.Fatalf("borrow active providers: %v", err)
	}
	defer release()
	resolved, err := ProviderResolver(providers)("disabled-model", "disabled-model", "", NewModelCache())
	if err != nil || len(resolved.CandidateProviders) != 0 {
		t.Fatalf("disabled model became routable: %#v, %v", resolved, err)
	}
}

func TestEmptyCatalogCanRecover(t *testing.T) {
	p := &mockProvider{instanceID: "catalog-empty", models: &types.ModelsResponse{Object: "list"}}
	cache := NewModelCache()
	_, _ = GetCachedOrFetchModels(p, cache)
	p.models = &types.ModelsResponse{Data: []types.Model{{ID: "gpt-6-astra"}}}
	// A lifecycle change must bypass even a failure backoff.
	_ = registry.GetProviderRegistry().Register(p, false)
	got, err := GetCachedOrFetchModels(p, cache)
	if err != nil || len(got.Data) != 1 {
		t.Fatalf("recovered catalog = %+v, error = %v", got, err)
	}
}

func TestCatalogRenameDeleteAndRecreation(t *testing.T) {
	reg := registry.GetProviderRegistry()
	p := &mockProvider{instanceID: "catalog-old-id", models: &types.ModelsResponse{Data: []types.Model{{ID: "old"}}}}
	cache := NewModelCache()
	_ = reg.Register(p, false)
	_, _ = GetCachedOrFetchModels(p, cache)
	if err := reg.Rename(p.instanceID, "catalog-new-id"); err != nil {
		t.Fatal(err)
	}
	replacement := &mockProvider{instanceID: "catalog-old-id", models: &types.ModelsResponse{Data: []types.Model{{ID: "astra"}}}}
	_ = reg.Register(replacement, false)
	got, err := GetCachedOrFetchModels(replacement, cache)
	if err != nil || got.Data[0].ID != "astra" {
		t.Fatalf("renamed owner inherited %+v %v", got, err)
	}
	if err := reg.Remove(replacement.instanceID); err != nil {
		t.Fatal(err)
	}
	replacement.models = &types.ModelsResponse{Data: []types.Model{{ID: "recreated"}}}
	_ = reg.Register(replacement, false)
	got, err = GetCachedOrFetchModels(replacement, cache)
	if err != nil || got.Data[0].ID != "recreated" {
		t.Fatalf("recreated owner inherited %+v %v", got, err)
	}
	reg.WaitForPendingSaves()
}
