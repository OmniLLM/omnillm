package routes

import (
	"omnillm/internal/database"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/vmodelrouting"

	"github.com/rs/zerolog/log"
)

type resolvedModelAttempt struct {
	RequestedModel  string
	NormalizedModel string
}

func resolveRequestedModel(requestID string, requestedModel string) (string, string) {
	attempts := resolveRequestedModels(requestID, requestedModel)
	if len(attempts) == 0 {
		normalizedModel := modelrouting.NormalizeModelName(requestedModel)
		return requestedModel, normalizedModel
	}
	return attempts[0].RequestedModel, attempts[0].NormalizedModel
}

func resolveRequestedModels(requestID string, requestedModel string) []resolvedModelAttempt {
	normalizedModel := modelrouting.NormalizeModelName(requestedModel)

	vmodelStore := database.NewVirtualModelStore()
	vm, err := vmodelStore.Get(normalizedModel)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("model", requestedModel).Msg("Failed to load virtual model")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}
	if vm == nil || !vm.Enabled {
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	upstreamStore := database.NewVirtualModelUpstreamStore()
	upstreams, err := upstreamStore.GetForVModel(vm.VirtualModelID)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Failed to load virtual model upstreams")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	ordered := vmodelrouting.OrderUpstreams(upstreams, vm.LbStrategy, vm.VirtualModelID)
	if len(ordered) == 0 {
		log.Warn().Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Virtual model has no routable upstream")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	attempts := make([]resolvedModelAttempt, 0, len(ordered))
	for _, upstream := range ordered {
		log.Debug().
			Str("request_id", requestID).
			Str("virtual_model", vm.VirtualModelID).
			Str("upstream", upstream.ModelID).
			Str("strategy", string(vm.LbStrategy)).
			Msg("Virtual model routing candidate")
		attempts = append(attempts, resolvedModelAttempt{
			RequestedModel:  upstream.ModelID,
			NormalizedModel: modelrouting.NormalizeModelName(upstream.ModelID),
		})
	}

	return attempts
}
