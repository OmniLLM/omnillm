package routes

import (
	"strings"

	"omnillm/internal/translation/toolarguments"

	"github.com/rs/zerolog/log"
)

func isClaudeCodeUserAgent(userAgent string) bool {
	for _, product := range strings.Fields(userAgent) {
		name, version, ok := strings.Cut(product, "/")
		if ok && name == "claude-cli" && version != "" && !strings.ContainsAny(version, "()/,;") {
			return true
		}
	}
	return false
}

func claudeCodeToolArgumentPolicy(userAgent, requestID, model, provider string, stream, cache bool) toolarguments.CompatibilityPolicy {
	if !isClaudeCodeUserAgent(userAgent) {
		return toolarguments.CompatibilityPolicy{}
	}
	return toolarguments.CompatibilityPolicy{
		ClaudeCodePlanAgentType: true,
		OnRepair: func(repair toolarguments.Repair) {
			log.Warn().
				Str("request_id", requestID).
				Str("api_shape", "anthropic").
				Str("model", model).
				Str("provider", provider).
				Str("tool_call_id", repair.ToolCallID).
				Str("tool_name", repair.ToolName).
				Bool("stream", stream).
				Bool("cache", cache).
				Str("repair_reason", repair.Reason).
				Msg("Claude Code tool argument compatibility repair applied")
		},
	}
}
