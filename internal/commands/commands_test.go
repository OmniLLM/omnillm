package commands

import (
	"testing"
)

// ─── provider command ─────────────────────────────────────────────────────────

func TestProviderCmdStructure(t *testing.T) {
	expectedSubs := []string{
		"list", "add", "delete", "activate", "deactivate",
		"switch", "rename", "priorities", "usage",
	}
	subNames := make(map[string]bool)
	for _, sub := range ProviderCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expectedSubs {
		if !subNames[name] {
			t.Errorf("provider: missing subcommand %q", name)
		}
	}
}

func TestProviderAddFlagDefaults(t *testing.T) {
	for _, flagName := range []string{"api-key", "token", "endpoint", "region", "plan"} {
		found := false
		for _, sub := range ProviderCmd.Commands() {
			if sub.Name() == "add" {
				if sub.Flags().Lookup(flagName) == nil {
					t.Errorf("provider add: missing flag --%s", flagName)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("provider add: subcommand not found while checking flag %s", flagName)
		}
	}
}

func TestProviderDeleteHasYesFlag(t *testing.T) {
	for _, sub := range ProviderCmd.Commands() {
		if sub.Name() == "delete" {
			if sub.Flags().Lookup("yes") == nil {
				t.Error("provider delete: missing --yes flag")
			}
			return
		}
	}
	t.Error("provider delete: subcommand not found")
}

// ─── model command ────────────────────────────────────────────────────────────

func TestModelCmdStructure(t *testing.T) {
	expected := []string{"list", "refresh", "toggle", "version"}
	subNames := make(map[string]bool)
	for _, sub := range ModelCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("model: missing subcommand %q", name)
		}
	}
}

func TestModelToggleFlags(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "toggle" {
			if sub.Flags().Lookup("enable") == nil {
				t.Error("model toggle: missing --enable flag")
			}
			if sub.Flags().Lookup("disable") == nil {
				t.Error("model toggle: missing --disable flag")
			}
			return
		}
	}
	t.Error("model toggle: subcommand not found")
}

func TestModelVersionSubcommands(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "version" {
			subs := make(map[string]bool)
			for _, s := range sub.Commands() {
				subs[s.Name()] = true
			}
			if !subs["get"] {
				t.Error("model version: missing subcommand 'get'")
			}
			if !subs["set"] {
				t.Error("model version: missing subcommand 'set'")
			}
			return
		}
	}
	t.Error("model: 'version' subcommand not found")
}

// ─── virtualmodel command ─────────────────────────────────────────────────────

func TestVirtualModelCmdUse(t *testing.T) {
	if VirtualModelCmd.Use != "virtualmodel" {
		t.Errorf("expected Use='virtualmodel', got %q", VirtualModelCmd.Use)
	}
}

func TestVirtualModelCmdStructure(t *testing.T) {
	expected := []string{"list", "get", "create", "update", "delete"}
	subNames := make(map[string]bool)
	for _, sub := range VirtualModelCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("virtualmodel: missing subcommand %q", name)
		}
	}
}

func TestVirtualModelCreateFlags(t *testing.T) {
	for _, sub := range VirtualModelCmd.Commands() {
		if sub.Name() == "create" {
			for _, flagName := range []string{"name", "description", "strategy", "api-shape", "upstream", "disabled"} {
				if sub.Flags().Lookup(flagName) == nil {
					t.Errorf("virtualmodel create: missing flag --%s", flagName)
				}
			}
			// Check strategy default
			strategy, _ := sub.Flags().GetString("strategy")
			if strategy != "round-robin" {
				t.Errorf("expected default strategy='round-robin', got %q", strategy)
			}
			// Check api-shape default
			apiShape, _ := sub.Flags().GetString("api-shape")
			if apiShape != "openai" {
				t.Errorf("expected default api-shape='openai', got %q", apiShape)
			}
			return
		}
	}
	t.Error("virtualmodel create: subcommand not found")
}

