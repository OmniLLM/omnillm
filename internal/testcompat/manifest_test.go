package testcompat

import "testing"

func TestSemanticScenarioManifestCompleteness(t *testing.T) {
	required := []string{
		"plain", "one-tool-cycle", "three-sequential-tool-cycles", "parallel-interleaved-calls",
		"mixed-text-and-calls", "empty-arguments", "large-result", "tool-error",
		"thinking-before-tool", "normal-completion", "abrupt-failure", "cancellation",
	}
	seen := make(map[string]bool)
	for _, scenario := range SemanticScenarios() {
		if scenario.Name == "" {
			t.Fatal("semantic scenario has an empty name")
		}
		if seen[scenario.Name] {
			t.Fatalf("duplicate semantic scenario %q", scenario.Name)
		}
		seen[scenario.Name] = true
	}
	for _, name := range required {
		if !seen[name] {
			t.Errorf("semantic scenario %q has no deterministic fixture", name)
		}
	}
}

func TestProviderStrategyManifestCompleteness(t *testing.T) {
	rows := ProviderStrategies()
	providers := make(map[string]int)
	strategies := make(map[string]bool)
	for _, row := range rows {
		if row.Name == "" || row.Model == "" || row.UpstreamShape == "" || row.StreamingStrategy == "" {
			t.Errorf("incomplete provider strategy row: %#v", row)
		}
		if strategies[row.Name] {
			t.Errorf("duplicate provider strategy name %q", row.Name)
		}
		strategies[row.Name] = true
		providers[string(row.Provider)]++
		for capabilityName, capability := range map[string]Capability{"tools": row.Tools, "streaming": row.Streaming} {
			if !capability.Supported && capability.NotApplicableReason == "" {
				t.Errorf("%s %s capability is unsupported without an explicit not-applicable reason", row.Name, capabilityName)
			}
		}
	}
	for _, provider := range SupportedProviderTypes() {
		if providers[string(provider)] == 0 {
			t.Errorf("supported provider %q has no deterministic strategy row", provider)
		}
	}
	for _, required := range []string{"copilot-chat", "copilot-responses", "copilot-claude-stream", "openai-compatible-chat", "openai-compatible-responses"} {
		if !strategies[required] {
			t.Errorf("dynamic upstream strategy %q has no deterministic row", required)
		}
	}
}
