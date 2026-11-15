package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"omnillm/internal/lib/catalogstate"
	"omnillm/internal/providers/types"
)

// ErrProviderBusy means an identity change or provider operation is in progress.
var ErrProviderBusy = errors.New("provider is busy")

// ErrProviderIdentityConflict means the target identity is already registered.
var ErrProviderIdentityConflict = errors.New("provider identity already exists")

// providerLifecycle is protected by ProviderRegistry.mu. Its address remains
// stable for the lifetime of a registered provider, even if its ID changes.
type providerLifecycle struct {
	borrowers int
	exclusive bool
}

// IdentityLease holds exclusive access to one provider lifecycle.
type IdentityLease struct {
	mu        sync.Mutex
	registry  *ProviderRegistry
	lifecycle *providerLifecycle
	id        string
	released  bool
	committed bool
}

func (pr *ProviderRegistry) lifecycleLocked(id string) *providerLifecycle {
	if pr.lifecycles == nil {
		pr.lifecycles = make(map[string]*providerLifecycle)
	}
	if pr.lifecycles[id] == nil {
		pr.lifecycles[id] = &providerLifecycle{}
	}
	return pr.lifecycles[id]
}

// BorrowProvider returns a provider and a release function that protects its
// identity for the duration of the caller's work.
func (pr *ProviderRegistry) BorrowProvider(id string) (types.Provider, func(), error) {
	pr.mu.Lock()
	provider, exists := pr.providers[id]
	if !exists {
		pr.mu.Unlock()
		return nil, nil, fmt.Errorf("provider %q not found", id)
	}
	lifecycle := pr.lifecycleLocked(id)
	if lifecycle.exclusive {
		pr.mu.Unlock()
		return nil, nil, ErrProviderBusy
	}
	lifecycle.borrowers++
	pr.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			pr.mu.Lock()
			lifecycle.borrowers--
			pr.mu.Unlock()
		})
	}
	return provider, release, nil
}

// BorrowActiveProviders atomically protects all currently active candidates.
func (pr *ProviderRegistry) BorrowActiveProviders() ([]types.Provider, func(), error) {
	return pr.borrowProviders(true)
}

// BorrowProviders atomically protects all currently registered providers.
func (pr *ProviderRegistry) BorrowProviders() ([]types.Provider, func(), error) {
	return pr.borrowProviders(false)
}

func (pr *ProviderRegistry) borrowProviders(activeOnly bool) ([]types.Provider, func(), error) {
	pr.mu.Lock()
	providers := make([]types.Provider, 0, len(pr.providers))
	lifecycles := make([]*providerLifecycle, 0, len(pr.providers))
	for id, provider := range pr.providers {
		if _, isActive := pr.activeProviders[id]; activeOnly && !isActive {
			continue
		}
		lifecycle := pr.lifecycleLocked(id)
		if lifecycle.exclusive {
			pr.mu.Unlock()
			return nil, nil, ErrProviderBusy
		}
		providers = append(providers, provider)
		lifecycles = append(lifecycles, lifecycle)
	}
	for _, lifecycle := range lifecycles {
		lifecycle.borrowers++
	}
	pr.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			pr.mu.Lock()
			for _, lifecycle := range lifecycles {
				lifecycle.borrowers--
			}
			pr.mu.Unlock()
		})
	}
	return providers, release, nil
}

// BorrowActiveProvider atomically protects the currently selected primary provider.
func (pr *ProviderRegistry) BorrowActiveProvider() (types.Provider, func(), error) {
	pr.mu.Lock()
	provider := pr.activeProvider
	if provider == nil {
		pr.mu.Unlock()
		return nil, nil, errors.New("no active provider set")
	}
	id := provider.GetInstanceID()
	provider, exists := pr.providers[id]
	if !exists {
		pr.mu.Unlock()
		return nil, nil, fmt.Errorf("active provider %q is no longer registered", id)
	}
	lifecycle := pr.lifecycleLocked(id)
	if lifecycle.exclusive {
		pr.mu.Unlock()
		return nil, nil, ErrProviderBusy
	}
	lifecycle.borrowers++
	pr.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			pr.mu.Lock()
			lifecycle.borrowers--
			pr.mu.Unlock()
		})
	}
	return provider, release, nil
}

// TryLockIdentity excludes provider operations without waiting for in-flight work.
func (pr *ProviderRegistry) TryLockIdentity(id string) (func(), error) {
	lease, err := pr.BeginIdentityChange(id)
	if err != nil {
		return nil, err
	}
	return lease.Release, nil
}

// BeginIdentityChange excludes provider work without waiting for in-flight work.
func (pr *ProviderRegistry) BeginIdentityChange(id string) (*IdentityLease, error) {
	pr.mu.Lock()
	if _, exists := pr.providers[id]; !exists {
		pr.mu.Unlock()
		return nil, fmt.Errorf("provider %q not found", id)
	}
	lifecycle := pr.lifecycleLocked(id)
	if lifecycle.exclusive || lifecycle.borrowers != 0 {
		pr.mu.Unlock()
		return nil, ErrProviderBusy
	}
	lifecycle.exclusive = true
	pr.mu.Unlock()
	return &IdentityLease{registry: pr, lifecycle: lifecycle, id: id}, nil
}

// Release ends an identity change. It is safe to call more than once.
func (lease *IdentityLease) Release() {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.released {
		return
	}
	lease.registry.mu.Lock()
	lease.lifecycle.exclusive = false
	lease.registry.mu.Unlock()
	lease.released = true
}

// Commit atomically transfers durable state and publishes a prepared provider.
func (lease *IdentityLease) Commit(ctx context.Context, replacement types.Provider) error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.released || lease.committed {
		return errors.New("identity lease is no longer available")
	}
	if replacement == nil {
		return errors.New("replacement provider is required")
	}
	newID := replacement.GetInstanceID()
	pr := lease.registry
	pr.mu.Lock()
	defer pr.mu.Unlock()
	oldProvider, exists := pr.providers[lease.id]
	if !exists || pr.lifecycles[lease.id] != lease.lifecycle || !lease.lifecycle.exclusive {
		return errors.New("provider lifecycle changed")
	}
	if newID == "" || newID == lease.id || replacement.GetID() != oldProvider.GetID() {
		return errors.New("invalid replacement provider identity")
	}
	if _, exists := pr.providers[newID]; exists {
		return fmt.Errorf("%w: %q", ErrProviderIdentityConflict, newID)
	}
	if err := pr.instanceStore.RenameIdentity(ctx, lease.id, newID); err != nil {
		return err
	}
	oldID := lease.id
	delete(pr.providers, oldID)
	delete(pr.lifecycles, oldID)
	pr.providers[newID] = replacement
	pr.lifecycles[newID] = lease.lifecycle
	if _, active := pr.activeProviders[oldID]; active {
		delete(pr.activeProviders, oldID)
		pr.activeProviders[newID] = struct{}{}
	}
	if pr.activeProvider != nil && pr.activeProvider.GetInstanceID() == oldID {
		pr.activeProvider = replacement
	}
	catalogstate.Invalidate(oldID)
	catalogstate.Invalidate(newID)
	lease.id = newID
	lease.committed = true
	return nil
}
