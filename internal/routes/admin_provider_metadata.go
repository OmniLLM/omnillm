package routes

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
	"omnillm/internal/lib/providermetadata"
	"omnillm/internal/registry"
)

// handleProviderMetadata reads only explicit public fields; it never starts OAuth.
func handleProviderMetadata(c *gin.Context) {
	id := c.Param("id")
	provider, err := registry.GetProviderRegistry().GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider_not_found"})
		return
	}
	providerType := provider.GetID()
	result := providermetadata.OAuthCapabilities(providerType)
	result.ID = id

	config, err := loadProviderConfig(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider metadata unavailable"})
		return
	}
	if value, ok := firstStringValue(config, "endpoint", "base_url", "baseUrl"); ok {
		result.APIEndpointOrigin = value
	}
	if value, ok := firstStringValue(config, "region"); ok {
		result.Region = value
	}
	if value, ok := firstStringValue(config, "plan"); ok {
		result.Plan = value
	}
	if value, ok := firstStringValue(config, "api_format", "apiFormat"); ok {
		result.APIFormat = value
	}
	if value, ok := firstStringValue(config, "api_version", "apiVersion"); ok {
		result.APIVersion = value
	}

	if providerType == "antigravity" {
		record, err := database.NewTokenStore().Get(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider metadata unavailable"})
			return
		}
		if record != nil {
			var publicFields struct {
				ClientID string `json:"client_id"`
			}
			if err := json.Unmarshal([]byte(record.TokenData), &publicFields); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "provider metadata unavailable"})
				return
			}
			result.OAuthClientID = publicFields.ClientID
		}
	}
	c.JSON(http.StatusOK, providermetadata.Sanitize(providerType, result))
}
