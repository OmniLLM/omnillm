package commands

import (
	"github.com/spf13/cobra"
	"testing"
)

func TestTypeSafeEnvironmentCredentials(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "environment-sentinel")
	cmd := &cobra.Command{}
	addProviderAuthFlags(cmd)
	if err := promptForProviderAuth(cmd, "typesafe"); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetString("api-key"); got != "environment-sentinel" {
		t.Fatal("environment key not used")
	}
	_ = cmd.Flags().Set("api-key", "explicit-sentinel")
	if err := promptForProviderAuth(cmd, "typesafe"); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetString("api-key"); got != "explicit-sentinel" {
		t.Fatal("explicit key replaced")
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	_ = cmd.Flags().Set("api-key", "")
	_ = cmd.Flags().Set("yes", "true")
	if err := promptForProviderAuth(cmd, "typesafe"); err == nil {
		t.Fatal("missing key accepted")
	}
}
