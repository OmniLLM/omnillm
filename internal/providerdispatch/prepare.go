package providerdispatch

import (
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/lib/affinity"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

type Attempt struct {
	RequestedModel            string
	NormalizedModel           string
	ProviderID                string
	OnlyIfPreviousUnavailable bool
}

type ResolveFunc func(requestedModel, normalizedModel, providerID string, cache *modelrouting.ModelCache) (*modelrouting.ResolvedModelRoute, error)

type PreparedCandidate struct {
	Candidate  *Candidate
	ProviderID string
}

func (e *Executor) PrepareCandidates(attempt Attempt, request *cif.CanonicalRequest, cache *modelrouting.ModelCache, resolve ResolveFunc) ([]PreparedCandidate, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if resolve == nil {
		return nil, fmt.Errorf("resolve function is nil")
	}

	modelRoute, err := resolve(attempt.RequestedModel, attempt.NormalizedModel, attempt.ProviderID, cache)
	if err != nil {
		return nil, err
	}
	if len(modelRoute.CandidateProviders) == 0 {
		return nil, nil
	}

	// Channel affinity: if this conversation prefix was previously pinned to a
	// still-available instance, move it to the front so upstream prompt-cache
	// stays warm. This only re-orders — fallback order is otherwise unchanged.
	candidateProviders := modelRoute.CandidateProviders
	if inst, ok := affinity.Get().Lookup(request, attempt.RequestedModel); ok {
		reordered := moveInstanceToFront(candidateProviders, inst)
		// Only log when the hit actually changed ordering (i.e. >1 candidate and
		// the pinned instance wasn't already first). Avoids noise on the common
		// single-instance path where affinity is a no-op.
		if len(reordered) > 1 && reordered[0].GetInstanceID() == inst && candidateProviders[0].GetInstanceID() != inst {
			log.Debug().
				Str("model", attempt.RequestedModel).
				Str("affinity_instance", inst).
				Msg("Channel affinity hit — pinned instance moved to front")
		}
		candidateProviders = reordered
	}

	attemptRequest := *request
	attemptRequest.Model = attempt.RequestedModel
	if attempt.ProviderID == "" && attempt.NormalizedModel != attemptRequest.Model {
		attemptRequest.Model = attempt.NormalizedModel
	}

	prepared := make([]PreparedCandidate, 0, len(candidateProviders))
	for _, provider := range candidateProviders {
		candidate, err := e.BuildCandidate(provider, &attemptRequest)
		if err != nil {
			continue
		}
		prepared = append(prepared, PreparedCandidate{
			Candidate:  candidate,
			ProviderID: provider.GetInstanceID(),
		})
	}

	return prepared, nil
}

// moveInstanceToFront returns providers with the instance matching instanceID
// moved to position 0. If not present, the slice is returned unchanged.
func moveInstanceToFront(providers []types.Provider, instanceID string) []types.Provider {
	if instanceID == "" || len(providers) < 2 {
		return providers
	}
	idx := -1
	for i, p := range providers {
		if p.GetInstanceID() == instanceID {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return providers
	}
	reordered := make([]types.Provider, 0, len(providers))
	reordered = append(reordered, providers[idx])
	reordered = append(reordered, providers[:idx]...)
	reordered = append(reordered, providers[idx+1:]...)
	return reordered
}
