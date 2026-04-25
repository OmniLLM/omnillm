package copilot

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"omnillm/internal/cif"
)

func newCopilotToolNameMapper(request *cif.CanonicalRequest) *copilotToolNameMapper {
	mapper := &copilotToolNameMapper{
		upstreamByCanonical: make(map[string]string),
		canonicalByUpstream: make(map[string]string),
	}
	if request == nil {
		return mapper
	}

	register := func(name string) {
		if name == "" {
			return
		}
		if _, exists := mapper.upstreamByCanonical[name]; exists {
			return
		}

		alias := normalizeCopilotToolName(name)
		if alias == name {
			return
		}

		mapper.upstreamByCanonical[name] = alias
		mapper.canonicalByUpstream[alias] = name
	}

	for _, tool := range request.Tools {
		register(tool.Name)
	}

	switch choice := request.ToolChoice.(type) {
	case map[string]interface{}:
		if functionName, _ := choice["functionName"].(string); functionName != "" {
			register(functionName)
		}
		if function, ok := choice["function"].(map[string]interface{}); ok {
			if name, _ := function["name"].(string); name != "" {
				register(name)
			}
		}
	}

	for _, message := range request.Messages {
		switch msg := message.(type) {
		case cif.CIFUserMessage:
			for _, part := range msg.Content {
				if toolResult, ok := part.(cif.CIFToolResultPart); ok {
					register(toolResult.ToolName)
				}
			}
		case cif.CIFAssistantMessage:
			for _, part := range msg.Content {
				if toolCall, ok := part.(cif.CIFToolCallPart); ok {
					register(toolCall.ToolName)
				}
			}
		}
	}

	if len(mapper.upstreamByCanonical) > 0 {
		// logging is optional; keep lightweight here
	}

	return mapper
}

func (m *copilotToolNameMapper) toUpstream(name string) string {
	if m == nil {
		return name
	}
	if aliased, ok := m.upstreamByCanonical[name]; ok {
		return aliased
	}
	return name
}

func (m *copilotToolNameMapper) fromUpstream(name string) string {
	if m == nil {
		return name
	}
	if original, ok := m.canonicalByUpstream[name]; ok {
		return original
	}
	return name
}

func normalizeCopilotToolName(name string) string {
	if name == "" {
		return ""
	}

	sanitized := copilotToolNamePattern.ReplaceAllString(name, "_")
	sanitized = strings.Trim(sanitized, "_-")
	if sanitized == "" {
		sanitized = "tool"
	}

	if sanitized == name && len(sanitized) <= copilotMaxToolNameLength {
		return name
	}

	sum := sha1.Sum([]byte(name))
	hashSuffix := fmt.Sprintf("%x", sum[:])[:12]
	maxPrefixLength := copilotMaxToolNameLength - len(hashSuffix) - 1
	if maxPrefixLength < 1 {
		maxPrefixLength = 1
	}
	if len(sanitized) > maxPrefixLength {
		sanitized = sanitized[:maxPrefixLength]
	}
	sanitized = strings.TrimRight(sanitized, "_-")
	if sanitized == "" {
		sanitized = "tool"
	}
	if len(sanitized) > maxPrefixLength {
		sanitized = sanitized[:maxPrefixLength]
	}

	return fmt.Sprintf("%s_%s", sanitized, hashSuffix)
}

func (a *CopilotAdapter) convertOpenAIStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "stop":
		return cif.StopReasonEndTurn
	case "length":
		return cif.StopReasonMaxTokens
	case "tool_calls":
		return cif.StopReasonToolUse
	case "content_filter":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}
