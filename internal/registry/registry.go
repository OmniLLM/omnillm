// Package registry provides a centralized provider registry system
package registry

import (
	"fmt"
	"omnillm/internal/database"
	"omnillm/internal/lib/catalogstate"
	"omnillm/internal/providers/types"
	"sync"

	"github.com/rs/zerolog/log"
)

type ProviderRegistry struct {
	mu              sync.RWMutex
	saveMu          sync.Mutex
	persistenceMu   sync.Mutex
	saveWG          sync.WaitGroup
	providers       map[string]types.Provider
	lifecycles      map[string]*providerLifecycle
	activeProvider  types.Provider
	activeProviders map[string]struct{}
	instanceStore   *database.ProviderInstanceStore
}

func (pr *ProviderRegistry) saveConfigInBackground() {
	pr.saveMu.Lock()
	pr.saveWG.Add(1)
	go func() {
		defer pr.saveWG.Done()
		_ = pr.saveConfigAsync()
	}()
	pr.saveMu.Unlock()
}

// WaitForPendingSaves waits for provider persistence already scheduled by the
// registry. It is primarily useful during orderly shutdown and test cleanup.
func (pr *ProviderRegistry) WaitForPendingSaves() {
	pr.saveMu.Lock()
	defer pr.saveMu.Unlock()
	pr.saveWG.Wait()
}

var (
	globalRegistry *ProviderRegistry
	once           sync.Once
)

func GetProviderRegistry() *ProviderRegistry {
	once.Do(func() {
		globalRegistry = &ProviderRegistry{
			providers:       make(map[string]types.Provider),
			activeProviders: make(map[string]struct{}),
			instanceStore:   database.NewProviderInstanceStore(),
		}
	})
	return globalRegistry
}

func (pr *ProviderRegistry) Register(provider types.Provider, saveConfig bool) error {
	return pr.register(provider, saveConfig, false)
}

// RegisterNew registers a new instance without replacing an existing provider.
func (pr *ProviderRegistry) RegisterNew(provider types.Provider, saveConfig bool) error {
	return pr.register(provider, saveConfig, true)
}

func (pr *ProviderRegistry) register(provider types.Provider, saveConfig, requireNew bool) error {
	pr.mu.Lock()
	id := provider.GetInstanceID()
	if _, exists := pr.providers[id]; exists {
		if requireNew {
			pr.mu.Unlock()
			return fmt.Errorf("provider %q already exists", id)
		}
		lifecycle := pr.lifecycleLocked(id)
		if lifecycle.exclusive || lifecycle.borrowers != 0 {
			pr.mu.Unlock()
			return ErrProviderBusy
		}
	}
	pr.lifecycleLocked(id)
	pr.lifecycles[id] = &providerLifecycle{}
	catalogstate.Invalidate(id)
	pr.providers[id] = provider
	shouldSave := saveConfig
	pr.mu.Unlock()

	if shouldSave {
		pr.saveConfigInBackground()
	}

	log.Debug().Str("provider", provider.GetInstanceID()).Msg("Provider registered")
	return nil
}

func (pr *ProviderRegistry) GetProvider(instanceID string) (types.Provider, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	provider, exists := pr.providers[instanceID]
	if !exists {
		available := make([]string, 0, len(pr.providers))
		for id := range pr.providers {
			available = append(available, id)
		}
		return nil, fmt.Errorf("provider '%s' not found. Available: %v", instanceID, available)
	}
	return provider, nil
}

// TouchProvider invalidates provider caches and schedules metadata persistence
// after updating an existing provider without replacing its lifecycle.
func (pr *ProviderRegistry) TouchProvider(instanceID string) error {
	pr.mu.Lock()
	if _, exists := pr.providers[instanceID]; !exists {
		pr.mu.Unlock()
		return fmt.Errorf("provider %q not found", instanceID)
	}
	if pr.lifecycleLocked(instanceID).exclusive {
		pr.mu.Unlock()
		return ErrProviderBusy
	}
	catalogstate.Invalidate(instanceID)
	pr.mu.Unlock()
	pr.saveConfigInBackground()
	return nil
}

