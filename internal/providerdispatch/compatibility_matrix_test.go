package providerdispatch

import (
	"strings"
	"testing"

	"omnillm/internal/testcompat"
)

func TestCompatibilityProviderManifestMatchesDispatchDefaults(t *testing.T) {
	for _, row := range testcompat.ProviderStrategies() {
		if row.UpstreamShape == testcompat.ShapeClaude || row.UpstreamShape == testcompat.ShapeGemini ||
			row.Provider == "github-copilot" || row.Provider == "openai-compatible" || row.Provider == "openai" {
			// Dynamic and provider-native strategies are selected by their adapters.
			continue
		}
		got := strings.ReplaceAll(DefaultUpstreamAPI(string(row.Provider), row.Model), ".", "_")
		want := string(row.UpstreamShape)
		if got != want {
			t.Errorf("%s default upstream API = %q, want %q", row.Name, got, want)
		}
	}
}
