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
	if port != 5005 {
		t.Fatalf("expected default port 5005, got %d", port)
	}

	provider, err := StartCmd.Flags().GetString("provider")
	if err != nil {
		t.Fatalf("get provider flag: %v", err)
	}
	if provider != "github-copilot" {
		t.Fatalf("expected default provider github-copilot, got %q", provider)
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
	cmd.SetArgs([]string{"--port", "5005", "--rate-limit", "bad"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid argument \"bad\"") {
		t.Fatalf("expected invalid rate-limit error, got %v", err)
	}
}
