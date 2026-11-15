package routes

import (
	"errors"
	"net/http"
	"omnillm/internal/registry"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupUsageRoutes(router *gin.RouterGroup) {
	router.GET("/usage", handleUsage)
}

func handleUsage(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()
	activeProvider, releaseProvider, err := providerRegistry.BorrowActiveProvider()
	if err != nil {
		if errors.Is(err, registry.ErrProviderBusy) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "Provider identity is being updated; retry the request", "type": "provider_error"}})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "No active provider",
				"type":    "invalid_request_error",
			},
		})
		return
	}
	defer releaseProvider()

	usage, err := activeProvider.GetUsage()
	if err != nil {
		log.Error().Err(err).
			Str("provider", activeProvider.GetInstanceID()).
			Msg("Failed to get usage")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to get usage data",
				"type":    "api_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}
