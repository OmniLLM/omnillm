package routes

import (
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
	activeProvider, err := providerRegistry.GetActive()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "No active provider",
				"type":    "invalid_request_error",
			},
		})
		return
	}

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
