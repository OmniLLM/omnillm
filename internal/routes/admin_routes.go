package routes

import (
	"github.com/gin-gonic/gin"
)

func SetupAdminRoutes(router *gin.RouterGroup, port int) {
	// Provider management
	router.GET("/providers", handleGetProviders)
	router.POST("/providers/login", handleProviderLogin)
	router.GET("/providers/login/:flow_id", handleProviderLoginStatus)
	router.DELETE("/providers/login/:flow_id", handleProviderLoginCancel)
	router.POST("/providers/switch", handleSwitchProvider)
	router.GET("/providers/priorities", handleGetProviderPriorities)
	router.POST("/providers/priorities", handleSetProviderPriorities)

	// Instance-specific routes resolve an ID, alias, or display name to the
	// canonical instance ID before invoking the existing handlers.
	providerByReference := router.Group("/providers/:id", resolveProviderReferenceParam)
	providerByReference.DELETE("", handleDeleteProvider)
	providerByReference.GET("/models", handleListProviderModels)
	providerByReference.POST("/models/refresh", handleRefreshProviderModels)
	providerByReference.POST("/models/toggle", handleToggleProviderModel)
	providerByReference.GET("/models/:modelId/version", handleGetModelVersion)
	providerByReference.PUT("/models/:modelId/version", handleSetModelVersion)
	providerByReference.GET("/usage", handleGetProviderUsage)
	providerByReference.POST("/auth", handleProviderAuth)
	providerByReference.POST("/auth/initiate-device-code", handleInitiateDeviceCode)
	providerByReference.POST("/auth/complete-device-code", handleCompleteDeviceCode)
	providerByReference.PUT("/config", handleUpdateProviderConfig)
	providerByReference.PATCH("/name", handleRenameProvider)
	providerByReference.POST("/activate", handleActivateProvider)
	providerByReference.POST("/deactivate", handleDeactivateProvider)

	// Provider type-specific routes (use specific path to avoid conflicts with wildcard :id routes)
	router.POST("/providers/auth-and-create/:type", handleAuthAndCreateProvider)

	// Antigravity Google OAuth2 authorization-code flow
	// Note: oauth-callback and oauth-status are registered on the public group in server.go
	router.POST("/providers/antigravity/start-oauth", handleAntigravityStartOAuth)
	router.POST("/providers/openai/start-oauth", handleOpenAIStartOAuth)

	// System info and status
	router.GET("/status", handleGetStatus)
	router.GET("/auth-status", handleGetAuthStatus)
	router.POST("/auth/cancel", handleCancelAuth)

	// Settings
	router.GET("/settings/log-level", handleGetLogLevel)
	router.PUT("/settings/log-level", handleSetLogLevel)
	router.GET("/settings/response-cache", handleGetResponseCache)
	router.PUT("/settings/response-cache", handleSetResponseCache)
	router.DELETE("/settings/response-cache", handleClearResponseCache)
	router.POST("/settings/test-log", handleTestLog)
	router.POST("/settings/debug-log", handleDebugLog)

	// Chat sessions
	router.GET("/chat/sessions", handleGetChatSessions)
	router.POST("/chat/sessions", handleCreateChatSession)
	router.DELETE("/chat/sessions", handleDeleteAllChatSessions)
	router.GET("/chat/sessions/:id", handleGetChatSession)
	router.PUT("/chat/sessions/:id", handleUpdateChatSession)
	router.POST("/chat/sessions/:id/messages", handleAddChatMessage)
	router.DELETE("/chat/sessions/:id", handleDeleteChatSession)

	// Logs streaming
	router.GET("/logs/stream", handleLogsStream)

	// Metering / usage data
	SetupMeteringRoutes(router)

	// Access token management
	SetupAccessTokenRoutes(router)

	// Config file management
	router.GET("/config", handleGetConfigFiles)
	router.GET("/config/:name", handleGetConfig)
	router.PUT("/config/:name", handleSaveConfig)
	router.POST("/config/:name/import", handleImportConfig)
	router.POST("/config/:name/backup", handleBackupConfig)
}
