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
		logRequestReceived(c, c.GetString("request_id"), apiShape, request)
	}
	return originalModel
}

func logRequestReceived(c *gin.Context, requestID, apiShape string, request *cif.CanonicalRequest) {
	if request == nil {
		return
	}
	client := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	log.Info().
		Str("request_id", requestID).
		Str("api_shape", apiShape).
		Str("model_requested", request.Model).
		Int("messages", len(request.Messages)).
		Int("tools", len(request.Tools)).
		Bool("stream", request.Stream).
		Str("client", client).
		Str("user_agent", userAgent).
		Msg("\x1b[33m-->\x1b[0m REQUEST")
}

// recordAttemptedCandidate remembers the most recently attempted candidate so
// the terminal failure log can name the provider and upstream model that
// actually failed. Without this the error log carries only an error string and
// cannot be correlated with the gateway request log.
func recordAttemptedCandidate(c *gin.Context, providerID, upstreamModel string) {
	if c == nil {
		return
	}
	c.Set("last_attempt_provider", providerID)
	c.Set("last_attempt_upstream_model", upstreamModel)
}

func writeProviderFailure(c *gin.Context, defaultType string, lastErr error) {
	// A client disconnect is not a provider failure.  Log it at info level and
	// skip the JSON body entirely -- nobody is listening on the other end, and
	// emitting a 502 here both pollutes error logs and misattributes the cause.
	if isClientCanceled(c, lastErr) {
		log.Info().Str("request_id", c.GetString("request_id")).Msg("Client canceled request before completion")
		c.Abort()
		return
	}

	errMsg := "All providers failed"
	if lastErr != nil {
		log.Error().Err(lastErr).
			Str("request_id", c.GetString("request_id")).
			Str("provider", c.GetString("last_attempt_provider")).
			Str("upstream_model", c.GetString("last_attempt_upstream_model")).
			Msg("Provider failure details")
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
