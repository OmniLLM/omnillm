package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsGetResponseCacheShowsBackendAvailability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/settings/response-cache" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"ttl_seconds":3600,"entries":2,"total_hits":7,"backend":"redis","available":false}`))
	}))
	defer server.Close()

	t.Setenv("OMNILLM_SERVER", server.URL)
	t.Setenv("OMNILLM_API_KEY", "test-key")

	cmd := *settingsGetResponseCacheCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(&cmd, nil); err != nil {
		t.Fatalf("run response-cache settings: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"Response cache: enabled",
		"Backend:        redis (degraded)",
		"TTL:            3600 seconds",
		"Entries:        2",
		"Total hits:     7",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output:\n%s", expected, output)
		}
	}
}

func TestResponseCacheAvailability(t *testing.T) {
	if got := responseCacheAvailability(true); got != "available" {
		t.Fatalf("available status = %q", got)
	}
	if got := responseCacheAvailability(false); got != "degraded" {
		t.Fatalf("degraded status = %q", got)
	}
}
