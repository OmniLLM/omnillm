package routes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newCanceledContext builds a gin context whose inbound request has already
// been canceled, mimicking a client that disconnected mid-flight.
func newCanceledContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	return c
}

func newLiveContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, rec
}

func TestIsClientCanceledDetectsWrappedCancellation(t *testing.T) {
	c := newCanceledContext()
	err := fmt.Errorf("streaming request failed: %w", context.Canceled)
	if !isClientCanceled(c, err) {
		t.Fatal("expected wrapped context.Canceled on a canceled request to be client cancellation")
	}
}

func TestIsClientCanceledIgnoresProviderErrorsOnCanceledContext(t *testing.T) {
	c := newCanceledContext()
	err := errors.New("upstream returned status 500")
	if isClientCanceled(c, err) {
		t.Fatal("a non-cancellation error must not be treated as client cancellation")
	}
}

func TestIsClientCanceledIgnoresCancellationOnLiveContext(t *testing.T) {
	c, _ := newLiveContext()
	err := fmt.Errorf("wrapped: %w", context.Canceled)
	if isClientCanceled(c, err) {
		t.Fatal("cancellation not originating from the client must not be misclassified")
	}
}

func TestIsClientCanceledHandlesNilInputs(t *testing.T) {
	c, _ := newLiveContext()
	if isClientCanceled(c, nil) {
		t.Fatal("nil error must not be client cancellation")
	}
	if isClientCanceled(nil, context.Canceled) {
		t.Fatal("nil context must not panic or report cancellation")
	}
}

func TestWriteProviderFailureSuppressesBodyOnClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	writeProviderFailure(c, "provider_error", fmt.Errorf("streaming request failed: %w", context.Canceled))

	if body := rec.Body.String(); body != "" {
		t.Fatalf("expected no response body for a canceled client, got %q", body)
	}
	if !c.IsAborted() {
		t.Fatal("expected the context to be aborted on client cancellation")
	}
}

func TestWriteProviderFailureStillReportsRealProviderErrors(t *testing.T) {
	c, rec := newLiveContext()

	writeProviderFailure(c, "provider_error", errors.New("upstream returned status 500"))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for a genuine provider failure, got %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected an error body for a genuine provider failure")
	}
}
