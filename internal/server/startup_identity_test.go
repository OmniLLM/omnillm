package server

import (
	"os"
	"os/exec"
	"testing"

	"github.com/google/uuid"

	"omnillm/internal/database"
	"omnillm/internal/registry"
)

func TestDefaultProviderUsesUUID(t *testing.T) {
	const childEnv = "OMNILLM_TEST_DEFAULT_PROVIDER_CHILD"
	if os.Getenv(childEnv) == "" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatalf("test executable: %v", err)
		}
		cmd := exec.Command(binary, "-test.run=^TestDefaultProviderUsesUUID$", "-test.count=1")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated startup test: %v\n%s", err, output)
		}
		return
	}

	reg := registry.GetProviderRegistry()
	if err := registerDefaultProviders(reg, StartOptions{}); err != nil {
		t.Fatalf("register default provider: %v", err)
	}
	reg.WaitForPendingSaves()
	providers := reg.ListProviders()
	if len(providers) != 1 {
		t.Fatalf("default providers = %d, want 1", len(providers))
	}
	id := providers[0].GetInstanceID()
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 4 || parsed.String() != id {
		t.Fatalf("default provider ID %q is not a canonical UUID v4", id)
	}
	record, err := database.NewProviderInstanceStore().Get(id)
	if err != nil || record == nil || !record.Activated {
		t.Fatalf("default provider record: %#v, %v", record, err)
	}
}

func TestStartupPreservesLegacyProviderID(t *testing.T) {
	const childEnv = "OMNILLM_TEST_LEGACY_PROVIDER_CHILD"
	if os.Getenv(childEnv) == "" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatalf("test executable: %v", err)
		}
		cmd := exec.Command(binary, "-test.run=^TestStartupPreservesLegacyProviderID$", "-test.count=1")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated startup test: %v\n%s", err, output)
		}
		return
	}
	const id = "alibaba-legacy"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "alibaba", Name: "Legacy", Activated: true}); err != nil {
		t.Fatalf("save legacy provider: %v", err)
	}
	if err := database.NewTokenStore().Save(id, map[string]string{"access_token": "legacy-key"}); err != nil {
		t.Fatalf("save legacy credentials: %v", err)
	}
	reg := registry.GetProviderRegistry()
	if err := registerDefaultProviders(reg, StartOptions{}); err != nil {
		t.Fatalf("load providers: %v", err)
	}
	reg.WaitForPendingSaves()
	providers := reg.ListProviders()
	if len(providers) != 1 || providers[0].GetInstanceID() != id || providers[0].GetToken() != "legacy-key" {
		t.Fatalf("legacy provider did not retain identity and credentials: %#v", providers)
	}
}
