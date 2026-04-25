package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const authorizationHeader = "Authorization"

type authConfig struct {
	apiKey string
}

func newAuthConfig(apiKey string) authConfig {
	return authConfig{apiKey: strings.TrimSpace(apiKey)}
}

func (a authConfig) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.apiKey == "" {
			c.Next()
			return
		}

		token, ok := extractBearerToken(c.GetHeader(authorizationHeader))
		if !ok {
			token = strings.TrimSpace(c.GetHeader("x-api-key"))
			ok = token != ""
		}
		// Query parameter auth is intentionally not accepted to prevent credential leakage in server logs.
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(a.apiKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Next()
	}
}

func extractBearerToken(header string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
