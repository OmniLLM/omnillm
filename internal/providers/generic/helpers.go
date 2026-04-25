package generic

import (
	"net/url"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"

	alibabapkg "omnillm/internal/providers/alibaba"
	antigravitypkg "omnillm/internal/providers/antigravity"
	googlepkg "omnillm/internal/providers/google"
)

func normalizeToolArguments(raw interface{}) map[string]interface{} {
	return shared.NormalizeToolArguments(raw)
}

func cifMessagesToGemini(messages []cif.CIFMessage) []map[string]interface{} {
	return shared.CIFMessagesToGemini(messages)
}

func sanitizeGeminiSchema(schema map[string]interface{}) map[string]interface{} {
	return shared.SanitizeGeminiSchema(schema)
}

func googleStopReason(reason string) cif.CIFStopReason {
	return googlepkg.StopReason(reason)
}

func antigravityStopReason(reason string) cif.CIFStopReason {
	return antigravitypkg.StopReason(reason)
}

func parseGoogleGeminiSSE(body interface{ Read([]byte) (int, error); Close() error }, eventCh chan cif.CIFStreamEvent) {
	googlepkg.ParseGeminiSSE(body, eventCh)
}

func parseAntigravitySSE(body interface{ Read([]byte) (int, error); Close() error }, eventCh chan cif.CIFStreamEvent) {
	antigravitypkg.ParseAntigravitySSE(body, eventCh)
}

func (p *GenericProvider) getAlibabaModels() (*types.ModelsResponse, error) {
	return alibabapkg.GetModels(p.instanceID, p.token, p.baseURL, p.config)
}

func (p *GenericProvider) getAlibabaModelsHardcoded() *types.ModelsResponse {
	return alibabapkg.GetModelsHardcoded(p.instanceID)
}

func (p *GenericProvider) fetchAlibabaModelsFromAPI() (*types.ModelsResponse, error) {
	return alibabapkg.FetchModelsFromAPI(p.instanceID, p.token, p.baseURL, p.config)
}

func isAlibabaChatCompletionsModel(modelID string) bool {
	return alibabapkg.IsChatCompletionsModel(modelID)
}

func alibabaModelMetadata(modelID string) (types.Model, bool) {
	return alibabapkg.ModelMetadata(modelID)
}

func normalizeAlibabaBaseURL(config map[string]interface{}) string {
	return alibabapkg.NormalizeBaseURL(config)
}

func normalizeAlibabaAPIPlan(plan string) string {
	return alibabapkg.NormalizeAPIPlan(plan)
}

func defaultAlibabaAPIBaseURL(plan, region string) string {
	return alibabapkg.DefaultAPIBaseURL(plan, region)
}

func alibabaAPIKeyProviderName(config map[string]interface{}) string {
	return alibabapkg.APIKeyProviderName(config)
}

func deriveAzureName(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if strings.HasSuffix(host, ".openai.azure.com") {
		resource := strings.TrimSuffix(host, ".openai.azure.com")
		if resource != "" {
			return resource
		}
	}
	if strings.HasSuffix(host, ".cognitiveservices.azure.com") {
		resource := strings.TrimSuffix(host, ".cognitiveservices.azure.com")
		if resource != "" {
			return resource
		}
	}
	return ""
}

func ensureAlibabaBaseURL(raw string) string {
	return alibabapkg.EnsureBaseURL(raw)
}

func (p *GenericProvider) detectRegion() string {
	if strings.Contains(strings.ToLower(p.instanceID), "global") {
		return "global"
	}
	if p.baseURL != "" && strings.Contains(strings.ToLower(p.baseURL), "dashscope-intl") {
		return "global"
	}
	return "china"
}

func (p *GenericProvider) alibabaHeaders(stream bool) map[string]string {
	token := p.ensureFreshAlibabaToken()
	return alibabapkg.Headers(token, stream, p.config)
}

func stringSliceFromConfig(config map[string]interface{}, key string) []string {
	if config == nil {
		return nil
	}
	raw, exists := config[key]
	if !exists {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func (p *GenericProvider) fetchGoogleModels() (*types.ModelsResponse, error) {
	return googlepkg.FetchModels(p.instanceID, p.token, p.baseURL)
}
