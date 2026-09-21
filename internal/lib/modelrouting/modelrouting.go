// Package modelrouting provides sophisticated model routing and provider selection
package modelrouting

import (
	"fmt"
	"omnillm/internal/database"
	"omnillm/internal/lib/catalogcache"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"omnillm/internal/systemone"
	"sort"
	"strings"
	"time"
)

type ResolvedModelRoute struct {
	SelectedModel      *types.Model     `json:"selectedModel"`
	CandidateProviders []types.Provider `json:"candidateProviders"`
	AvailableModels    []types.Model    `json:"availableModels"`
}

type ModelCache struct{ catalog *catalogcache.Cache }

func NewModelCache() *ModelCache {
	return &ModelCache{catalog: catalogcache.New(5*time.Minute, 0)}
}

func GetCachedOrFetchModels(provider types.Provider, cache *ModelCache) (*types.ModelsResponse, error) {
	// Providers owning freshness must not be wrapped in another TTL.
	if managed, ok := provider.(interface{ ManagesModelCache() bool }); ok && managed.ManagesModelCache() {
		return provider.GetModels()
	}
	return cache.catalog.Get(provider.GetInstanceID(), provider.GetID(), func() (*types.ModelsResponse, error) {
		models, err := provider.GetModels()
		if models != nil && models.Degraded {
			copy := *models
			copy.Degraded = true
			if copy.Source == "" {
				copy.Source = "fallback"
			}
			return &copy, nil
		}
		return models, err
	})
}

func GetEnabledModelsByProvider(providers []types.Provider, cache *ModelCache) (map[string][]types.Model, error) {
	modelsByProvider := make(map[string][]types.Model)
	stateCache := database.GetModelStateCache()

	for _, provider := range providers {
		providerModels, err := GetCachedOrFetchModels(provider, cache)
		if err != nil {
			continue // Skip this provider if we can't get models
		}

		instanceID := provider.GetInstanceID()
		disabledModels := stateCache.GetDisabledModels(instanceID)

		enabledModels := make([]types.Model, 0, len(providerModels.Data))
		for _, m := range providerModels.Data {
			if !disabledModels[m.ID] {
				enabledModels = append(enabledModels, m)
			}
		}
		modelsByProvider[instanceID] = enabledModels
	}

	return modelsByProvider, nil
}

func SortProvidersByPriority(providers []types.Provider) []types.Provider {
	sorted := make([]types.Provider, len(providers))
	copy(sorted, providers)

	// Load priorities from the in-memory cache (zero DB queries on hot path).
	instances := database.GetModelResolutionCache().GetAllProviderInstances()
	priorityMap := make(map[string]int, len(instances))
	for _, inst := range instances {
		priorityMap[inst.InstanceID] = inst.Priority
	}

	sort.Slice(sorted, func(i, j int) bool {
		pi := priorityMap[sorted[i].GetInstanceID()]
		pj := priorityMap[sorted[j].GetInstanceID()]
		if pi != pj {
			return pi < pj
		}
		// Fall back to alphabetical if same priority
		return sorted[i].GetInstanceID() < sorted[j].GetInstanceID()
	})

	return sorted
}

// ResolveProvidersForModel determines which providers and models to use for a given request.
// If providerID is non-empty, only that specific provider is considered as a candidate.
//
//nolint:gocyclo // model resolution involves multiple fallback strategies
func ResolveProvidersForModel(requestedModel, normalizedModel, providerID string, cache *ModelCache) (*ResolvedModelRoute, error) {
	return resolveProvidersForOperation(requestedModel, normalizedModel, providerID, cache, false)
}

// ResolveEvaluationProviders applies the same identity and ordering rules to native evaluations.
func ResolveEvaluationProviders(requestedModel, normalizedModel, providerID string, cache *ModelCache) (*ResolvedModelRoute, error) {
	return resolveProvidersForOperation(requestedModel, normalizedModel, providerID, cache, true)
}

