// Package registry provides a centralized provider registry system
package registry

import (
	"encoding/json"
	"fmt"
	"sync"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

type ProviderRegistry struct {
	mu              sync.RWMutex
	providers       map[string]types.Provider
	activeProvider  types.Provider
	activeProviders map[string]struct{}
	configStore     *database.ConfigStore
	instanceStore   *database.ProviderInstanceStore
}

var globalRegistry *ProviderRegistry
var once sync.Once

func GetProviderRegistry() *ProviderRegistry {
	once.Do(func() {
		globalRegistry = &ProviderRegistry{
			providers:       make(map[string]types.Provider),
			activeProviders: make(map[string]struct{}),
			configStore:     database.NewConfigStore(),
			instanceStore:   database.NewProviderInstanceStore(),
		}
	})
	return globalRegistry
}

func (pr *ProviderRegistry) Register(provider types.Provider, saveConfig bool) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.providers[provider.GetInstanceID()] = provider

	if saveConfig {
		if err := pr.saveConfigAsync(); err != nil {
			log.Warn().Err(err).Msg("Failed to save config after provider registration")
		}
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

func (pr *ProviderRegistry) SetActive(instanceID string) (types.Provider, error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	provider, exists := pr.providers[instanceID]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", instanceID)
	}

	pr.activeProvider = provider
	pr.activeProviders[instanceID] = struct{}{}

	if err := pr.saveConfigAsync(); err != nil {
		log.Warn().Err(err).Msg("Failed to save config after setting active provider")
	}

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
	defer pr.mu.Unlock()

	provider, exists := pr.providers[instanceID]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", instanceID)
	}

	pr.activeProviders[instanceID] = struct{}{}
	if pr.activeProvider == nil {
		pr.activeProvider = provider
	}

	if err := pr.saveConfigAsync(); err != nil {
		log.Warn().Err(err).Msg("Failed to save config after adding active provider")
	}

	return provider, nil
}

func (pr *ProviderRegistry) RemoveActive(instanceID string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

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

	if err := pr.saveConfigAsync(); err != nil {
		log.Warn().Err(err).Msg("Failed to save config after removing active provider")
	}

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
	defer pr.mu.Unlock()

	provider, exists := pr.providers[oldInstanceID]
	if !exists {
		return fmt.Errorf("provider '%s' not found", oldInstanceID)
	}

	// Check if target instanceID already exists
	if _, exists := pr.providers[newInstanceID]; exists {
		log.Warn().Str("old", oldInstanceID).Str("new", newInstanceID).Msg("Cannot rename provider: target already exists")
		return fmt.Errorf("cannot rename %s to %s: target already exists", oldInstanceID, newInstanceID)
	}

	delete(pr.providers, oldInstanceID)
	pr.providers[newInstanceID] = provider

	if _, wasActive := pr.activeProviders[oldInstanceID]; wasActive {
		delete(pr.activeProviders, oldInstanceID)
		pr.activeProviders[newInstanceID] = struct{}{}
	}

	if pr.activeProvider != nil && pr.activeProvider.GetInstanceID() == newInstanceID {
		// Provider object was mutated in place, so already updated
	}

	if err := pr.saveConfigAsync(); err != nil {
		log.Warn().Err(err).Msg("Failed to save config after provider rename")
	}

	return nil
}

func (pr *ProviderRegistry) Remove(instanceID string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	_, exists := pr.providers[instanceID]
	if !exists {
		return fmt.Errorf("provider '%s' not found", instanceID)
	}

	// Remove from registry
	delete(pr.providers, instanceID)

	// Remove from active providers
	delete(pr.activeProviders, instanceID)
	if pr.activeProvider != nil && pr.activeProvider.GetInstanceID() == instanceID {
		pr.activeProvider = nil
		// Pick another active provider if available
		for id := range pr.activeProviders {
			if p, exists := pr.providers[id]; exists {
				pr.activeProvider = p
				break
			}
		}
	}

	// Clean up token file
	tokenStore := database.NewTokenStore()
	if err := tokenStore.Delete(instanceID); err != nil {
		log.Warn().Str("instance", instanceID).Err(err).Msg("Failed to clean up token")
	}

	// Save config after removal
	if err := pr.saveConfigAsync(); err != nil {
		log.Warn().Err(err).Msg("Failed to save config after provider removal")
	}

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

func (pr *ProviderRegistry) NextInstanceID(providerType string) string {
	existing := pr.GetInstancesOfType(providerType)

	// First instance keeps the plain ID, subsequent ones get unique suffixes
	if len(existing) == 0 {
		return providerType
	}

	// For providers that use username-based IDs (like GitHub Copilot),
	// we need to avoid conflicts with existing instances
	counter := 2
	var candidateID string
	for {
		candidateID = fmt.Sprintf("%s-%d", providerType, counter)
		if !pr.IsRegistered(candidateID) {
			break
		}
		counter++
	}

	return candidateID
}

func (pr *ProviderRegistry) saveConfigAsync() error {
	// Save provider instances and activation state to database
	for id, provider := range pr.providers {
		_, isActive := pr.activeProviders[id]

		record := &database.ProviderInstanceRecord{
			InstanceID: id,
			ProviderID: provider.GetID(),
			Name:       provider.GetName(),
			Activated:  isActive,
		}

		// Preserve existing priority from DB if available
		if existing, err := pr.instanceStore.Get(id); err == nil && existing != nil {
			record.Priority = existing.Priority
		}

		if err := pr.instanceStore.Save(record); err != nil {
			log.Warn().Err(err).Str("instance", id).Msg("Failed to save provider instance")
		}
	}

	// Save active provider IDs to config store
	activeIDs := make([]string, 0, len(pr.activeProviders))
	for id := range pr.activeProviders {
		activeIDs = append(activeIDs, id)
	}

	activeJSON, _ := json.Marshal(activeIDs)
	if err := pr.configStore.Set("active_providers", string(activeJSON)); err != nil {
		log.Warn().Err(err).Msg("Failed to save active providers config")
	}

	log.Debug().Strs("active", activeIDs).Msg("Provider configuration saved")
	return nil
}
