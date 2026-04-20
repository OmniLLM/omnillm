package routes

import (
	"net/http"
	"omnillm/internal/registry"

	"github.com/gin-gonic/gin"
)

func SetupTokenRoutes(router *gin.RouterGroup) {
	router.GET("/token", handleToken)
}

func handleToken(c *gin.Context) {
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

	token := activeProvider.GetToken()
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "Provider not authenticated",
				"type":    "authentication_error",
			},
		})
		return
	}

	responseToken := maskToken(token)
	if getSecurityOptions().ShowToken {
		responseToken = token
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": responseToken,
		"token_type":   "Bearer",
		"masked":       !getSecurityOptions().ShowToken,
	})
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "********" + token[len(token)-4:]
}