func resolveProvidersForOperation(requestedModel, normalizedModel, providerID string, cache *ModelCache, evaluation bool) (*ResolvedModelRoute, error) {
	reg := registry.GetProviderRegistry()
	activeProviders := make([]types.Provider, 0)
	for _, p := range reg.GetActiveProviders() {
		_, native := p.(systemone.Evaluator)
		if native != evaluation {
			if p.GetInstanceID() == providerID && !evaluation {
				return nil, systemone.ErrUnsupported
			}
			continue
		}
		activeProviders = append(activeProviders, p)
	}

	// If a specific provider is requested, filter to only that provider.
	if providerID != "" {
		filtered := make([]types.Provider, 0, 1)
		for _, p := range activeProviders {
			if p.GetInstanceID() == providerID {
				filtered = append(filtered, p)
				break
			}
		}
		if len(filtered) == 0 {
			return &ResolvedModelRoute{
				SelectedModel:      nil,
				CandidateProviders: []types.Provider{},
			}, nil
		}
		activeProviders = filtered
	}

	if len(activeProviders) == 0 {
		if len(reg.GetActiveProviders()) > 0 {
			return &ResolvedModelRoute{CandidateProviders: []types.Provider{}}, nil
		}
		return nil, fmt.Errorf("no active providers available")
	}

	// When a specific provider is pinned (e.g. from a virtual model upstream), skip
	// the model-list check entirely. The /models API may be unavailable or may not
	// list the model (e.g. DeepSeek stripped from the Alibaba hardcoded fallback when
	// /models returns 401), but the provider can still serve the model. Synthesise a
	// passthrough entry so the request proceeds.
	if providerID != "" && len(activeProviders) == 1 {
		provider := activeProviders[0]
		pinned := types.Model{
			ID:       requestedModel,
			Name:     requestedModel,
			Provider: provider.GetInstanceID(),
		}
		return &ResolvedModelRoute{
			SelectedModel:      &pinned,
			CandidateProviders: []types.Provider{provider},
		}, nil
	}

	modelsByProvider, err := GetEnabledModelsByProvider(activeProviders, cache)
	if err != nil {
		return nil, fmt.Errorf("failed to get models by provider: %w", err)
	}

	// Collect all available models
	var availableModels []types.Model
	for _, models := range modelsByProvider {
		availableModels = append(availableModels, models...)
	}

	// Find the selected model
	var selectedModel *types.Model
	for _, model := range availableModels {
		if model.ID == requestedModel || model.ID == normalizedModel {
			selectedModel = &model
			break
		}
	}

	if selectedModel == nil {
		return &ResolvedModelRoute{
			SelectedModel:      nil,
			CandidateProviders: []types.Provider{},
			AvailableModels:    availableModels,
		}, nil
	}

	selectedModes := modelAPIModes(*selectedModel)

	// Find candidate providers that have this model
	var candidateProviders []types.Provider
	for _, provider := range activeProviders {
		providerModels := modelsByProvider[provider.GetInstanceID()]
		for _, model := range providerModels {
			if model.ID != requestedModel && model.ID != normalizedModel {
				continue
			}
			if provider.GetID() == string(types.ProviderAlibaba) && len(selectedModes) > 0 {
				// Always use openai-compatible mode since we simplified the provider
				providerMode := "openai-compatible"
				if !containsString(selectedModes, providerMode) {
					continue
				}
			}
			candidateProviders = append(candidateProviders, provider)
			break
		}
	}

	// Sort providers by priority
	candidateProviders = SortProvidersByPriority(candidateProviders)

	return &ResolvedModelRoute{
		SelectedModel:      selectedModel,
		CandidateProviders: candidateProviders,
		AvailableModels:    availableModels,
	}, nil
}

func modelAPIModes(model types.Model) []string {
	if model.Capabilities == nil {
		return nil
	}
	raw, ok := model.Capabilities["api_modes"]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func providerConfig(provider types.Provider) map[string]interface{} {
	type configProvider interface {
		GetConfig() map[string]interface{}
	}
	if configured, ok := provider.(configProvider); ok {
		return configured.GetConfig()
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ParseProviderPrefix splits a model string of the form "<instanceID>/<modelID>"
// into the provider instance ID and the bare model ID.
//
// If the input contains no "/" the returned providerID is empty and modelID
// equals the original input, so callers can always use modelID as the real
// model name regardless of whether a prefix was supplied.
//
// Examples:
//
//	"copilot-jzhu-abk/gpt-4o-mini"  → ("copilot-jzhu-abk", "gpt-4o-mini")
//	"gpt-4o-mini"                    → ("", "gpt-4o-mini")
func ParseProviderPrefix(modelName string) (providerID, modelID string) {
	if before, after, found := strings.Cut(modelName, "/"); found {
		return before, after
	}
	return "", modelName
}

func NormalizeModelName(modelName string) string {
	normalizedModel := strings.TrimSpace(strings.ToLower(modelName))

	// Model name normalization - maps aliases and dated versions to canonical names
	switch normalizedModel {
	case "gpt-4":
		return "gpt-4o"
	case "gpt-3.5-turbo":
		return "gpt-4o-mini"
	case "claude-3-sonnet":
		return "claude-3-5-sonnet-20241022"
	case "haiku", "haiku-4.5", "claude-haiku", "claude-haiku-4.5", "claude-haiku-4-5":
		return "claude-haiku-4.5"
	case "sonnet", "sonnet-4", "claude-sonnet", "claude-sonnet-4":
		return "claude-sonnet-4"
	case "sonnet-4.5", "sonnet-4-5", "claude-sonnet-4.5", "claude-sonnet-4-5":
		return "claude-sonnet-4.5"
	case "sonnet-4.6", "sonnet-4-6", "claude-sonnet-4.6", "claude-sonnet-4-6":
		return "claude-sonnet-4.6"
	case "opus", "opus-4.6", "opus-4-6", "claude-opus", "claude-opus-4.6", "claude-opus-4-6":
		return "claude-opus-4.6"
	default:
		switch {
		case strings.HasPrefix(normalizedModel, "claude-haiku-4-5"):
			return "claude-haiku-4.5"
		case strings.HasPrefix(normalizedModel, "claude-sonnet-4-5"):
			return "claude-sonnet-4.5"
		case strings.HasPrefix(normalizedModel, "claude-sonnet-4-6"):
			return "claude-sonnet-4.6"
		case strings.HasPrefix(normalizedModel, "claude-opus-4-6"):
			return "claude-opus-4.6"
		default:
			return modelName
		}
	}
}
