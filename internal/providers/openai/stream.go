package openai

import (
	"context"
	"fmt"

	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
)

// Stream issues a streaming Responses request against the Codex backend.
//
// The upstream requires stream:true — non-streaming requests are rejected with
// {"detail":"Stream must be set to true"} — so this is the only transport, and
// Execute collects from it.
func Stream(ctx context.Context, p *Provider, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if p == nil {
		return nil, fmt.Errorf("openai: nil provider")
	}
	if request == nil {
		return nil, fmt.Errorf("openai: nil request")
	}

	headers := p.GetHeaders(false)
	if headers["Authorization"] == "Bearer " {
		return nil, fmt.Errorf("openai: not authenticated — sign in via the ChatGPT browser flow")
	}

	model := p.RemapModel(request.Model)
	payload := buildCodexPayload(model, request)

	return openaicompat.StreamResponses(ctx, p.GetBaseURL()+"/responses", headers, payload)
}

// buildCodexPayload builds the Responses payload and strips the fields the
// Codex backend rejects.
func buildCodexPayload(model string, request *cif.CanonicalRequest) map[string]interface{} {
	payload := openaicompat.BuildResponsesPayload(model, request, true, openaicompat.ResponsesConfig{
		PromptCacheMode: openaicompat.PromptCacheOpenAINative,
		Extras: map[string]interface{}{
			// The Codex backend is stateless for our purposes and rejects
			// stream:false, so pin both flags explicitly.
			"store":  false,
			"stream": true,
		},
	})

	// The Codex backend rejects max_output_tokens with
	// {"detail":"Unsupported parameter: max_output_tokens"}.
	delete(payload, "max_output_tokens")

	// It likewise rejects the standard Responses "user" field with
	// {"detail":"Unsupported parameter: user"}. This is populated from
	// metadata.user_id on Anthropic-shape requests (Claude Code always sends
	// it) and from the top-level "user" field on OpenAI-shape requests.
	delete(payload, "user")

	return payload
}
