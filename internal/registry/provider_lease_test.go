package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"omnillm/internal/database"
)

func TestProviderLeaseBlocksIdentityChange(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "lease-provider", name: "Lease provider"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	got, release, err := reg.BorrowProvider(provider.instanceID)
	if err != nil || got != provider {
		t.Fatalf("borrow provider = %v, %v", got, err)
	}
	if _, err := reg.TryLockIdentity(provider.instanceID); !errors.Is(err, ErrProviderBusy) {
		t.Fatalf("exclusive lock while borrowed = %v, want busy", err)
	}
	release()
	release() // Releasing more than once must not corrupt the lease count.
	unlock, err := reg.TryLockIdentity(provider.instanceID)
	if err != nil {
		t.Fatalf("exclusive lock after release: %v", err)
	}
	if _, _, err := reg.BorrowProvider(provider.instanceID); !errors.Is(err, ErrProviderBusy) {
		t.Fatalf("borrow during exclusive lock = %v, want busy", err)
	}
	unlock()
	unlock()
	_, releaseAgain, err := reg.BorrowProvider(provider.instanceID)
	if err != nil {
		t.Fatalf("borrow after unlock: %v", err)
	}
	releaseAgain()
}

func TestIdentityLeaseTransfersRegistryAndDurableState(t *testing.T) {
	reg := newTestRegistry()
	const oldID = "lease-transfer-old"
	const newID = "lease-transfer-new"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "mock", Name: "Original", Activated: true}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	provider := &testProvider{id: "mock", instanceID: oldID, name: "Original"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.SetActive(oldID); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(newID)
		_ = reg.Remove(oldID)
		reg.WaitForPendingSaves()
		_ = store.Delete(oldID)
		_ = store.Delete(newID)
	})
	lease, err := reg.BeginIdentityChange(oldID)
	if err != nil {
		t.Fatalf("begin identity change: %v", err)
	}
	defer lease.Release()
	replacement := &testProvider{id: "mock", instanceID: newID, name: "Original"}
	if err := lease.Commit(context.Background(), replacement); err != nil {
		t.Fatalf("commit identity change: %v", err)
	}
	if reg.IsRegistered(oldID) || !reg.IsActiveProvider(newID) {
		t.Fatal("registry identity or activation not transferred")
	}
	active, err := reg.GetActive()
	if err != nil || active != replacement {
		t.Fatalf("primary provider = %v, %v", active, err)
	}
	old, err := store.Get(oldID)
	if err != nil || old != nil {
		t.Fatalf("old durable identity: %#v, %v", old, err)
	}
	updated, err := store.Get(newID)
	if err != nil || updated == nil || !updated.Activated {
		t.Fatalf("new durable identity: %#v, %v", updated, err)
	}
}

func TestIdentityChangesDoNotResurrectBackgroundSnapshots(t *testing.T) {
	reg := newTestRegistry()
	store := database.NewProviderInstanceStore()
	current := "lease-stale-0"
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: current, ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := reg.Register(&testProvider{id: "mock", instanceID: current, name: "Mock"}, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i <= 20; i++ {
			id := fmt.Sprintf("lease-stale-%d", i)
			_ = reg.Remove(id)
		}
		reg.WaitForPendingSaves()
		for i := 0; i <= 20; i++ {
			_ = store.Delete(fmt.Sprintf("lease-stale-%d", i))
		}
	})
	for i := 1; i <= 20; i++ {
		oldID := current
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = reg.TouchProvider(oldID)
		}()
		var lease *IdentityLease
		for {
			var err error
			lease, err = reg.BeginIdentityChange(oldID)
			if err == nil {
				break
			}
			if !errors.Is(err, ErrProviderBusy) {
				t.Fatalf("begin identity change: %v", err)
			}
			reg.WaitForPendingSaves()
		}
		current = fmt.Sprintf("lease-stale-%d", i)
		if err := lease.Commit(context.Background(), &testProvider{id: "mock", instanceID: current, name: "Mock"}); err != nil {
			lease.Release()
			t.Fatalf("commit identity change: %v", err)
		}
		lease.Release()
		<-done
		reg.WaitForPendingSaves()
		old, err := store.Get(oldID)
		if err != nil || old != nil {
			t.Fatalf("background save restored old ID %q: %#v, %v", oldID, old, err)
		}
	}
}

