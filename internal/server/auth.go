package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
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
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		// Check master API key first (constant-time).
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.apiKey)) == 1 {
			c.Next()
			return
		}

		// Fall back to access tokens: hash the presented token and look it up.
		hash := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(hash[:])
		store := database.NewAccessTokenStore()
		rec, err := store.ValidateByHash(tokenHash)
		if err == nil && rec != nil {
			c.Set("access_token_id", rec.ID)
			c.Set("access_token_name", rec.Name)
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