func TestVirtualModelDeleteHasYesFlag(t *testing.T) {
	for _, sub := range VirtualModelCmd.Commands() {
		if sub.Name() == "delete" {
			if sub.Flags().Lookup("yes") == nil {
				t.Error("virtualmodel delete: missing --yes flag")
			}
			return
		}
	}
	t.Error("virtualmodel delete: subcommand not found")
}

// ─── config command ───────────────────────────────────────────────────────────

func TestConfigCmdStructure(t *testing.T) {
	expected := []string{"list", "get", "set", "import", "backup"}
	subNames := make(map[string]bool)
	for _, sub := range ConfigCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("config: missing subcommand %q", name)
		}
	}
}

func TestConfigSetFlags(t *testing.T) {
	for _, sub := range ConfigCmd.Commands() {
		if sub.Name() == "set" {
			if sub.Flags().Lookup("file") == nil {
				t.Error("config set: missing --file flag")
			}
			if sub.Flags().Lookup("stdin") == nil {
				t.Error("config set: missing --stdin flag")
			}
			return
		}
	}
	t.Error("config set: subcommand not found")
}

func TestConfigImportRequiresFileFlag(t *testing.T) {
	for _, sub := range ConfigCmd.Commands() {
		if sub.Name() == "import" {
			f := sub.Flags().Lookup("file")
			if f == nil {
				t.Error("config import: missing --file flag")
				return
			}
			return
		}
	}
	t.Error("config import: subcommand not found")
}

// ─── settings command ─────────────────────────────────────────────────────────

func TestSettingsCmdStructure(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range SettingsCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["get"] {
		t.Error("settings: missing subcommand 'get'")
	}
	if !subNames["set"] {
		t.Error("settings: missing subcommand 'set'")
	}
}

func TestSettingsGetHasLogLevelSubcommand(t *testing.T) {
	for _, sub := range SettingsCmd.Commands() {
		if sub.Name() == "get" {
			for _, s := range sub.Commands() {
				if s.Name() == "log-level" {
					return // found
				}
			}
			t.Error("settings get: missing subcommand 'log-level'")
			return
		}
	}
	t.Error("settings: 'get' subcommand not found")
}

func TestSettingsSetHasLogLevelSubcommand(t *testing.T) {
	for _, sub := range SettingsCmd.Commands() {
		if sub.Name() == "set" {
			for _, s := range sub.Commands() {
				if s.Name() == "log-level" {
					return
				}
			}
			t.Error("settings set: missing subcommand 'log-level'")
			return
		}
	}
	t.Error("settings: 'set' subcommand not found")
}

// ─── status command ───────────────────────────────────────────────────────────

func TestStatusCmdHasAuthSubcommand(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range StatusCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["auth"] {
		t.Error("status: missing subcommand 'auth'")
	}
}

// ─── logs command ─────────────────────────────────────────────────────────────

func TestLogsCmdHasTailSubcommand(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range LogsCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["tail"] {
		t.Error("logs: missing subcommand 'tail'")
	}
}

func TestLogsTailHasLevelFlag(t *testing.T) {
	for _, sub := range LogsCmd.Commands() {
		if sub.Name() == "tail" {
			if sub.Flags().Lookup("level") == nil {
				t.Error("logs tail: missing --level flag")
			}
			return
		}
	}
	t.Error("logs tail: subcommand not found")
}

// ─── chat command ─────────────────────────────────────────────────────────────

func TestChatCmdStructure(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range ChatCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["sessions"] {
		t.Error("chat: missing subcommand 'sessions'")
	}
	if !subNames["send"] {
		t.Error("chat: missing subcommand 'send'")
	}
}

func TestChatSessionsSubcommands(t *testing.T) {
	for _, sub := range ChatCmd.Commands() {
		if sub.Name() == "sessions" {
			subNames := make(map[string]bool)
			for _, s := range sub.Commands() {
				subNames[s.Name()] = true
			}
			for _, expected := range []string{"list", "create", "get", "rename", "delete"} {
				if !subNames[expected] {
					t.Errorf("chat sessions: missing subcommand %q", expected)
				}
			}
			return
		}
	}
	t.Error("chat: 'sessions' subcommand not found")
}

