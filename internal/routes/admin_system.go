package routes

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"omnillm/internal/registry"
)

func GetVersion() string {
	// Try to read from VERSION file first (source of truth)
	data, err := os.ReadFile("VERSION")
	if err == nil {
		version := strings.TrimSpace(string(data))
		if version != "" {
			return strings.TrimPrefix(version, "v")
		}
	}
	return "0.0.1" // fallback (without v prefix; frontend adds it)
}

func MakePublicInfoHandler(port int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version":      GetVersion(),
			"port":         port,
			"backend":      "golang",
			"uptime":       time.Since(serverStartTime).String(),
			"authRequired": true,
		})
	}
}

func handleGetStatus(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()
	activeProviders := providerRegistry.GetActiveProviders()
	var activeProvider map[string]interface{}
	modelCount := 0

	if len(activeProviders) > 0 {
		activeProvider = gin.H{
			"id":   activeProviders[0].GetInstanceID(),
			"name": activeProviders[0].GetName(),
		}
		for _, provider := range activeProviders {
			if models, err := loadProviderModels(provider, false); err == nil {
				modelCount += countEnabledModels(models)
			}
		}
	}

	// Build auth flow state for response
	activeAuthFlowMu.RLock()
	flow := activeAuthFlow
	activeAuthFlowMu.RUnlock()

	var authFlowResp interface{}
	if flow != nil {
		flowMap := gin.H{
			"providerId": flow.ProviderID,
			"status":     flow.Status,
		}
		if flow.InstructionURL != "" {
			flowMap["instructionURL"] = flow.InstructionURL
		}
		if flow.UserCode != "" {
			flowMap["userCode"] = flow.UserCode
		}
		if flow.Error != "" {
			flowMap["error"] = flow.Error
		}
		authFlowResp = flowMap
	}

	manualApproval, rateLimiter := getAdminStatusSnapshot()

	c.JSON(http.StatusOK, gin.H{
		"activeProvider":   activeProvider,
		"modelCount":       modelCount,
		"manualApprove":    manualApproval,
		"rateLimitSeconds": rateLimiter.GetIntervalSeconds(),
		"rateLimitWait":    rateLimiter.GetWaitOnLimit(),
		"authFlow":         authFlowResp,
		"status":           "healthy",
		"services": gin.H{
			"api": "running",
			"providers": gin.H{
				"total":  len(providerRegistry.ListProviders()),
				"active": len(activeProviders),
			},
			"database": "connected",
		},
		"uptime":    time.Since(serverStartTime).String(),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func handleGetAuthStatus(c *gin.Context) {
	activeAuthFlowMu.RLock()
	flow := activeAuthFlow
	activeAuthFlowMu.RUnlock()

	if flow == nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle"})
		return
	}

	resp := gin.H{
		"providerId": flow.ProviderID,
		"status":     flow.Status,
	}
	if flow.InstructionURL != "" {
		resp["instructionURL"] = flow.InstructionURL
	}
	if flow.UserCode != "" {
		resp["userCode"] = flow.UserCode
	}
	if flow.Error != "" {
		resp["error"] = flow.Error
	}

	// Clear completed/errored flows after reporting them
	if flow.Status == "complete" || flow.Status == "error" {
		activeAuthFlowMu.Lock()
		if activeAuthFlow == flow {
			activeAuthFlow = nil
		}
		activeAuthFlowMu.Unlock()
	}

	c.JSON(http.StatusOK, resp)
}

func handleCancelAuth(c *gin.Context) {
	activeAuthFlowMu.Lock()
	flow := activeAuthFlow
	// Only cancel flows that are still in-progress; if the flow already
	// completed/errored, clearing it here would race with the frontend's
	// final poll that reads the "complete" status.
	if flow != nil && (flow.Status == "pending" || flow.Status == "awaiting_user") {
		if flow.cancelFn != nil {
			flow.cancelFn()
		}
		activeAuthFlow = nil
	} else {
		flow = nil // signal to caller: nothing was cancelled
	}
	activeAuthFlowMu.Unlock()

	if flow == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "No active auth flow"})
		return
	}
	log.Info().Str("provider", flow.ProviderID).Msg("OAuth flow cancelled by user")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Auth flow cancelled"})
}

func handleGetLogLevel(c *gin.Context) {
	level := zerolog.Level(currentLogLevel.Load())
	c.JSON(http.StatusOK, gin.H{
		"level":  level.String(),
		"levels": []string{"fatal", "error", "warn", "info", "debug", "trace"},
	})
}

func handleSetLogLevel(c *gin.Context) {
	var req struct {
		Level string `json:"level"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	level, err := zerolog.ParseLevel(req.Level)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid log level: %s", req.Level)})
		return
	}

	currentLogLevel.Store(int32(level))
	zerolog.SetGlobalLevel(level)

	log.Info().Str("level", req.Level).Msg("Log level changed")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"level":   req.Level,
		"message": "Log level updated to " + req.Level,
	})
}

func handleTestLog(c *gin.Context) {
	log.Trace().Msg("Test trace message")
	log.Debug().Msg("Test debug message")
	log.Info().Msg("Test info message")
	log.Warn().Msg("Test warn message")
	log.Error().Msg("Test error message")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Test log messages sent at all levels",
	})
}

func handleDebugLog(c *gin.Context) {
	var body interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Debug().Err(err).Msg("Debug log entry with invalid payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}
	log.Debug().Interface("payload", body).Msg("Debug log entry")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
