package copilot

import (
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/testcompat"
)

func TestCopilotResponsesPayloadPreservesCodexToolName(t *testing.T) {
	provider := NewGitHubCopilotProvider("compat-tool-name", "")
	adapter := &CopilotAdapter{provider: provider}
	description := "Run a shell command"
	payload := adapter.buildResponsesPayload(&cif.CanonicalRequest{
		Model: "gpt-5.6-sol",
		Tools: []cif.CIFTool{{
			Name:             "exec",
			Description:      &description,
			ParametersSchema: map[string]interface{}{"type": "object"},
			ToolKind:         cif.CIFToolKindCustom,
			Format:           map[string]interface{}{"type": "text"},
		}},
	}, false)

	tools := payload["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["name"] != "exec" || tools[0]["type"] != "custom" {
		t.Fatalf("Copilot Responses tools = %#v", tools)
	}
	if _, hasParameters := tools[0]["parameters"]; hasParameters {
		t.Fatalf("custom tool unexpectedly has function parameters: %#v", tools[0])
	}
}

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
