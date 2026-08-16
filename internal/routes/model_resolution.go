package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/lib/affinity"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/virtualmodelrouting"

	"github.com/rs/zerolog/log"
)

type resolvedModelAttempt struct {
	RequestedModel            string
	NormalizedModel           string
	ProviderID                string // non-empty when resolved from a virtual model upstream with a specific provider
	OnlyIfPreviousUnavailable bool
}

func resolveRequestedModels(requestID, requestedModel string) []resolvedModelAttempt {
	normalizedModel := modelrouting.NormalizeModelName(requestedModel)

	cache := database.GetModelResolutionCache()

	vm := cache.GetVirtualModel(requestedModel)
	if vm == nil && normalizedModel != requestedModel {
		vm = cache.GetVirtualModel(normalizedModel)
	}
	if vm == nil || !vm.Enabled {
		attempts := []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
		return appendProviderPrefixFallback(requestID, attempts, requestedModel)
	}

	upstreams := cache.GetUpstreams(vm.VirtualModelID)
	if len(upstreams) == 0 {
		log.Warn().Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Virtual model has no routable upstream")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	ordered := virtualmodelrouting.OrderUpstreams(upstreams, vm.LbStrategy, vm.VirtualModelID)
	if len(ordered) == 0 {
		log.Warn().Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Virtual model has no routable upstream")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	attempts := make([]resolvedModelAttempt, 0, len(ordered))
	for _, upstream := range ordered {
		prefix, bareUpstreamModel := modelrouting.ParseProviderPrefix(upstream.ModelID)
		executionModel := upstream.ModelID
		providerID := upstream.ProviderID
		if resolvedInstanceID, ok := lookupProviderPrefix(prefix); ok {
			executionModel = bareUpstreamModel
			if providerID == "" {
				providerID = resolvedInstanceID
			}
		}

		log.Debug().
			Str("request_id", requestID).
			Str("virtual_model", vm.VirtualModelID).
			Str("upstream", upstream.ModelID).
			Str("execution_model", executionModel).
			Str("provider", providerID).
			Str("strategy", string(vm.LbStrategy)).
			Msg("Virtual model routing candidate")
		attempts = append(attempts, resolvedModelAttempt{
			RequestedModel:  executionModel,
			NormalizedModel: modelrouting.NormalizeModelName(executionModel),
			ProviderID:      providerID,
		})
	}

	return attempts
}

func preferAffinityAttempt(attempts []resolvedModelAttempt, request *cif.CanonicalRequest, requestedModel string) []resolvedModelAttempt {
	instance, ok := affinity.Get().Lookup(request, requestedModel)
	if !ok || len(attempts) < 2 {
		return attempts
	}
	for index, attempt := range attempts {
		if attempt.ProviderID != instance || index == 0 {
			continue
		}
		ordered := make([]resolvedModelAttempt, 0, len(attempts))
		ordered = append(ordered, attempts[index])
		ordered = append(ordered, attempts[:index]...)
		ordered = append(ordered, attempts[index+1:]...)
		return ordered
	}
	return attempts
}

func resolveRequestedModelsForRequest(requestID, requestedModel string, request *cif.CanonicalRequest) []resolvedModelAttempt {
	return preferAffinityAttempt(resolveRequestedModels(requestID, requestedModel), request, requestedModel)
}

func appendProviderPrefixFallback(requestID string, attempts []resolvedModelAttempt, requestedModel string) []resolvedModelAttempt {
	prefix, bareModel := modelrouting.ParseProviderPrefix(requestedModel)
	resolvedInstanceID, ok := lookupProviderPrefix(prefix)
	if !ok || bareModel == "" {
		return attempts
	}

	log.Debug().
		Str("request_id", requestID).
		Str("provider_prefix", prefix).
		Str("resolved_instance_id", resolvedInstanceID).
		Str("model", bareModel).
		Msg("Adding provider-qualified model fallback")
	return append(attempts, resolvedModelAttempt{
		RequestedModel:            bareModel,
		NormalizedModel:           modelrouting.NormalizeModelName(bareModel),
		ProviderID:                resolvedInstanceID,
		OnlyIfPreviousUnavailable: true,
	})
}

// lookupProviderPrefix maps a user-supplied prefix to a known registry instance ID.
//
// The lookup order is:
//  1. Exact match against a registered instance ID (e.g. "alibaba-2").
//  2. Case-insensitive match against the provider's subtitle — the short label
//     users set in the UI (e.g. "alipay01").
func lookupProviderPrefix(prefix string) (string, bool) {
	if prefix == "" {
		return "", false
	}
	return database.GetModelResolutionCache().LookupProviderPrefix(prefix)
}
