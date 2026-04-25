// Package generic provides a generic provider implementation for alibaba, antigravity, azure-openai, and google.
// It acts as a facade that delegates to provider-specific sub-packages while maintaining
// backward compatibility for callers that use *GenericProvider type assertions.
package generic

import (
	"omnillm/internal/providers/types"
	"sync"

	alibabapkg "omnillm/internal/providers/alibaba"
	antigravitypkg "omnillm/internal/providers/antigravity"
	azurepkg "omnillm/internal/providers/azure"
	kimipkg "omnillm/internal/providers/kimi"
)

// ─── Model catalogs (kept for white-box test access) ─────────────────────────

var providerModels = map[string][]types.Model{
	"antigravity":  antigravitypkg.Models,
	"alibaba":      alibabapkg.Models,
	"azure-openai": azurepkg.DefaultModels,
	"kimi":         kimipkg.Models,
	"google": {
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-flash-preview-05-20", Name: "Gemini 2.5 Flash Preview 05-20", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.5-pro-preview-06-05", Name: "Gemini 2.5 Pro Preview 06-05", MaxTokens: 65536, Provider: "google"},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash Lite", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", MaxTokens: 8192, Provider: "google"},
		{ID: "gemini-1.5-flash-8b", Name: "Gemini 1.5 Flash-8B", MaxTokens: 8192, Provider: "google"},
	},
}

var providerBaseURLs = map[string]string{
	"antigravity":  antigravitypkg.ProductionBaseURL,
	"alibaba":      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	"azure-openai": "",
	"kimi":         "https://api.moonshot.cn/v1",
	"google":       "https://generativelanguage.googleapis.com",
}

const alibabaUserAgent = alibabapkg.UserAgent

// ─── Types ────────────────────────────────────────────────────────────────────

// GenericProvider is a minimal provider implementation for non-copilot providers.
// The struct fields are kept identical to the original to preserve backward compatibility
// for callers that use *GenericProvider type assertions (e.g., admin.go).
type GenericProvider struct {
	id         string
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	// configOnce ensures config is loaded from the database exactly once,
	// even under concurrent requests.  Replaces the racy configLoaded bool.
	configOnce sync.Once
}

// GenericAdapter wraps GenericProvider for the ProviderAdapter interface.
type GenericAdapter struct {
	provider *GenericProvider
}
