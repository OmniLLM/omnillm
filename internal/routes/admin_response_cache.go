package routes

import (
	"net/http"
	"strconv"

	"omnillm/internal/database"
	"omnillm/internal/lib/responsecache"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// handleGetResponseCache reports current response-cache settings and stats.
func handleGetResponseCache(c *gin.Context) {
	cfg := responsecache.LoadConfig()
	store := responsecache.CurrentStore()
	stats, err := store.Stats(c.Request.Context())
	available := store.Available()
	if err != nil {
		stats = responsecache.Stats{}
		available = false
		log.Warn().Err(err).Msg("Failed to read response cache stats")
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":         cfg.Enabled,
		"ttl_seconds":     int(cfg.TTL.Seconds()),
		"entries":         stats.Entries,
		"total_hits":      stats.TotalHits,
		"payload_bytes":   stats.PayloadBytes,
		"lookup_hits":     stats.LookupHits,
		"lookup_misses":   stats.LookupMisses,
		"lookup_hit_rate": stats.LookupHitRate,
		"stats_since":     stats.StatsSince,
		"backend":         "redis",
		"available":       available,
	})
}

// handleSetResponseCache updates response-cache settings (enable flag + TTL).
func handleSetResponseCache(c *gin.Context) {
	var req struct {
		Enabled    *bool `json:"enabled"`
		TTLSeconds *int  `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	store := database.NewConfigStore()
	if req.Enabled != nil {
		val := "false"
		if *req.Enabled {
			val = "true"
		}
		if err := store.Set("response_cache.enabled", val); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.TTLSeconds != nil {
		if *req.TTLSeconds < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ttl_seconds must be >= 0"})
			return
		}
		if err := store.Set("response_cache.ttl_seconds", strconv.Itoa(*req.TTLSeconds)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	cfg := responsecache.LoadConfig()
	log.Info().Bool("enabled", cfg.Enabled).Int("ttl_seconds", int(cfg.TTL.Seconds())).Msg("Response cache settings updated")
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"enabled":     cfg.Enabled,
		"ttl_seconds": int(cfg.TTL.Seconds()),
	})
}

// handleClearResponseCache purges all cached responses.
func handleClearResponseCache(c *gin.Context) {
	removed, err := responsecache.CurrentStore().Clear(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Info().Int64("removed", removed).Msg("Response cache cleared")
	c.JSON(http.StatusOK, gin.H{"success": true, "removed": removed})
}
