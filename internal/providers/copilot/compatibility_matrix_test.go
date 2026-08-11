package copilot

import (
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCompatibilityManifestCopilotShapeRows(t *testing.T) {
	provider := NewGitHubCopilotProvider("compat-shapes", "")
	provider.shapeCache = map[string]copilotAPIShape{
		"claude-opus-4.7": shapeChat,
		"gpt-5.5":         shapeResponses,
	}
	adapter := &CopilotAdapter{provider: provider}

	for _, row := range testcompat.ProviderStrategies() {
		if row.Provider != "github-copilot" || row.UpstreamShape == testcompat.ShapeClaude {
			continue
		}
		got := adapter.selectShape(row.Model, &cif.CanonicalRequest{Model: row.Model})
		if string(got) != string(row.UpstreamShape) {
			t.Errorf("%s selected %q, want %q", row.Name, got, row.UpstreamShape)
		}
	}
}
