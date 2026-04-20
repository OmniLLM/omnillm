package routes

import (
	"net/http"
	"omnillm/internal/cif"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func captureIncomingHeaders(c *gin.Context, request *cif.CanonicalRequest) {
	if request == nil || log.Logger.GetLevel() > zerolog.DebugLevel {
		return
	}
	request.IncomingHeaders = make(map[string]string)
	for k, v := range c.Request.Header {
		if len(v) > 0 {
			request.IncomingHeaders[k] = v[0]
		}
	}
}

func prepareCanonicalRequest(c *gin.Context, request *cif.CanonicalRequest, apiShape string) string {
	captureIncomingHeaders(c, request)
	setInboundAPIShape(request, apiShape)
	originalModel := ""
	if request != nil {
		originalModel = request.Model
		logRequestReceived(c.GetString("request_id"), apiShape, request)
	}
	return originalModel
}

func logRequestReceived(requestID, apiShape string, request *cif.CanonicalRequest) {
	if request == nil {
		return
	}
	log.Info().
		Str("request_id", requestID).
		Str("api_shape", apiShape).
		Str("model_requested", request.Model).
		Int("messages", len(request.Messages)).
		Int("tools", len(request.Tools)).
		Bool("stream", request.Stream).
		Msg("\x1b[33m-->\x1b[0m REQUEST")
}

func writeProviderFailure(c *gin.Context, defaultType string, lastErr error) {
	errMsg := "All providers failed"
	if lastErr != nil {
		log.Error().Err(lastErr).Msg("Provider failure details")
	}
	c.JSON(providerFailureStatus(lastErr), gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    providerFailureType(defaultType, lastErr),
		},
	})
}

func writeResolveProvidersError(c *gin.Context, err error, errorType string) {
	log.Error().Err(err).Msg("Failed to resolve providers")
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"message": "Failed to resolve providers",
			"type":    errorType,
		},
	})
}
