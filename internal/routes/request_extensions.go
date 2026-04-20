package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/providers/types"
)

type upstreamAPIReporter interface {
	UpstreamAPI(request *cif.CanonicalRequest, model string) string
}

func ensureRequestExtensions(request *cif.CanonicalRequest) *cif.Extensions {
	if request == nil {
		return nil
	}
	if request.Extensions == nil {
		request.Extensions = &cif.Extensions{}
	}
	return request.Extensions
}

func setInboundAPIShape(request *cif.CanonicalRequest, apiShape string) {
	extensions := ensureRequestExtensions(request)
	if extensions == nil {
		return
	}
	extensions.InboundAPIShape = &apiShape
}

func detectUpstreamAPI(providerID string, adapter types.ProviderAdapter, request *cif.CanonicalRequest, model string) string {
	if reporter, ok := adapter.(upstreamAPIReporter); ok {
		return reporter.UpstreamAPI(request, model)
	}
	return upstreamAPIForProvider(providerID, model)
}
