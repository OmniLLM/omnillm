package routes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"omnillm/internal/database"
)

// SetupAccessTokenRoutes registers the access-token management endpoints.
func SetupAccessTokenRoutes(router *gin.RouterGroup) {
	router.GET("/access-tokens", handleListAccessTokens)
	router.POST("/access-tokens", handleCreateAccessToken)
	router.DELETE("/access-tokens/:id", handleDeleteAccessToken)
}

func handleListAccessTokens(c *gin.Context) {
	store := database.NewAccessTokenStore()
	tokens, err := store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tokens == nil {
		tokens = []database.AccessTokenRecord{}
	}
	c.JSON(http.StatusOK, tokens)
}

type createAccessTokenRequest struct {
	Name      string `json:"name" binding:"required"`
	ExpiresAt string `json:"expires_at,omitempty"` // RFC3339 or empty
}

func handleCreateAccessToken(c *gin.Context) {
	var req createAccessTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Generate a random token: omnillm_ + 32 hex bytes
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	rawToken := "omnillm_" + hex.EncodeToString(rawBytes)

	// Hash for storage
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	prefix := rawToken[:16] // "omnillm_" + first 8 hex chars

	id := uuid.New().String()

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_at format, use RFC3339"})
			return
		}
		expiresAt = &t
	}

	store := database.NewAccessTokenStore()
	if err := store.Create(id, req.Name, tokenHash, prefix, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"token":      rawToken, // shown only once
		"prefix":     prefix,
		"expires_at": expiresAt,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleDeleteAccessToken(c *gin.Context) {
	id := c.Param("id")
	store := database.NewAccessTokenStore()
	if err := store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
