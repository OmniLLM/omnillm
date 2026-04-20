package alibaba

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// Adapter implements types.ProviderAdapter using openaicompat for HTTP.
type Adapter struct {
	provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return RemapModel(model) }

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	cr, err := a.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	return openaicompat.Execute(ctx, ChatURL(a.provider.baseURL), Headers(a.provider.token, false, a.provider.config), cr)
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	cr, err := a.buildRequest(request, true)
	if err != nil {
		return nil, err
	}
	return openaicompat.Stream(ctx, ChatURL(a.provider.baseURL), Headers(a.provider.token, true, a.provider.config), cr)
}

// buildRequest converts a CIF request into an openaicompat.ChatRequest with
// DashScope-specific extras (enable_thinking, stream_options).
func (a *Adapter) buildRequest(request *cif.CanonicalRequest, stream bool) (*openaicompat.ChatRequest, error) {
	model := a.RemapModel(request.Model)

	defTemp := 0.55
	defTopP := 1.0

	extras := map[string]interface{}{}
	if IsReasoningModel(model) && len(request.Tools) == 0 {
		extras["enable_thinking"] = true
	}

	cfg := openaicompat.Config{
		DefaultTemperature:   &defTemp,
		DefaultTopP:          &defTopP,
		IncludeUsageInStream: stream,
		Extras:               extras,
	}
	return openaicompat.BuildChatRequest(model, request, stream, cfg)
}

// GetModels returns the available models for this Alibaba instance.
func GetModels(instanceID, token, baseURL string, config map[string]interface{}) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated")
	}
	resp, err := FetchModelsFromAPI(instanceID, token, baseURL, config)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}
	log.Warn().Err(err).Str("provider", instanceID).Msg("alibaba: falling back to hardcoded model list")
	return GetModelsHardcoded(instanceID), nil
}

// GetModelsHardcoded returns the hardcoded model catalog.
func GetModelsHardcoded(instanceID string) *types.ModelsResponse {
	result := make([]types.Model, len(Models))
	for i, m := range Models {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// FetchModelsFromAPI fetches available models from the DashScope API.
func FetchModelsFromAPI(instanceID, token, baseURL string, _ map[string]interface{}) (*types.ModelsResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to create models request: %w", err)
	}
	for k, v := range Headers(token, false, nil) {
		req.Header.Set(k, v)
	}

	resp, err := alibabaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alibaba: models fetch failed (%d)", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to read models response: %w", err)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("alibaba: failed to decode models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" || !IsChatCompletionsModel(item.ID) {
			continue
		}
		m := types.Model{ID: item.ID, Name: item.ID, Provider: instanceID}
		if meta, ok := ModelMetadata(item.ID); ok {
			if meta.Name != "" {
				m.Name = meta.Name
			}
			m.Description = meta.Description
			m.Capabilities = meta.Capabilities
			m.MaxTokens = meta.MaxTokens
		}
		models = append(models, m)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}
