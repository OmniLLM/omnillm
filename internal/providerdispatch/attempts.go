package providerdispatch

import (
	"errors"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
)

type CandidateHandler func(candidate *Candidate, providerID string) error

type AttemptErrorHandler func(attempt Attempt, err error)

type AttemptEmptyHandler func(attempt Attempt)

// AbortError wraps an error that must stop the failover loop immediately
// instead of falling through to the next candidate or attempt.  It exists for
// failures that no other provider could possibly resolve -- most importantly a
// client disconnect, where retrying every remaining candidate would issue N
// doomed upstream requests on an already-dead context.
type AbortError struct {
	Err error
}

func (e *AbortError) Error() string {
	if e == nil || e.Err == nil {
		return "attempt aborted"
	}
	return e.Err.Error()
}

func (e *AbortError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Abort marks err as terminal for the failover loop.  Returns nil for a nil
// err so callers can wrap unconditionally.
func Abort(err error) error {
	if err == nil {
		return nil
	}
	return &AbortError{Err: err}
}

func isAbort(err error) bool {
	var abortErr *AbortError
	return errors.As(err, &abortErr)
}

func (e *Executor) TryAttempts(attempts []Attempt, request *cif.CanonicalRequest, cache *modelrouting.ModelCache, resolve ResolveFunc, onEmpty AttemptEmptyHandler, onError AttemptErrorHandler, handle CandidateHandler) error {
	var lastErr error

	for _, attempt := range attempts {
		prepared, err := e.PrepareCandidates(attempt, request, cache, resolve)
		if err != nil {
			if onError != nil {
				onError(attempt, err)
			}
			return err
		}

		if len(prepared) == 0 {
			lastErr = fmt.Errorf("model '%s' not found or no providers available", attempt.RequestedModel)
			if onEmpty != nil {
				onEmpty(attempt)
			}
			continue
		}

		for _, preparedCandidate := range prepared {
			candidate := preparedCandidate.Candidate
			providerID := preparedCandidate.ProviderID
			lastErr = handle(candidate, providerID)
			if lastErr == nil {
				return nil
			}
			if isAbort(lastErr) {
				return lastErr
			}
		}
	}

	return lastErr
}