func (pr *ProviderRegistry) SetActive(instanceID string) (types.Provider, error) {
	pr.mu.Lock()
	provider, exists := pr.providers[instanceID]
	if !exists {
		pr.mu.Unlock()
		return nil, fmt.Errorf("provider '%s' not found", instanceID)
	}
	if pr.lifecycleLocked(instanceID).exclusive {
		pr.mu.Unlock()
		return nil, ErrProviderBusy
	}

	pr.activeProvider = provider
	pr.activeProviders[instanceID] = struct{}{}
	pr.mu.Unlock()

	pr.saveConfigInBackground()

	log.Debug().Str("provider", instanceID).Msg("Provider set as active")
	return provider, nil
}

func (pr *ProviderRegistry) GetActive() (types.Provider, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	if pr.activeProvider == nil {
		return nil, fmt.Errorf("no active provider set")
	}
	return pr.activeProvider, nil
}

func (pr *ProviderRegistry) AddActive(instanceID string) (types.Provider, error) {
	pr.mu.Lock()
	provider, exists := pr.providers[instanceID]
	if !exists {
		pr.mu.Unlock()
		return nil, fmt.Errorf("provider '%s' not found", instanceID)
	}
	if pr.lifecycleLocked(instanceID).exclusive {
		pr.mu.Unlock()
		return nil, ErrProviderBusy
	}

	pr.activeProviders[instanceID] = struct{}{}
	if pr.activeProvider == nil {
		pr.activeProvider = provider
	}
	pr.mu.Unlock()

	pr.saveConfigInBackground()

	return provider, nil
}

func (pr *ProviderRegistry) RemoveActive(instanceID string) error {
	pr.mu.Lock()
	if _, exists := pr.providers[instanceID]; exists && pr.lifecycleLocked(instanceID).exclusive {
		pr.mu.Unlock()
		return ErrProviderBusy
	}
	delete(pr.activeProviders, instanceID)
	if pr.activeProvider != nil && pr.activeProvider.GetInstanceID() == instanceID {
		// Pick another active provider as the primary, or nil
		pr.activeProvider = nil
		for id := range pr.activeProviders {
			if provider, exists := pr.providers[id]; exists {
				pr.activeProvider = provider
				break
			}
		}
	}
	pr.mu.Unlock()

	pr.saveConfigInBackground()

	return nil
}

func (pr *ProviderRegistry) GetActiveProviders() []types.Provider {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var providers []types.Provider
	for id := range pr.activeProviders {
		if provider, exists := pr.providers[id]; exists {
			providers = append(providers, provider)
		}
	}
	return providers
}

func (pr *ProviderRegistry) IsActiveProvider(instanceID string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	_, exists := pr.activeProviders[instanceID]
	return exists
}

func (pr *ProviderRegistry) ListProviders() []types.Provider {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	providers := make([]types.Provider, 0, len(pr.providers))
	for _, provider := range pr.providers {
		providers = append(providers, provider)
	}
	return providers
}

func (pr *ProviderRegistry) IsRegistered(instanceID string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	_, exists := pr.providers[instanceID]
	return exists
}

func (pr *ProviderRegistry) GetProviderMap() map[string]types.Provider {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make(map[string]types.Provider)
	for k, v := range pr.providers {
		result[k] = v
	}
	return result
}

func (pr *ProviderRegistry) Rename(oldInstanceID, newInstanceID string) error {
	pr.mu.Lock()
	provider, exists := pr.providers[oldInstanceID]
	if !exists {
		pr.mu.Unlock()
		return fmt.Errorf("provider '%s' not found", oldInstanceID)
	}
	lifecycle := pr.lifecycleLocked(oldInstanceID)
	if lifecycle.exclusive || lifecycle.borrowers != 0 {
		pr.mu.Unlock()
		return ErrProviderBusy
	}

	if _, exists := pr.providers[newInstanceID]; exists {
		pr.mu.Unlock()
		log.Warn().Str("old", oldInstanceID).Str("new", newInstanceID).Msg("Cannot rename provider: target already exists")
		return fmt.Errorf("cannot rename %s to %s: target already exists", oldInstanceID, newInstanceID)
	}

	catalogstate.Invalidate(oldInstanceID)
	catalogstate.Invalidate(newInstanceID)
	delete(pr.providers, oldInstanceID)
	pr.providers[newInstanceID] = provider
	delete(pr.lifecycles, oldInstanceID)
	pr.lifecycles[newInstanceID] = lifecycle

	if _, wasActive := pr.activeProviders[oldInstanceID]; wasActive {
		delete(pr.activeProviders, oldInstanceID)
		pr.activeProviders[newInstanceID] = struct{}{}
	}
	pr.mu.Unlock()

	pr.saveConfigInBackground()

	return nil
}