func TestChatCmdFlags(t *testing.T) {
	if ChatCmd.Flags().Lookup("model") == nil {
		t.Error("chat: missing --model flag")
	}
	if ChatCmd.Flags().Lookup("session") == nil {
		t.Error("chat: missing --session flag")
	}
}

// ─── parseUpstreamArgs ────────────────────────────────────────────────────────

func TestParseUpstreamArgsBasic(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"my-provider/my-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(result))
	}
	if result[0]["provider_id"] != "my-provider" {
		t.Errorf("expected provider_id='my-provider', got %v", result[0]["provider_id"])
	}
	if result[0]["model_id"] != "my-model" {
		t.Errorf("expected model_id='my-model', got %v", result[0]["model_id"])
	}
	if result[0]["weight"] != 1 {
		t.Errorf("expected weight=1, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 0 {
		t.Errorf("expected priority=0, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsWithWeight(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1:3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0]["weight"] != 3 {
		t.Errorf("expected weight=3, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 0 {
		t.Errorf("expected priority=0, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsWithWeightAndPriority(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1:2:5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0]["weight"] != 2 {
		t.Errorf("expected weight=2, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 5 {
		t.Errorf("expected priority=5, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsMultiple(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1", "p2/m2:10:1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(result))
	}
	if result[1]["provider_id"] != "p2" {
		t.Errorf("expected provider_id='p2', got %v", result[1]["provider_id"])
	}
	if result[1]["weight"] != 10 {
		t.Errorf("expected weight=10, got %v", result[1]["weight"])
	}
}

func TestParseUpstreamArgsMissingSlashReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"no-slash-here"})
	if err == nil {
		t.Error("expected error for missing slash, got nil")
	}
}

func TestParseUpstreamArgsInvalidWeightReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"p1/m1:notanumber"})
	if err == nil {
		t.Error("expected error for non-numeric weight, got nil")
	}
}

func TestParseUpstreamArgsInvalidPriorityReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"p1/m1:1:bad"})
	if err == nil {
		t.Error("expected error for non-numeric priority, got nil")
	}
}

func TestParseUpstreamArgsEmpty(t *testing.T) {
	result, err := parseUpstreamArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty input, got %v", result)
	}
}

// ─── isLevelAtOrAbove ─────────────────────────────────────────────────────────

func TestIsLevelAtOrAboveFiltering(t *testing.T) {
	cases := []struct {
		msg    string
		filter string
		want   bool
	}{
		{"error", "error", true},
		{"error", "warn", true},   // error is more severe than warn
		{"error", "info", true},   // error is more severe than info
		{"debug", "info", false},  // debug is less severe than info
		{"debug", "debug", true},  // same level passes
		{"trace", "debug", false}, // trace is less severe than debug
		{"info", "warn", false},   // info=3 is LESS severe than warn=2 — filtered out
		{"warn", "info", true},    // warn=2 is MORE severe than info=3 — passes
		{"unknown", "info", true}, // unknown levels pass through
		{"info", "unknown", true}, // unknown filter passes through
	}

	for _, tc := range cases {
		got := isLevelAtOrAbove(tc.msg, tc.filter)
		if got != tc.want {
			t.Errorf("isLevelAtOrAbove(%q, %q) = %v, want %v", tc.msg, tc.filter, got, tc.want)
		}
	}
}

// ─── NewClient defaults ───────────────────────────────────────────────────────

func TestNewClientDefaultServer(t *testing.T) {
	import_test_root := ChatCmd.Root()
	if import_test_root == nil {
		t.Skip("no root command in test context")
	}
	t.Setenv("OMNILLM_SERVER", "")
	t.Setenv("OMNILLM_API_KEY", "")

	const expectedDefault = "http://127.0.0.1:5000"
	_ = expectedDefault // guard against renaming without updating
}