func TestProviderLeasesAreIndependent(t *testing.T) {
	reg := newTestRegistry()
	for _, id := range []string{"lease-first", "lease-second"} {
		if err := reg.Register(&testProvider{id: "mock", instanceID: id, name: id}, false); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	_, release, err := reg.BorrowProvider("lease-first")
	if err != nil {
		t.Fatalf("borrow first: %v", err)
	}
	defer release()
	unlock, err := reg.TryLockIdentity("lease-second")
	if err != nil {
		t.Fatalf("exclusive lock for unrelated provider: %v", err)
	}
	unlock()
}

func TestProviderLeasePreventsReplacementAndRemoval(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "lease-mutation", name: "original"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	_, release, err := reg.BorrowProvider(provider.instanceID)
	if err != nil {
		t.Fatalf("borrow provider: %v", err)
	}
	replacement := &testProvider{id: "mock", instanceID: provider.instanceID, name: "replacement"}
	if err := reg.Register(replacement, false); !errors.Is(err, ErrProviderBusy) {
		t.Fatalf("replace borrowed provider = %v, want busy", err)
	}
	if err := reg.Remove(provider.instanceID); !errors.Is(err, ErrProviderBusy) {
		t.Fatalf("remove borrowed provider = %v, want busy", err)
	}
	if got, err := reg.GetProvider(provider.instanceID); err != nil || got != provider {
		t.Fatalf("original provider changed: %v, %v", got, err)
	}
	release()
	if err := reg.Register(replacement, false); err != nil {
		t.Fatalf("replace released provider: %v", err)
	}
}

func TestRegisterNewDoesNotReplaceExistingProvider(t *testing.T) {
	reg := newTestRegistry()
	first := &testProvider{id: "mock", instanceID: "unique-register", name: "original"}
	second := &testProvider{id: "mock", instanceID: "unique-register", name: "replacement"}
	if err := reg.RegisterNew(first, false); err != nil {
		t.Fatalf("register first provider: %v", err)
	}
	if err := reg.RegisterNew(second, false); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
	if got, err := reg.GetProvider(first.instanceID); err != nil || got != first {
		t.Fatalf("existing provider overwritten: %v, %v", got, err)
	}
}

func TestTouchProviderWhileBorrowedDoesNotReplaceIdentity(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "lease-touch", name: "touch"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	_, release, err := reg.BorrowProvider(provider.instanceID)
	if err != nil {
		t.Fatalf("borrow provider: %v", err)
	}
	if err := reg.TouchProvider(provider.instanceID); err != nil {
		t.Fatalf("touch borrowed provider: %v", err)
	}
	release()
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(provider.instanceID)
		reg.WaitForPendingSaves()
		_ = reg.instanceStore.Delete(provider.instanceID)
	})
}

func TestProviderLeaseConcurrentBorrowAndExclusiveLock(t *testing.T) {
	reg := newTestRegistry()
	const id = "lease-race"
	if err := reg.Register(&testProvider{id: "mock", instanceID: id, name: id}, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_, release, err := reg.BorrowProvider(id)
				if err == nil {
					release()
				} else if !errors.Is(err, ErrProviderBusy) {
					t.Errorf("borrow provider: %v", err)
				}
				unlock, err := reg.TryLockIdentity(id)
				if err == nil {
					unlock()
				} else if !errors.Is(err, ErrProviderBusy) {
					t.Errorf("lock provider: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestBorrowActiveProvidersProtectsEveryCandidate(t *testing.T) {
	reg := newTestRegistry()
	for _, id := range []string{"lease-active-a", "lease-active-b"} {
		if err := reg.Register(&testProvider{id: "mock", instanceID: id, name: id}, false); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		if _, err := reg.AddActive(id); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		for _, id := range []string{"lease-active-a", "lease-active-b"} {
			_ = reg.Remove(id)
		}
		reg.WaitForPendingSaves()
		for _, id := range []string{"lease-active-a", "lease-active-b"} {
			_ = reg.instanceStore.Delete(id)
		}
	})

	providers, release, err := reg.BorrowActiveProviders()
	if err != nil || len(providers) != 2 {
		t.Fatalf("borrow active providers: %d, %v", len(providers), err)
	}
	for _, provider := range providers {
		if _, err := reg.TryLockIdentity(provider.GetInstanceID()); !errors.Is(err, ErrProviderBusy) {
			t.Fatalf("exclusive lock during borrowed candidates = %v, want busy", err)
		}
	}
	release()
	for _, provider := range providers {
		unlock, err := reg.TryLockIdentity(provider.GetInstanceID())
		if err != nil {
			t.Fatalf("exclusive lock after candidates released: %v", err)
		}
		unlock()
	}
}

func TestBorrowProvidersIncludesInactiveInstances(t *testing.T) {
	reg := newTestRegistry()
	for _, id := range []string{"lease-list-a", "lease-list-b"} {
		if err := reg.Register(&testProvider{id: "mock", instanceID: id, name: id}, false); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	providers, release, err := reg.BorrowProviders()
	if err != nil || len(providers) != 2 {
		t.Fatalf("borrow providers: %d, %v", len(providers), err)
	}
	defer release()
	for _, provider := range providers {
		if _, err := reg.TryLockIdentity(provider.GetInstanceID()); !errors.Is(err, ErrProviderBusy) {
			t.Fatalf("exclusive lock during listing = %v, want busy", err)
		}
	}
}

func TestBorrowPrimaryProviderProtectsItsIdentity(t *testing.T) {
	reg := newTestRegistry()
	provider := &testProvider{id: "mock", instanceID: "lease-primary", name: "primary"}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.SetActive(provider.instanceID); err != nil {
		t.Fatalf("set primary: %v", err)
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(provider.instanceID)
		reg.WaitForPendingSaves()
		_ = reg.instanceStore.Delete(provider.instanceID)
	})
	got, release, err := reg.BorrowActiveProvider()
	if err != nil || got != provider {
		t.Fatalf("borrow primary = %v, %v", got, err)
	}
	if _, err := reg.TryLockIdentity(provider.instanceID); !errors.Is(err, ErrProviderBusy) {
		t.Fatalf("exclusive lock during primary borrow = %v, want busy", err)
	}
	release()
}
