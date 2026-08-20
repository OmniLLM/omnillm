package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCacheStatsCommandRegistration(t *testing.T) {
	stats, _, err := CacheCmd.Find([]string{"stats"})
	if err != nil || stats != cacheStatsCmd {
		t.Fatalf("cache stats registration = (%v, %v), want cacheStatsCmd", stats, err)
	}
}

func TestCacheStatsTable(t *testing.T) {
	server := newCacheStatsServer(t, `{"enabled":true,"ttl_seconds":60,"entries":2,"total_hits":7,"payload_bytes":1536,"lookup_hits":3,"lookup_misses":1,"lookup_hit_rate":0.75,"stats_since":"2026-08-20T08:00:00Z","backend":"redis","available":true}`)
	cmd := newCacheStatsTestCommand(t, server.URL, "table")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runCacheStats(cmd, nil); err != nil {
		t.Fatalf("runCacheStats: %v", err)
	}
	for _, expected := range []string{
		"Exact-response cache statistics", "enabled", "60 seconds", "redis (available)", "1.5 KiB (1536 B)",
		"Lookup hits", "3", "Lookup misses", "1", "75.00%", "2026-08-20T08:00:00Z", "Live-entry total hits", "7",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("expected %q in output:\n%s", expected, out.String())
		}
	}
}

func TestCacheStatsUnavailableAndOlderServer(t *testing.T) {
	server := newCacheStatsServer(t, `{"entries":2,"total_hits":7,"backend":"redis","available":false}`)
	cmd := newCacheStatsTestCommand(t, server.URL, "table")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runCacheStats(cmd, nil); err != nil {
		t.Fatalf("runCacheStats: %v", err)
	}
	for _, expected := range []string{"redis (degraded)", "0 B", "unavailable"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("expected %q in output:\n%s", expected, out.String())
		}
	}
}

func TestCacheStatsJSONPreservesServerResponse(t *testing.T) {
	body := `{"lookup_hit_rate":null,"stats_since":null,"extension":{"kept":true}}`
	server := newCacheStatsServer(t, body)
	cmd := newCacheStatsTestCommand(t, server.URL, "json")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runCacheStats(cmd, nil); err != nil {
		t.Fatalf("runCacheStats: %v", err)
	}
	for _, expected := range []string{`"lookup_hit_rate": null`, `"stats_since": null`, `"kept": true`} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("expected %q in JSON output:\n%s", expected, out.String())
		}
	}
}

func newCacheStatsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/settings/response-cache" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func newCacheStatsTestCommand(t *testing.T, serverURL, output string) *cobra.Command {
	t.Helper()
	t.Setenv("OMNILLM_SERVER", serverURL)
	t.Setenv("OMNILLM_API_KEY", "test-key")
	root := &cobra.Command{Use: "omnillm"}
	root.PersistentFlags().String("server", "", "")
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().String("output", output, "")
	cmd := &cobra.Command{Use: "stats"}
	root.AddCommand(cmd)
	return cmd
}