func (pr *ProviderRegistry) Remove(instanceID string) error {
	// Avoid rejecting ordinary removal just because an earlier activation save
	// is still persisting the same provider.
	pr.WaitForPendingSaves()
	pr.mu.Lock()
	_, exists := pr.providers[instanceID]
	if !exists {
		pr.mu.Unlock()
		return fmt.Errorf("provider '%s' not found", instanceID)
	}
	lifecycle := pr.lifecycleLocked(instanceID)
	if lifecycle.exclusive || lifecycle.borrowers != 0 {
		pr.mu.Unlock()
		return ErrProviderBusy
	}

	catalogstate.Invalidate(instanceID)
	delete(pr.providers, instanceID)
	delete(pr.lifecycles, instanceID)
	delete(pr.activeProviders, instanceID)
	if pr.activeProvider != nil && pr.activeProvider.GetInstanceID() == instanceID {
		pr.activeProvider = nil
		for id := range pr.activeProviders {
			if p, exists := pr.providers[id]; exists {
				pr.activeProvider = p
				break
			}
		}
	}
	pr.mu.Unlock()

	// Clean up token file (outside lock)
	tokenStore := database.NewTokenStore()
	if err := tokenStore.Delete(instanceID); err != nil {
		log.Warn().Str("instance", instanceID).Err(err).Msg("Failed to clean up token")
	}

	pr.saveConfigInBackground()

	log.Debug().Str("provider", instanceID).Msg("Provider removed")
	return nil
}

func (pr *ProviderRegistry) GetInstancesOfType(providerType string) []types.Provider {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var instances []types.Provider
	for _, provider := range pr.providers {
		if provider.GetID() == providerType {
			instances = append(instances, provider)
		}
	}
	return instances
}

func (pr *ProviderRegistry) saveConfigAsync() error {
	// Serialize saves, then lease each provider while reading and persisting it.
	// A stale snapshot must never recreate an identity after it is renamed.
	pr.persistenceMu.Lock()
	defer pr.persistenceMu.Unlock()
	pr.mu.RLock()
	ids := make([]string, 0, len(pr.providers))
	for id := range pr.providers {
		ids = append(ids, id)
	}
	pr.mu.RUnlock()

	// Persist activation state to provider_instances — this is the single source
	// of truth for which providers are active. The old config.active_providers
	// key is no longer written.
	for _, id := range ids {
		provider, release, err := pr.BorrowProvider(id)
		if err != nil {
			continue
		}
		isActive := pr.IsActiveProvider(id)

		record := &database.ProviderInstanceRecord{
			InstanceID: id,
			ProviderID: provider.GetID(),
			Name:       provider.GetName(),
			Activated:  isActive,
		}

		// Preserve existing name, subtitle, and priority from DB if available
		if existing, err := pr.instanceStore.Get(id); err == nil && existing != nil {
			record.Priority = existing.Priority
			// Keep the user-customized name/subtitle, falling back to the in-memory default
			if existing.Name != "" {
				record.Name = existing.Name
			}
			if existing.Subtitle != "" {
				record.Subtitle = existing.Subtitle
			}
		}

		if err := pr.instanceStore.Save(record); err != nil {
			log.Warn().Err(err).Str("instance", id).Msg("Failed to save provider instance")
		}
		release()
	}

	pr.mu.RLock()
	activeIDs := make([]string, 0, len(pr.activeProviders))
	for id := range pr.activeProviders {
		activeIDs = append(activeIDs, id)
	}
	pr.mu.RUnlock()
	log.Debug().Strs("active", activeIDs).Msg("Provider configuration saved")
	return nil
}
