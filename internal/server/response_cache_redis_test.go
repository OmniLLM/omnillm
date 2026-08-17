package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"omnillm/internal/lib/ratelimit"
	"omnillm/internal/lib/responsecache"
	"omnillm/internal/routes"
)

func TestRedactRedisErrorCredentials(t *testing.T) {
	for _, raw := range []string{
		"dial redis://user:secret@example.test:6379/0 failed",
		"parse redis://user:sec@ret@example.test:bad/0 failed",
	} {
		err := redactRedisError(assertionError(raw))
		message := err.Error()
		if strings.Contains(message, "user") || strings.Contains(message, "secret") || strings.Contains(message, "sec@ret") {
			t.Fatalf("redacted error exposed credentials: %q", message)
		}
		if !strings.Contains(message, "***@example.test") {
			t.Fatalf("redacted error omitted safe endpoint context: %q", message)
		}
	}
}

func TestConfigureResponseCacheRedisInvalidURLFailsOpen(t *testing.T) {
	store, restore := configureResponseCacheRedis(StartOptions{
		ResponseCacheRedisURL:    "://user:secret@bad",
		ResponseCacheRedisPrefix: "omnillm",
	})
	defer restore()
	if store != nil {
		t.Fatal("invalid Redis URL should install the unavailable store")
	}
}

func TestHealthEndpointsIgnoreRedisAvailability(t *testing.T) {
	restore := responsecache.ConfigureStore(nil)
	defer restore()
	router := buildRouter(0, "", newTestChatOptions(), 0)

	for _, path := range []string{"/", "/health", "/healthz"} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
	}
}

func newTestChatOptions() routes.ChatCompletionOptions {
	return routes.ChatCompletionOptions{RateLimiter: ratelimit.NewRateLimiter(0, false)}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
