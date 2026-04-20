package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/registry"
)

func SetupEmbeddingRoutes(router *gin.RouterGroup) {
	router.POST("/embeddings", handleEmbeddings)
}

func handleEmbeddings(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Try active providers in priority order
	providerRegistry := registry.GetProviderRegistry()
	activeProviders := providerRegistry.GetActiveProviders()

	if len(activeProviders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "No active providers available",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	var lastErr error
	for _, provider := range activeProviders {
		result, err := provider.CreateEmbeddings(payload)
		if err != nil {
			lastErr = err
			log.Warn().Err(err).
				Str("provider", provider.GetInstanceID()).
				Msg("Embeddings failed for provider")
			continue
		}

		c.JSON(http.StatusOK, result)
		return
	}

	errMsg := "No providers could handle the embeddings request"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"message": errMsg,
			"type":    "provider_error",
		},
	})
}
