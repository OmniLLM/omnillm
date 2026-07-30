package providerdispatch

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestAbortWrapsAndUnwraps(t *testing.T) {
	inner := fmt.Errorf("streaming failed: %w", context.Canceled)
	wrapped := Abort(inner)

	if !isAbort(wrapped) {
		t.Fatal("expected Abort() result to be recognized as an abort")
	}
	if !errors.Is(wrapped, context.Canceled) {
		t.Fatal("Abort must preserve the wrapped error chain")
	}
	if wrapped.Error() != inner.Error() {
		t.Fatalf("expected message %q, got %q", inner.Error(), wrapped.Error())
	}
}

func TestAbortReturnsNilForNilError(t *testing.T) {
	if Abort(nil) != nil {
		t.Fatal("Abort(nil) must be nil so callers can wrap unconditionally")
	}
}

func TestIsAbortRejectsPlainErrors(t *testing.T) {
	if isAbort(errors.New("ordinary provider failure")) {
		t.Fatal("a plain error must not be treated as an abort")
	}
	if isAbort(nil) {
		t.Fatal("nil must not be treated as an abort")
	}
}

func TestIsAbortDetectsNestedAbort(t *testing.T) {
	nested := fmt.Errorf("candidate 2: %w", Abort(errors.New("client gone")))
	if !isAbort(nested) {
		t.Fatal("isAbort must find an AbortError deeper in the chain")
	}
}
