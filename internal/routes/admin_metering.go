package routes

import (
	"net/http"
	"omnillm/internal/database"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func SetupMeteringRoutes(router *gin.RouterGroup) {
	router.GET("/metering/logs", handleMeteringLogs)
	router.GET("/metering/stats", handleMeteringStats)
	router.GET("/metering/by-model", handleMeteringByModel)
	router.GET("/metering/by-provider", handleMeteringByProvider)
	router.GET("/metering/by-client", handleMeteringByClient)
	router.GET("/metering/models", handleMeteringModels)
	router.GET("/metering/providers", handleMeteringProviders)
	router.GET("/metering/clients", handleMeteringClients)
}

// parseMeteringFilter reads common query params shared by all metering endpoints.
func parseMeteringFilter(c *gin.Context) database.MeteringFilter {
	f := database.MeteringFilter{
		ModelID:    c.Query("model_id"),
		ProviderID: c.Query("provider_id"),
		Client:     c.Query("client"),
		APIShape:   c.Query("api_shape"),
	}
	if s := c.Query("since"); s != "" {
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = ts
		}
	}
	if s := c.Query("until"); s != "" {
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			f.Until = ts
		}
	}
	return f
}

// GET /api/admin/metering/logs?model_id=&provider_id=&api_shape=&since=&until=&page=1&page_size=50
func handleMeteringLogs(c *gin.Context) {
	f := parseMeteringFilter(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	db := database.GetDatabase()
	records, total, err := db.ListMeteringRecords(f, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GET /api/admin/metering/stats?model_id=&provider_id=&api_shape=&since=&until=
func handleMeteringStats(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	stats, err := db.GetMeteringStats(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GET /api/admin/metering/by-model?since=&until=
func handleMeteringByModel(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	breakdown, err := db.GetMeteringByModel(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": breakdown})
}

// GET /api/admin/metering/by-provider?since=&until=
func handleMeteringByProvider(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	breakdown, err := db.GetMeteringByProvider(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": breakdown})
}

// GET /api/admin/metering/by-client?since=&until=
func handleMeteringByClient(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	breakdown, err := db.GetMeteringByClient(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": breakdown})
}

// GET /api/admin/metering/models?since=&until=
func handleMeteringModels(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	models, err := db.GetDistinctMeteringModels(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": models})
}

// GET /api/admin/metering/providers?since=&until=
func handleMeteringProviders(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	providers, err := db.GetDistinctMeteringProviders(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": providers})
}

// GET /api/admin/metering/clients?since=&until=
func handleMeteringClients(c *gin.Context) {
	f := parseMeteringFilter(c)

	db := database.GetDatabase()
	clients, err := db.GetDistinctMeteringClients(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": clients})
}
