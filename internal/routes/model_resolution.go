package routes

import (
	"omnimodel/internal/database"
	"omnimodel/internal/lib/modelrouting"
	"omnimodel/internal/lib/vmodelrouting"

	"github.com/rs/zerolog/log"
)

func resolveRequestedModel(requestID string, requestedModel string) (string, string) {
	normalizedModel := modelrouting.NormalizeModelName(requestedModel)

	vmodelStore := database.NewVirtualModelStore()
	vm, err := vmodelStore.Get(normalizedModel)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("model", requestedModel).Msg("Failed to load virtual model")
		return requestedModel, normalizedModel
	}
	if vm == nil || !vm.Enabled {
		return requestedModel, normalizedModel
	}

	upstreamStore := database.NewVirtualModelUpstreamStore()
	upstreams, err := upstreamStore.GetForVModel(vm.VirtualModelID)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Failed to load virtual model upstreams")
		return requestedModel, normalizedModel
	}

	selected := vmodelrouting.SelectUpstream(upstreams, vm.LbStrategy, vm.VirtualModelID)
	if selected == nil {
		log.Warn().Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Virtual model has no routable upstream")
		return requestedModel, normalizedModel
	}

	log.Debug().
		Str("request_id", requestID).
		Str("virtual_model", vm.VirtualModelID).
		Str("upstream", selected.ModelID).
		Str("strategy", string(vm.LbStrategy)).
		Msg("Virtual model routing")

	return selected.ModelID, modelrouting.NormalizeModelName(selected.ModelID)
}
