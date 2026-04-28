package shared

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const openAIMaxUserIDLength = 64

// RandomID generates a random hexadecimal ID string.
func RandomID() string {
	return fmt.Sprintf("%x%x", time.Now().UnixNano(), rand.Int63())
}

// FirstString returns the first non-empty string value for the given keys in a map.
func FirstString(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

// ShortTokenSuffix returns the last 5 characters of a token for display purposes.
func ShortTokenSuffix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) >= 5 {
		return trimmed[len(trimmed)-5:]
	}
	if trimmed == "" {
		return "token"
	}
	return trimmed
}

// TruncateOpenAIUserID trims whitespace and enforces the OpenAI-compatible
// user identifier length limit before forwarding requests upstream.
func TruncateOpenAIUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if len(trimmed) <= openAIMaxUserIDLength {
		return trimmed
	}
	return trimmed[:openAIMaxUserIDLength]
}

// IsReasoningModel returns true for models that do not support the temperature
// or top_p sampling parameters (OpenAI o-series, gpt-5 family, and Azure
// Responses API-only models like gpt-5.4-pro / gpt-5.1-codex-max).
// These models use internal reasoning and reject temperature/top_p with a 400.
func IsReasoningModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	// o-series reasoning models: o1, o1-mini, o3, o3-mini, o4-mini, …
	if strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") {
		return true
	}
	// gpt-5 family and later generations reject temperature
	if strings.HasPrefix(lower, "gpt-5") || strings.HasPrefix(lower, "gpt-6") {
		return true
	}
	// Azure Responses API-specific models known to reject temperature
	if strings.Contains(lower, "gpt-5.4-pro") || strings.Contains(lower, "gpt-5.1-codex-max") {
		return true
	}
	return false
}
