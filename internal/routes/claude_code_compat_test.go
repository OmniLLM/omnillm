package routes

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/translation/toolarguments"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestIsClaudeCodeUserAgent(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"claude-cli/2.1.235 (external, cli)", true},
		{"prefix/1 claude-cli/2.1.235 suffix/2", true},
		{"claude-cli/", false},
		{"claude-cli/2.1.235,", false},
		{"my-claude-cli/2.1.235", false},
		{"codex-tui/0.148.0", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isClaudeCodeUserAgent(tt.value); got != tt.want {
			t.Errorf("isClaudeCodeUserAgent(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestClaudeCodeCompatibilityPolicyAndDiagnostic(t *testing.T) {
	original := log.Logger
	defer func() { log.Logger = original }()
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)

	policy := claudeCodeToolArgumentPolicy("claude-cli/2.1.235 (external, cli)", "req", "gpt-5.6-sol", "copilot", true, false)
	if !policy.ClaudeCodePlanAgentType || policy.OnRepair == nil {
		t.Fatal("Claude Code policy was not selected")
	}
	policy.OnRepair(toolarguments.Repair{Reason: toolarguments.ClaudeCodePlanAgentTypeRepair, ToolCallID: "call", ToolName: "Agent"})

	var record map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode diagnostic: %v: %s", err, output.String())
	}
	for key, want := range map[string]interface{}{
		"request_id": "req", "api_shape": "anthropic", "model": "gpt-5.6-sol", "provider": "copilot",
		"tool_call_id": "call", "tool_name": "Agent", "repair_reason": toolarguments.ClaudeCodePlanAgentTypeRepair,
	} {
		if record[key] != want {
			t.Errorf("diagnostic %s = %#v, want %#v", key, record[key], want)
		}
	}
	for _, forbidden := range []string{"arguments", "subagent_type", "prompt", toolarguments.ClaudeCodePlanAgentTypeSentinel} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, output.String())
		}
	}
	if policy := claudeCodeToolArgumentPolicy("codex-tui/0.148.0", "req", "model", "provider", false, false); policy.ClaudeCodePlanAgentType || policy.OnRepair != nil {
		t.Fatal("non-Claude Code caller selected compatibility policy")
	}
}

func TestClaudeCodeCompatibilityRepairsCachedResponse(t *testing.T) {
	request := &cif.CanonicalRequest{Tools: []cif.CIFTool{{
		Name: "Agent",
		ParametersSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"subagent_type": map[string]interface{}{"type": "string", "enum": []interface{}{"Plan", "Explore"}},
				"prompt":        map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"subagent_type", "prompt"},
		},
	}}}
	cached := &cif.CanonicalResponse{Content: []cif.CIFContentPart{cif.CIFToolCallPart{
		Type: "tool_call", ToolCallID: "call", ToolName: "Agent",
		ToolArguments: map[string]interface{}{"subagent_type": toolarguments.ClaudeCodePlanAgentTypeSentinel, "prompt": "keep"},
	}}}
	policy := claudeCodeToolArgumentPolicy("claude-cli/2.1.235", "req", "model", "response-cache", false, true)
	repaired := toolarguments.NormalizeResponseWithPolicy(sanitizeResponseCacheHit(cached, request), request.Tools, policy)
	arguments := repaired.Content[0].(cif.CIFToolCallPart).ToolArguments
	if arguments["subagent_type"] != "Plan" || arguments["prompt"] != "keep" {
		t.Fatalf("cached arguments = %#v", arguments)
	}
	if cached.Content[0].(cif.CIFToolCallPart).ToolArguments["subagent_type"] != toolarguments.ClaudeCodePlanAgentTypeSentinel {
		t.Fatal("cached response was mutated")
	}
}
