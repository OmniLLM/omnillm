package modelscope

import (
	"context"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

// Adapter implements types.ProviderAdapter for ModelScope.
// Unlike the DashScope adapter, it does NOT inject enable_thinking into requests.
type Adapter struct {
	provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return model }

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	cr, err := a.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	return openaicompat.Execute(ctx, chatURL(a.provider.baseURL), headers(a.provider.token), cr)
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	// ModelScope streaming can be unreliable for OmniLLM's bridged request shapes.
	// Execute upstream non-streaming and let the route layer re-stream the
	// completed CIF response to the client.
	response, err := a.Execute(ctx, request)
	if err != nil {
		return nil, err
	}
	return shared.StreamResponse(response), nil
}

// buildRequest converts a CIF request into an openaicompat.ChatRequest.
// Critically, this does NOT inject enable_thinking — ModelScope models do not
// support it and return empty responses when it is present with tools.
func (a *Adapter) buildRequest(request *cif.CanonicalRequest, stream bool) (*openaicompat.ChatRequest, error) {
	model := a.RemapModel(request.Model)

	if model == "" {
		return nil, fmt.Errorf("modelscope: model is required")
	}

	defTemp := 0.55
	defTopP := 1.0

	cfg := openaicompat.Config{
		DefaultTemperature:   &defTemp,
		DefaultTopP:          &defTopP,
		IncludeUsageInStream: stream,
		// No Extras — no enable_thinking, no DashScope-specific parameters.
	}
	return openaicompat.BuildChatRequest(model, request, stream, cfg)
}
