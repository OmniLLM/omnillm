package commands

import (
	"strings"
	"testing"
)

func TestStartCmdDefaults(t *testing.T) {
	port, err := StartCmd.Flags().GetInt("port")
	if err != nil {
		t.Fatalf("get port flag: %v", err)
	}
	if port != 5000 {
		t.Fatalf("expected default port 5000, got %d", port)
	}

	provider, err := StartCmd.Flags().GetString("provider")
	if err != nil {
		t.Fatalf("get provider flag: %v", err)
	}
	if provider != "github-copilot" {
		t.Fatalf("expected default provider github-copilot, got %q", provider)
	}

	redisURL, err := StartCmd.Flags().GetString("response-cache-redis-url")
	if err != nil {
		t.Fatalf("get Redis URL flag: %v", err)
	}
	if redisURL != "redis://127.0.0.1:6379/0" {
		t.Fatalf("Redis URL default = %q", redisURL)
	}

	redisPrefix, err := StartCmd.Flags().GetString("response-cache-redis-prefix")
	if err != nil {
		t.Fatalf("get Redis prefix flag: %v", err)
	}
	if redisPrefix != "omnillm" {
		t.Fatalf("Redis prefix default = %q", redisPrefix)
	}
}

func TestResponseCacheRedisEnvironmentOverride(t *testing.T) {
	t.Setenv("OMNILLM_RESPONSE_CACHE_REDIS_URL", "rediss://user:secret@example.test:6380/2")
	t.Setenv("OMNILLM_RESPONSE_CACHE_REDIS_PREFIX", "team-a")

	if got := environmentOverride("redis://127.0.0.1:6379/0", "OMNILLM_RESPONSE_CACHE_REDIS_URL"); got != "rediss://user:secret@example.test:6380/2" {
		t.Fatalf("Redis URL environment override = %q", got)
	}
	if got := environmentOverride("omnillm", "OMNILLM_RESPONSE_CACHE_REDIS_PREFIX"); got != "team-a" {
		t.Fatalf("Redis prefix environment override = %q", got)
	}
}

func TestResponseCacheRedisEnvironmentBlankUsesDefault(t *testing.T) {
	t.Setenv("OMNILLM_RESPONSE_CACHE_REDIS_URL", "  ")
	if got := environmentOverride("redis://127.0.0.1:6379/0", "OMNILLM_RESPONSE_CACHE_REDIS_URL"); got != "redis://127.0.0.1:6379/0" {
		t.Fatalf("blank environment override = %q", got)
	}
}

func TestStartCmdRejectsInvalidPort(t *testing.T) {
	cmd := *StartCmd
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--port", "not-a-number"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid argument \"not-a-number\"") {
		t.Fatalf("expected invalid port error, got %v", err)
	}
}

func TestStartCmdRejectsInvalidRateLimit(t *testing.T) {
	cmd := *StartCmd
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--port", "5000", "--rate-limit", "bad"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid argument \"bad\"") {
		t.Fatalf("expected invalid rate-limit error, got %v", err)
	}
}
