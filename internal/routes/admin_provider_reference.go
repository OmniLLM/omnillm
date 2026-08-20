package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
)

func resolveProviderReference(reference string) (string, error) {
	return database.GetModelResolutionCache().ResolveProviderReference(reference)
}

func writeProviderReferenceError(c *gin.Context, reference string, err error) {
	var ambiguous *database.AmbiguousProviderReferenceError
	if errors.As(err, &ambiguous) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "ambiguous_provider_reference",
			"message":      ambiguous.Error(),
			"reference":    reference,
			"matching_ids": ambiguous.Matches,
		})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error":     "provider_not_found",
		"message":   err.Error(),
		"reference": reference,
	})
}

func resolveProviderReferenceParam(c *gin.Context) {
	reference := strings.TrimSpace(c.Param("id"))
	instanceID, err := resolveProviderReference(reference)
	if err != nil {
		writeProviderReferenceError(c, reference, err)
		c.Abort()
		return
	}
	for index := range c.Params {
		if c.Params[index].Key == "id" {
			c.Params[index].Value = instanceID
			break
		}
	}
	c.Next()
}
