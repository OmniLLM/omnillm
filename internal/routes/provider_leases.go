package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

// borrowModelProviders holds each active provider stable until a request finishes.
func borrowModelProviders(c *gin.Context) ([]types.Provider, func(), bool) {
	providers, release, err := registry.GetProviderRegistry().BorrowActiveProviders()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{"message": "Provider identity is being updated; retry the request", "type": "provider_error"},
		})
		return nil, nil, false
	}
	return providers, release, true
}
