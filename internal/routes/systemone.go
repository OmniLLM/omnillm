package routes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/systemone"
)

func SetupSystemOneRoutes(router *gin.RouterGroup) { router.POST("/systemone", handleSystemOne) }
func writeSystemOneError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": "systemone_error"}})
}
func handleSystemOne(c *gin.Context) {
	start := time.Now()
	body, err := readGatewayRequestBody(c.Request.Body)
	if err != nil {
		status, message := gatewayRequestBodyError(err)
		writeSystemOneError(c, status, message)
		return
	}
	request, err := systemone.ParseRequest(body)
	if err != nil {
		writeSystemOneError(c, http.StatusBadRequest, err.Error())
		return
	}
	requestID := c.GetString("request_id")
	attempts, err := resolveRequestedModels(requestID, request.Model)
	if err != nil {
		writeSystemOneError(c, http.StatusBadRequest, "invalid provider reference")
		return
	}
	var lastErr error
	unavailable := true
	visited := make(map[string]bool)
	for _, attempt := range attempts {
		if c.Request.Context().Err() != nil {
			return
		}
		if attempt.OnlyIfPreviousUnavailable && !unavailable {
			continue
		}
		resolved, err := modelrouting.ResolveEvaluationProviders(
			attempt.RequestedModel, attempt.NormalizedModel, attempt.ProviderID, modelCache,
		)
		if err != nil {
			lastErr = err
			continue
		}
		unavailable = len(resolved.CandidateProviders) == 0
		for _, provider := range resolved.CandidateProviders {
			if c.Request.Context().Err() != nil {
				return
			}
			id := provider.GetInstanceID()
			if visited[id] {
				continue
			}
			visited[id] = true
			evaluator, ok := provider.(systemone.Evaluator)
			if !ok {
				continue
			}
			upstreamRequest := *request
			upstreamRequest.Model = attempt.RequestedModel
			result, err := evaluator.Evaluate(c.Request.Context(), &upstreamRequest)
			if err != nil {
				lastErr = err
				continue
			}
			recordUsage(
				requestID, request.Model, result.Model, id, normalizeMeteringClient(c.GetHeader("User-Agent")),
				"systemone", &cif.CIFUsage{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens},
				time.Since(start).Milliseconds(), false, http.StatusOK, "",
			)
			c.JSON(http.StatusOK, result)
			return
		}
	}
	if c.Request.Context().Err() != nil {
		return
	}
	if lastErr == nil {
		writeSystemOneError(c, http.StatusNotFound, "no eligible TypeSafe model or provider available")
		return
	}
	status := http.StatusBadGateway
	var upstream *systemone.UpstreamError
	if errors.As(lastErr, &upstream) {
		if upstream.Status >= 400 && upstream.Status <= 599 {
			status = upstream.Status
		}
		if upstream.RetryAfter != "" {
			c.Header("Retry-After", upstream.RetryAfter)
		}
	}
	writeSystemOneError(c, status, lastErr.Error())
}
