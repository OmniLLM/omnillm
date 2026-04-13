package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"omnimodel/internal/database"
	alibabapkg "omnimodel/internal/providers/alibaba"
	"omnimodel/internal/providers/copilot"
	"omnimodel/internal/providers/generic"
	"omnimodel/internal/providers/types"
	"omnimodel/internal/registry"
	ghservice "omnimodel/internal/services/github"
)

// Log subscriber for SSE streaming
type logSubscriber struct {
	ch   chan string
	done chan struct{}
}

var (
	logSubscribersMu sync.RWMutex
	logSubscribers   = make(map[*logSubscriber]struct{})
	currentLogLevel  = zerolog.InfoLevel
	serverStartTime  = time.Now()

	// Active OAuth device code flow state
	activeAuthFlowMu sync.RWMutex
	activeAuthFlow   *authFlowState
)

type authFlowState struct {
	ProviderID     string             `json:"providerId"`
	Status         string             `json:"status"` // pending, awaiting_user, complete, error
	InstructionURL string             `json:"instructionURL,omitempty"`
	UserCode       string             `json:"userCode,omitempty"`
	Error          string             `json:"error,omitempty"`
	deviceCode     string             // internal, not exposed
	codeVerifier   string             // internal PKCE verifier for Alibaba OAuth
	cancelFn       context.CancelFunc // cancels the background polling goroutine
}

// BroadcastLog sends a log message to all SSE subscribers
func BroadcastLog(level, message string) {
	timestamp := time.Now().Format(time.RFC3339)
	BroadcastLogLine(fmt.Sprintf("[%s] | backend | %s | %s", timestamp, strings.ToUpper(level), message))
}

// BroadcastLogLine sends a preformatted log line to all SSE subscribers.
func BroadcastLogLine(line string) {
	logSubscribersMu.RLock()
	defer logSubscribersMu.RUnlock()

	data := formatSSEData(line)
	for sub := range logSubscribers {
		select {
		case sub.ch <- data:
		default:
			// subscriber too slow, skip
		}
	}
}

func formatSSEData(message string) string {
	var builder strings.Builder
	for _, line := range strings.Split(strings.TrimRight(message, "\n"), "\n") {
		builder.WriteString("data: ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	return builder.String()
}

func SetupAdminRoutes(router *gin.RouterGroup, port int) {
	// Provider management
	router.GET("/providers", handleGetProviders)
	router.POST("/providers/switch", handleSwitchProvider)
	router.GET("/providers/priorities", handleGetProviderPriorities)
	router.POST("/providers/priorities", handleSetProviderPriorities)

	// Instance-specific routes (all :id routes before :type routes)
	router.DELETE("/providers/:id", handleDeleteProvider)
	router.GET("/providers/:id/models", handleListProviderModels)
	router.POST("/providers/:id/models/toggle", handleToggleProviderModel)
	router.GET("/providers/:id/models/:modelId/version", handleGetModelVersion)
	router.PUT("/providers/:id/models/:modelId/version", handleSetModelVersion)
	router.GET("/providers/:id/usage", handleGetProviderUsage)
	router.POST("/providers/:id/auth", handleProviderAuth)
	router.POST("/providers/:id/auth/initiate-device-code", handleInitiateDeviceCode)
	router.POST("/providers/:id/auth/complete-device-code", handleCompleteDeviceCode)
	router.PUT("/providers/:id/config", handleUpdateProviderConfig)
	router.POST("/providers/:id/activate", handleActivateProvider)
	router.POST("/providers/:id/deactivate", handleDeactivateProvider)

	// Provider type-specific routes (use specific path to avoid conflicts with wildcard :id routes)
	router.POST("/providers/add/:type", handleAddProviderInstance)
	router.POST("/providers/auth-and-create/:type", handleAuthAndCreateProvider)

	// System info and status
	router.GET("/info", makeGetInfoHandler(port))
	router.GET("/status", handleGetStatus)
	router.GET("/auth-status", handleGetAuthStatus)
	router.POST("/auth/cancel", handleCancelAuth)

	// Settings
	router.GET("/settings/log-level", handleGetLogLevel)
	router.PUT("/settings/log-level", handleSetLogLevel)
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
}

type providerModelView struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	MaxTokens    int                    `json:"max_tokens,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

func loadProviderConfig(instanceID string) (map[string]interface{}, error) {
	configStore := database.NewProviderConfigStore()
	record, err := configStore.Get(instanceID)
	if err != nil || record == nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
		return nil, err
	}

	return config, nil
}

func normalizeProviderConfigForFrontend(providerType string, config map[string]interface{}) map[string]interface{} {
	if len(config) == 0 {
		return nil
	}

	switch providerType {
	case "azure-openai":
		normalized := map[string]interface{}{}
		if endpoint, ok := firstStringValue(config, "endpoint"); ok {
			normalized["endpoint"] = endpoint
		}
		if apiVersion, ok := firstStringValue(config, "apiVersion", "api_version"); ok {
			normalized["apiVersion"] = apiVersion
		}
		if deployments := stringSliceValue(config["deployments"]); len(deployments) > 0 {
			normalized["deployments"] = deployments
		}
		if len(normalized) == 0 {
			return nil
		}
		return normalized
	case "alibaba":
		normalized := map[string]interface{}{}
		if baseURL, ok := firstStringValue(config, "baseUrl", "base_url"); ok {
			normalized["baseUrl"] = baseURL
		}
		if region, ok := firstStringValue(config, "region"); ok {
			normalized["region"] = region
		}
		if plan, ok := firstStringValue(config, "plan"); ok {
			normalized["plan"] = plan
		}
		if len(normalized) == 0 {
			return nil
		}
		return normalized
	default:
		return config
	}
}

func normalizeProviderConfigForStorage(providerType string, config map[string]interface{}) map[string]interface{} {
	switch providerType {
	case "azure-openai":
		endpoint, _ := firstStringValue(config, "endpoint")
		apiVersion, _ := firstStringValue(config, "apiVersion", "api_version")
		deployments := stringSliceValue(config["deployments"])

		normalized := map[string]interface{}{}
		if endpoint != "" {
			normalized["endpoint"] = endpoint
		}
		if apiVersion != "" {
			normalized["api_version"] = apiVersion
		}
		if len(deployments) > 0 {
			normalized["deployments"] = deployments
		}
		return normalized
	case "alibaba":
		baseURL, _ := firstStringValue(config, "baseUrl", "base_url")
		region, _ := firstStringValue(config, "region")
		plan, _ := firstStringValue(config, "plan")

		normalized := map[string]interface{}{}
		if baseURL != "" {
			normalized["base_url"] = baseURL
		}
		if region != "" {
			normalized["region"] = region
		}
		if plan != "" {
			normalized["plan"] = plan
		}
		return normalized
	default:
		return config
	}
}

func loadProviderModels(provider types.Provider) ([]providerModelView, error) {
	stateStore := database.NewModelStateStore()
	states, err := stateStore.GetAllByInstance(provider.GetInstanceID())
	if err != nil {
		return nil, err
	}

	stateByID := make(map[string]database.ProviderModelStateRecord, len(states))
	for _, state := range states {
		stateByID[state.ModelID] = state
	}

	modelsResp, err := provider.GetModels()
	if err != nil {
		if len(states) == 0 {
			return nil, err
		}

		models := make([]providerModelView, 0, len(states))
		for _, state := range states {
			models = append(models, providerModelView{
				ID:      state.ModelID,
				Name:    state.ModelID,
				Enabled: state.Enabled,
			})
		}
		return models, nil
	}

	models := make([]providerModelView, 0, len(modelsResp.Data)+len(states))
	seen := make(map[string]struct{}, len(modelsResp.Data))

	for _, model := range modelsResp.Data {
		if _, exists := seen[model.ID]; exists {
			continue
		}
		enabled := true
		if state, ok := stateByID[model.ID]; ok {
			enabled = state.Enabled
		}

		models = append(models, providerModelView{
			ID:           model.ID,
			Name:         model.Name,
			Description:  model.Description,
			MaxTokens:    model.MaxTokens,
			Enabled:      enabled,
			Capabilities: model.Capabilities,
		})
		seen[model.ID] = struct{}{}
	}

	for _, state := range states {
		if _, exists := seen[state.ModelID]; exists {
			continue
		}

		models = append(models, providerModelView{
			ID:      state.ModelID,
			Name:    state.ModelID,
			Enabled: state.Enabled,
		})
	}

	return models, nil
}

func countEnabledModels(models []providerModelView) int {
	enabled := 0
	for _, model := range models {
		if model.Enabled {
			enabled++
		}
	}

	return enabled
}

func firstStringValue(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value, true
		}
	}

	return "", false
}

func stringSliceValue(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// ─── Provider endpoints ───

func handleGetProviders(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()
	providers := providerRegistry.ListProviders()

	providerList := make([]map[string]interface{}, 0, len(providers))
	for _, provider := range providers {
		config, configErr := loadProviderConfig(provider.GetInstanceID())
		if configErr != nil {
			log.Warn().Err(configErr).Str("provider", provider.GetInstanceID()).Msg("Failed to load provider config")
		}

		authStatus := "unauthenticated"
		if provider.GetToken() != "" {
			authStatus = "authenticated"
		}

		providerInfo := map[string]interface{}{
			"id":         provider.GetInstanceID(),
			"type":       provider.GetID(),
			"name":       provider.GetName(),
			"isActive":   providerRegistry.IsActiveProvider(provider.GetInstanceID()),
			"authStatus": authStatus,
		}

		if normalizedConfig := normalizeProviderConfigForFrontend(provider.GetID(), config); normalizedConfig != nil {
			providerInfo["config"] = normalizedConfig
		}

		if authStatus == "authenticated" {
			models, err := loadProviderModels(provider)
			if err != nil {
				log.Warn().Err(err).Str("provider", provider.GetInstanceID()).Msg("Failed to load provider models")
				providerInfo["totalModelCount"] = 0
				providerInfo["enabledModelCount"] = 0
			} else {
				providerInfo["totalModelCount"] = len(models)
				providerInfo["enabledModelCount"] = countEnabledModels(models)
			}
		} else {
			providerInfo["totalModelCount"] = 0
			providerInfo["enabledModelCount"] = 0
		}

		providerList = append(providerList, providerInfo)
	}

	c.JSON(http.StatusOK, providerList)
}

func handleSwitchProvider(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerID, _ := firstStringValue(req, "providerId", "provider_id")
	if providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "providerId is required"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.SetActive(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": fmt.Sprintf("Provider '%s' not found", providerID),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  fmt.Sprintf("Switched to provider %s", provider.GetInstanceID()),
		"provider": provider.GetInstanceID(),
	})
}

func handleAddProviderInstance(c *gin.Context) {
	providerType := c.Param("type")

	providerRegistry := registry.GetProviderRegistry()
	instanceID := providerRegistry.NextInstanceID(providerType)

	var provider types.Provider
	switch providerType {
	case "github-copilot":
		provider = copilot.NewGitHubCopilotProvider(instanceID)
	case "antigravity", "alibaba", "azure-openai", "google", "kimi":
		provider = generic.NewGenericProvider(providerType, instanceID, "")
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Unknown provider type '%s'", providerType),
		})
		return
	}

	if err := providerRegistry.Register(provider, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to register provider: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"provider": gin.H{
			"id":         provider.GetInstanceID(),
			"type":       provider.GetID(),
			"name":       provider.GetName(),
			"isActive":   false,
			"authStatus": "unauthenticated",
		},
	})
}

// handleAuthAndCreateProvider authenticates a new provider instance *before*
// persisting it to the database.  The provider is only saved when auth succeeds,
// eliminating the temporary placeholder record and the post-auth rename.
//
// POST /api/admin/providers/auth-and-create/:type
// Body: same as handleProviderAuth (method, apiKey, region, token, …)
func handleAuthAndCreateProvider(c *gin.Context) {
	providerType := c.Param("type")

	var req types.AuthOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()

	// Cancel any existing active auth flow before starting a new one to prevent
	// concurrent goroutines from corrupting the singleton activeAuthFlow state.
	activeAuthFlowMu.Lock()
	if activeAuthFlow != nil {
		if activeAuthFlow.cancelFn != nil {
			activeAuthFlow.cancelFn()
		}
		log.Info().Str("provider", activeAuthFlow.ProviderID).Msg("Auth-and-create: cancelled previous auth flow")
		activeAuthFlow = nil
	}
	activeAuthFlowMu.Unlock()

	switch providerType {

	// ── Alibaba ──────────────────────────────────────────────────────────────
	case "alibaba":
		if req.Method == "oauth" {
			// Build an ephemeral provider object — not yet in the registry.
			gen := generic.NewGenericProvider("alibaba", "alibaba-tmp", "")

			// Initiate device flow without creating a DB record first.
			flow, err := alibabapkg.InitiateDeviceFlow(context.Background())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": fmt.Sprintf("Failed to initiate Alibaba OAuth: %v", err),
				})
				return
			}

			verifyURL := flow.VerificationURIComplete
			if verifyURL == "" {
				verifyURL = flow.VerificationURI
			}
			if verifyURL != "" && !strings.Contains(verifyURL, "prompt=") {
				sep := "&"
				if !strings.Contains(verifyURL, "?") {
					sep = "?"
				}
				verifyURL += sep + "prompt=login"
			}

			// Use a sentinel provider ID for the auth flow; the real ID is
			// assigned by SaveAlibabaOAuthToken once the user authorizes.
			const pendingID = "alibaba-pending"

			alibabaCtx, alibabaCancel := context.WithCancel(context.Background())
			activeAuthFlowMu.Lock()
			activeAuthFlow = &authFlowState{
				ProviderID:     pendingID,
				Status:         "awaiting_user",
				InstructionURL: verifyURL,
				UserCode:       flow.UserCode,
				deviceCode:     flow.DeviceCode,
				codeVerifier:   flow.CodeVerifier,
				cancelFn:       alibabaCancel,
			}
			activeAuthFlowMu.Unlock()

			alibabaFlow := flow
			go func() {
				defer alibabaCancel()
				td, err := alibabapkg.PollForToken(
					alibabaCtx,
					alibabaFlow.DeviceCode,
					alibabaFlow.CodeVerifier,
					alibabaFlow.Interval,
					alibabaFlow.ExpiresIn,
				)
				if err != nil {
					if alibabaCtx.Err() != nil {
						return
					}
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = err.Error()
					}
					activeAuthFlowMu.Unlock()
					log.Error().Err(err).Str("type", "alibaba").Msg("Auth-and-create: Alibaba OAuth failed")
					return
				}

				// Derive canonical instance ID and persist token — first DB write.
				newInstanceID, err := gen.SaveAlibabaOAuthToken(td)
				if err != nil {
					log.Error().Err(err).Str("type", "alibaba").Msg("Auth-and-create: failed to save Alibaba OAuth token")
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = "Failed to save OAuth token"
					}
					activeAuthFlowMu.Unlock()
					return
				}

				// Register provider for the first time with its canonical ID.
				if err := providerRegistry.Register(gen, true); err != nil {
					log.Warn().Err(err).Str("provider", newInstanceID).Msg("Auth-and-create: failed to register Alibaba provider")
				}

				// Update status and provider ID atomically.
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
					activeAuthFlow.ProviderID = newInstanceID
					activeAuthFlow.Status = "complete"
				}
				activeAuthFlowMu.Unlock()

				log.Info().Str("provider", newInstanceID).Msg("Auth-and-create: Alibaba OAuth completed")
			}()

			c.JSON(http.StatusOK, gin.H{
				"success":          false,
				"requiresAuth":     true,
				"pending_id":       pendingID,
				"user_code":        flow.UserCode,
				"verification_uri": verifyURL,
				"message":          fmt.Sprintf("Visit %s and enter code: %s", verifyURL, flow.UserCode),
			})
			return
		}

		// API-key path — build provider with the canonical ID from the start to
		// avoid writing any ephemeral "alibaba-new" records to the database.
		suffix := req.APIKey
		if len(suffix) > 6 {
			suffix = suffix[len(suffix)-6:]
		}
		plan := strings.ToLower(strings.TrimSpace(req.Plan))
		switch plan {
		case "", "standard":
			plan = "standard"
		case "coding", "coding_plan", "coding-plan":
			plan = "coding-plan"
		default:
			plan = "standard"
		}
		region := req.Region
		if region == "" {
			region = "global"
		}
		planSlug := strings.ReplaceAll(plan, "-plan", "")
		canonicalID := "alibaba-" + planSlug + "-" + region + "-" + suffix
		gen := generic.NewGenericProvider("alibaba", canonicalID, "")

		if err := gen.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(gen, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         gen.GetInstanceID(),
				"type":       gen.GetID(),
				"name":       gen.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ── GitHub Copilot ───────────────────────────────────────────────────────
	case "github-copilot":
		cop := copilot.NewGitHubCopilotProvider("copilot-tmp")

		if req.Method == "oauth" || (req.GithubToken == "" && req.Token == "") {
			deviceCode, err := cop.InitiateDeviceCodeFlow()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
				})
				return
			}

			const pendingID = "copilot-pending"
			copilotCtx, copilotCancel := context.WithCancel(context.Background())
			activeAuthFlowMu.Lock()
			activeAuthFlow = &authFlowState{
				ProviderID:     pendingID,
				Status:         "awaiting_user",
				InstructionURL: deviceCode.VerificationURI,
				UserCode:       deviceCode.UserCode,
				deviceCode:     deviceCode.DeviceCode,
				cancelFn:       copilotCancel,
			}
			activeAuthFlowMu.Unlock()

			dc := deviceCode
			go func() {
				defer copilotCancel()
				if err := cop.PollAndCompleteDeviceCodeFlow(dc); err != nil {
					if copilotCtx.Err() != nil {
						return
					}
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = err.Error()
					}
					activeAuthFlowMu.Unlock()
					log.Error().Err(err).Str("type", "github-copilot").Msg("Auth-and-create: OAuth failed")
					return
				}

				// Assign a canonical instance ID using the GitHub username embedded in the provider name.
				canonicalID := providerRegistry.NextInstanceID("github-copilot")
				cop.SetInstanceID(canonicalID)

				if err := cop.SaveToDB(); err != nil {
					log.Error().Err(err).Str("provider", canonicalID).Msg("Auth-and-create: failed to save GitHub Copilot token")
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = "Failed to save token"
					}
					activeAuthFlowMu.Unlock()
					return
				}

				if err := providerRegistry.Register(cop, true); err != nil {
					log.Warn().Err(err).Str("provider", canonicalID).Msg("Auth-and-create: failed to register GitHub Copilot provider")
				}

				// Update status and provider ID atomically.
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
					activeAuthFlow.ProviderID = canonicalID
					activeAuthFlow.Status = "complete"
				}
				activeAuthFlowMu.Unlock()

				log.Info().Str("provider", canonicalID).Msg("Auth-and-create: GitHub Copilot OAuth completed")
			}()

			c.JSON(http.StatusOK, gin.H{
				"success":          false,
				"requiresAuth":     true,
				"pending_id":       pendingID,
				"user_code":        deviceCode.UserCode,
				"verification_uri": deviceCode.VerificationURI,
				"message":          fmt.Sprintf("Visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
			})
			return
		}

		// Direct token path.
		token := req.Token
		if token == "" {
			token = req.GithubToken
		}
		req.GithubToken = token
		if err := cop.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		canonicalID := providerRegistry.NextInstanceID("github-copilot")
		cop.SetInstanceID(canonicalID)

		if err := cop.SaveToDB(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to save provider credentials: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(cop, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         cop.GetInstanceID(),
				"type":       cop.GetID(),
				"name":       cop.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ── API-key based providers ────────────────────────────────────────────────
	case "azure-openai", "antigravity", "google", "kimi":
		instanceID := providerRegistry.NextInstanceID(providerType)
		gen := generic.NewGenericProvider(providerType, instanceID, "")

		if err := gen.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(gen, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         gen.GetInstanceID(),
				"type":       gen.GetID(),
				"name":       gen.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Unknown provider type '%s'", providerType),
		})
	}
}

func handleDeleteProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if err := providerRegistry.Remove(providerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	modelStateStore := database.NewModelStateStore()
	if states, err := modelStateStore.GetAllByInstance(providerID); err == nil {
		for _, state := range states {
			_ = modelStateStore.Delete(providerID, state.ModelID)
		}
	}
	modelConfigStore := database.NewModelConfigStore()
	if configs, err := modelConfigStore.GetAllByInstance(providerID); err == nil {
		for _, config := range configs {
			_ = modelConfigStore.Delete(providerID, config.ModelID)
		}
	}
	_ = database.NewProviderConfigStore().Delete(providerID)
	_ = database.NewProviderInstanceStore().Delete(providerID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Provider '%s' deleted", providerID),
	})
}

func handleGetProviderPriorities(c *gin.Context) {
	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get provider priorities from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve provider priorities"})
		return
	}

	priorities := make(map[string]int)
	for _, instance := range instances {
		priorities[instance.InstanceID] = instance.Priority
	}

	c.JSON(http.StatusOK, gin.H{"priorities": priorities})
}

func handleSetProviderPriorities(c *gin.Context) {
	var req struct {
		Priorities map[string]int `json:"priorities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	instanceStore := database.NewProviderInstanceStore()
	for id, priority := range req.Priorities {
		record, err := instanceStore.Get(id)
		if err != nil || record == nil {
			continue
		}
		record.Priority = priority
		instanceStore.Save(record)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider priorities updated",
	})
}

func handleListProviderModels(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	models, err := loadProviderModels(provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"total":  len(models),
	})
}

func handleToggleProviderModel(c *gin.Context) {
	providerID := c.Param("id")
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	modelID, _ := firstStringValue(req, "modelId", "model_id")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelId is required"})
		return
	}

	enabled, ok := req["enabled"].(bool)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
		return
	}

	modelStateStore := database.NewModelStateStore()
	if err := modelStateStore.SetEnabled(providerID, modelID, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist model state"})
		return
	}

	log.Info().
		Str("provider", providerID).
		Str("model", modelID).
		Bool("enabled", enabled).
		Msg("Model toggle requested")

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"modelId":  modelID,
		"model_id": modelID,
		"enabled":  enabled,
	})
}

func handleGetProviderUsage(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	usage, err := provider.GetUsage()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage data"})
		return
	}

	c.JSON(http.StatusOK, usage)
}

func handleProviderAuth(c *gin.Context) {
	providerID := c.Param("id")

	var req types.AuthOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	cop, isCopilot := provider.(*copilot.GitHubCopilotProvider)
	gen, isGeneric := provider.(*generic.GenericProvider)

	// ── Alibaba OAuth device-code flow ───────────────────────────────────────
	if isGeneric && gen.GetID() == "alibaba" && req.Method == "oauth" {
		flow, err := alibabapkg.InitiateDeviceFlow(context.Background())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to initiate Alibaba OAuth: %v", err),
			})
			return
		}

		// Pick the best verification URL to show the user.
		// Append prompt=login so Qwen shows the account chooser instead of
		// auto-signing in with the already-logged-in session.
		verifyURL := flow.VerificationURIComplete
		if verifyURL == "" {
			verifyURL = flow.VerificationURI
		}
		if verifyURL != "" && !strings.Contains(verifyURL, "prompt=") {
			sep := "&"
			if !strings.Contains(verifyURL, "?") {
				sep = "?"
			}
			verifyURL += sep + "prompt=login"
		}

		// Poll for the token in the background.
		alibabaCtx, alibabaCancel := context.WithCancel(context.Background())
		alibabaFlow := flow
		alibabaGen := gen
		alibabaReg := providerRegistry

		activeAuthFlowMu.Lock()
		activeAuthFlow = &authFlowState{
			ProviderID:     providerID,
			Status:         "awaiting_user",
			InstructionURL: verifyURL,
			UserCode:       flow.UserCode,
			deviceCode:     flow.DeviceCode,
			codeVerifier:   flow.CodeVerifier,
			cancelFn:       alibabaCancel,
		}
		activeAuthFlowMu.Unlock()

		go func() {
			defer alibabaCancel()
			td, err := alibabapkg.PollForToken(
				alibabaCtx,
				alibabaFlow.DeviceCode,
				alibabaFlow.CodeVerifier,
				alibabaFlow.Interval,
				alibabaFlow.ExpiresIn,
			)
			if err != nil {
				// Ignore cancellation — user cancelled intentionally
				if alibabaCtx.Err() != nil {
					return
				}
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
					activeAuthFlow.Status = "error"
					activeAuthFlow.Error = err.Error()
				}
				activeAuthFlowMu.Unlock()
				log.Error().Err(err).Str("provider", providerID).Msg("Alibaba OAuth device code flow failed")
				return
			}

			newInstanceID, err := alibabaGen.SaveAlibabaOAuthToken(td)
			if err != nil {
				log.Error().Err(err).Str("provider", providerID).Msg("Failed to save Alibaba OAuth token")
			}

			// Rename in registry: "alibaba-2" → "alibaba-oauth-china" etc.
			if newInstanceID != "" && newInstanceID != providerID {
				if err := alibabaReg.Rename(providerID, newInstanceID); err != nil {
					log.Warn().Err(err).Str("old", providerID).Str("new", newInstanceID).Msg("Failed to rename Alibaba provider in registry")
				} else {
					log.Info().Str("old", providerID).Str("new", newInstanceID).Msg("Alibaba provider renamed")
				}
			}

			if err := alibabaReg.Register(alibabaGen, true); err != nil {
				log.Warn().Err(err).Str("provider", alibabaGen.GetInstanceID()).Msg("Failed to update Alibaba provider in registry")
			}

			activeAuthFlowMu.Lock()
			if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
				activeAuthFlow.Status = "complete"
			}
			activeAuthFlowMu.Unlock()

			log.Info().Str("provider", providerID).Msg("Alibaba OAuth completed")
		}()

		c.JSON(http.StatusOK, gin.H{
			"success":          false,
			"requiresAuth":     true,
			"user_code":        flow.UserCode,
			"verification_uri": verifyURL,
			"message":          fmt.Sprintf("Visit %s and enter code: %s", verifyURL, flow.UserCode),
		})
		return
	}

	// Handle OAuth device code flow for GitHub Copilot
	if isCopilot && (req.Method == "oauth" || (req.GithubToken == "" && req.Token == "")) {
		// Start device code flow in a goroutine, return immediately with requiresAuth
		deviceCode, err := cop.InitiateDeviceCodeFlow()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
			})
			return
		}

		// Set auth flow state to awaiting_user
		copilotCtx, copilotCancel := context.WithCancel(context.Background())
		activeAuthFlowMu.Lock()
		activeAuthFlow = &authFlowState{
			ProviderID:     providerID,
			Status:         "awaiting_user",
			InstructionURL: deviceCode.VerificationURI,
			UserCode:       deviceCode.UserCode,
			deviceCode:     deviceCode.DeviceCode,
			cancelFn:       copilotCancel,
		}
		activeAuthFlowMu.Unlock()

		// Start polling for the access token in a goroutine
		dc := deviceCode
		prov := cop
		reg := providerRegistry
		go func() {
			defer copilotCancel()
			if err := prov.PollAndCompleteDeviceCodeFlow(dc); err != nil {
				// Ignore if cancelled
				if copilotCtx.Err() != nil {
					return
				}
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
					activeAuthFlow.Status = "error"
					activeAuthFlow.Error = err.Error()
				}
				activeAuthFlowMu.Unlock()
				log.Error().Err(err).Str("provider", providerID).Msg("OAuth device code flow failed")
				return
			}

			// Save token to database
			if err := prov.SaveToDB(); err != nil {
				log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token")
			}

			// Persist updated provider name to DB
			if err := reg.Register(prov, true); err != nil {
				log.Warn().Err(err).Str("provider", providerID).Msg("Failed to update provider in registry")
			}

			// Mark complete
			activeAuthFlowMu.Lock()
			if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
				activeAuthFlow.Status = "complete"
			}
			activeAuthFlowMu.Unlock()

			log.Info().Str("provider", providerID).Msg("GitHub Copilot OAuth completed")
		}()

		c.JSON(http.StatusOK, gin.H{
			"success":          false,
			"requiresAuth":     true,
			"user_code":        deviceCode.UserCode,
			"verification_uri": deviceCode.VerificationURI,
			"message":          fmt.Sprintf("Visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
		})
		return
	}

	// Direct token auth for GitHub Copilot
	if isCopilot && (req.Token != "" || req.GithubToken != "") {
		token := req.Token
		if token == "" {
			token = req.GithubToken
		}
		req.GithubToken = token
	}

	if err := provider.SetupAuth(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Authentication failed: %v", err),
		})
		return
	}

	// Save token to database if provider supports it
	if isCopilot {
		if err := cop.SaveToDB(); err != nil {
			log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token to database")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider authenticated successfully",
	})
}

func handleInitiateDeviceCode(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// ── Alibaba OAuth device-code initiation ──────────────────────────────
	if gen, ok := provider.(*generic.GenericProvider); ok && gen.GetID() == "alibaba" {
		flow, err := alibabapkg.InitiateDeviceFlow(context.Background())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to initiate Alibaba device code flow: %v", err),
			})
			return
		}

		verifyURL := flow.VerificationURIComplete
		if verifyURL == "" {
			verifyURL = flow.VerificationURI
		}

		// Store the flow state (including PKCE verifier) for the frontend-driven poll.
		activeAuthFlowMu.Lock()
		activeAuthFlow = &authFlowState{
			ProviderID:     providerID,
			Status:         "awaiting_user",
			InstructionURL: verifyURL,
			UserCode:       flow.UserCode,
			deviceCode:     flow.DeviceCode,
			codeVerifier:   flow.CodeVerifier,
		}
		activeAuthFlowMu.Unlock()

		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"user_code":        flow.UserCode,
			"device_code":      flow.DeviceCode,
			"verification_uri": verifyURL,
			"expires_in":       flow.ExpiresIn,
			"interval":         flow.Interval,
			"message":          fmt.Sprintf("Please visit %s and enter code: %s", verifyURL, flow.UserCode),
		})
		return
	}

	// ── GitHub Copilot device-code initiation ─────────────────────────────
	cop, ok := provider.(*copilot.GitHubCopilotProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Provider does not support device code OAuth flow",
		})
		return
	}

	deviceCode, err := cop.InitiateDeviceCodeFlow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to initiate device code flow: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"user_code":        deviceCode.UserCode,
		"device_code":      deviceCode.DeviceCode,
		"verification_uri": deviceCode.VerificationURI,
		"expires_in":       deviceCode.ExpiresIn,
		"interval":         deviceCode.Interval,
		"message":          fmt.Sprintf("Please visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
	})
}

func handleCompleteDeviceCode(c *gin.Context) {
	providerID := c.Param("id")

	var req struct {
		DeviceCode   string `json:"device_code"`
		UserCode     string `json:"user_code"`
		CodeVerifier string `json:"code_verifier"` // Alibaba PKCE; optional for GitHub
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body - requires device_code and user_code"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// ── Alibaba OAuth device-code completion ──────────────────────────────
	if gen, ok := provider.(*generic.GenericProvider); ok && gen.GetID() == "alibaba" {
		td, err := alibabapkg.PollForToken(
			context.Background(),
			req.DeviceCode,
			req.CodeVerifier,
			5,   // default interval
			600, // default expires_in
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to complete Alibaba OAuth: %v", err),
			})
			return
		}

		newInstanceID, err := gen.SaveAlibabaOAuthToken(td)
		if err != nil {
			log.Error().Err(err).Str("provider", providerID).Msg("Failed to save Alibaba OAuth token")
		}

		// Rename in registry: e.g. "alibaba-2" → "alibaba-oauth-china"
		if newInstanceID != "" && newInstanceID != providerID {
			if err := providerRegistry.Rename(providerID, newInstanceID); err != nil {
				log.Warn().Err(err).Str("old", providerID).Str("new", newInstanceID).Msg("Failed to rename Alibaba provider in registry")
			} else {
				log.Info().Str("old", providerID).Str("new", newInstanceID).Msg("Alibaba provider renamed")
			}
		}

		if err := providerRegistry.Register(gen, true); err != nil {
			log.Warn().Err(err).Str("provider", gen.GetInstanceID()).Msg("Failed to update Alibaba provider in registry")
		}

		activeAuthFlowMu.Lock()
		if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
			activeAuthFlow.Status = "complete"
		}
		activeAuthFlowMu.Unlock()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Alibaba authenticated successfully",
			"provider": gin.H{
				"id":   gen.GetInstanceID(),
				"name": gen.GetName(),
				"type": "alibaba",
			},
		})
		return
	}

	// ── GitHub Copilot device-code completion ─────────────────────────────
	cop, ok := provider.(*copilot.GitHubCopilotProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Provider does not support device code OAuth flow",
		})
		return
	}

	// Reconstruct the device code response for polling
	deviceCodeResp := &ghservice.DeviceCodeResponse{
		DeviceCode:      req.DeviceCode,
		UserCode:        req.UserCode,
		VerificationURI: "",  // Not needed for polling
		ExpiresIn:       600, // Default 10 minutes
		Interval:        5,   // Default 5 second poll interval
	}

	// Poll for access token
	if err := cop.PollAndCompleteDeviceCodeFlow(deviceCodeResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to complete OAuth: %v", err),
		})
		return
	}

	// Save tokens to database
	if err := cop.SaveToDB(); err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to save tokens to database")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GitHub Copilot authenticated successfully",
		"provider": gin.H{
			"id":   cop.GetInstanceID(),
			"name": cop.GetName(),
			"type": "github-copilot",
		},
	})
}

func handleUpdateProviderConfig(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	previousConfig, _ := loadProviderConfig(providerID)
	normalizedConfig := normalizeProviderConfigForStorage(provider.GetID(), config)
	if len(normalizedConfig) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid configuration fields supplied"})
		return
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(providerID, normalizedConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist provider config"})
		return
	}

	if provider.GetID() == "azure-openai" {
		oldDeployments := stringSliceValue(previousConfig["deployments"])
		newDeployments := stringSliceValue(normalizedConfig["deployments"])
		if len(oldDeployments) > 0 {
			newSet := make(map[string]struct{}, len(newDeployments))
			for _, deployment := range newDeployments {
				newSet[deployment] = struct{}{}
			}

			modelStateStore := database.NewModelStateStore()
			modelConfigStore := database.NewModelConfigStore()
			for _, deployment := range oldDeployments {
				if _, keep := newSet[deployment]; keep {
					continue
				}
				_ = modelStateStore.Delete(providerID, deployment)
				_ = modelConfigStore.Delete(providerID, deployment)
			}
		}
	}

	log.Info().
		Str("provider", providerID).
		Msg("Provider config update requested")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration updated for %s", providerID),
		"config":  normalizeProviderConfigForFrontend(provider.GetID(), normalizedConfig),
	})
}

func handleActivateProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if !providerRegistry.IsRegistered(providerID) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Provider " + providerID + " not found",
		})
		return
	}

	provider, err := providerRegistry.AddActive(providerID)
	if err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to activate provider")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to activate provider: " + err.Error(),
		})
		return
	}

	log.Info().Str("provider", providerID).Msg("Provider activated")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider " + providerID + " activated",
		"provider": gin.H{
			"id":   provider.GetInstanceID(),
			"name": provider.GetName(),
		},
	})
}

func handleDeactivateProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if !providerRegistry.IsRegistered(providerID) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Provider " + providerID + " not found",
		})
		return
	}

	if err := providerRegistry.RemoveActive(providerID); err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to deactivate provider")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to deactivate provider: " + err.Error(),
		})
		return
	}

	log.Info().Str("provider", providerID).Msg("Provider deactivated")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider " + providerID + " deactivated",
	})
}

// ─── System info ───

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

func makeGetInfoHandler(port int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": GetVersion(),
			"port":    port,
			"backend": "golang",
			"uptime":  time.Since(serverStartTime).String(),
			"build":   "dev",
			"features": gin.H{
				"cif_format":        true,
				"model_routing":     true,
				"rate_limiting":     true,
				"manual_approval":   true,
				"database":          true,
				"chat_history":      true,
				"provider_adapters": true,
				"streaming":         true,
			},
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
			if models, err := loadProviderModels(provider); err == nil {
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
	c.JSON(http.StatusOK, gin.H{
		"level":  currentLogLevel.String(),
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

	currentLogLevel = level
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
	c.ShouldBindJSON(&body)
	log.Debug().Interface("payload", body).Msg("Debug log entry")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func handleGetModelVersion(c *gin.Context) {
	instanceID := c.Param("id")
	modelID := c.Param("modelId")

	configStore := database.NewModelConfigStore()
	record, err := configStore.Get(instanceID, modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get model version"})
		return
	}

	version := ""
	if record != nil {
		version = record.Version
	}
	c.JSON(http.StatusOK, gin.H{"version": version})
}

func handleSetModelVersion(c *gin.Context) {
	instanceID := c.Param("id")
	modelID := c.Param("modelId")

	var req struct {
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	configStore := database.NewModelConfigStore()
	if err := configStore.SetVersion(instanceID, modelID, req.Version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set model version"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "version": req.Version})
}

// ─── Chat sessions ───

func handleGetChatSessions(c *gin.Context) {
	chatStore := database.NewChatStore()
	sessions, err := chatStore.ListSessions()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get chat sessions")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chat sessions"})
		return
	}

	var sessionList []map[string]interface{}
	for _, session := range sessions {
		sessionInfo := map[string]interface{}{
			"id":         session.SessionID,
			"title":      session.Title,
			"model_id":   session.ModelID,
			"api_shape":  session.APIShape,
			"created_at": session.CreatedAt.Format(time.RFC3339),
			"updated_at": session.UpdatedAt.Format(time.RFC3339),
		}

		messages, err := chatStore.GetMessages(session.SessionID)
		if err == nil {
			sessionInfo["message_count"] = len(messages)
		} else {
			sessionInfo["message_count"] = 0
		}

		sessionList = append(sessionList, sessionInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessionList,
		"total":    len(sessionList),
	})
}

func handleCreateChatSession(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
		ModelID   string `json:"model_id"`
		APIShape  string `json:"api_shape"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.APIShape == "" {
		req.APIShape = "openai"
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	chatStore := database.NewChatStore()
	if err := chatStore.CreateSession(sessionID, req.Title, req.ModelID, req.APIShape); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
	})
}

func handleDeleteAllChatSessions(c *gin.Context) {
	chatStore := database.NewChatStore()
	if err := chatStore.DeleteAllSessions(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "All sessions deleted",
	})
}

func handleGetChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	chatStore := database.NewChatStore()

	session, err := chatStore.GetSession(sessionID)
	if err != nil || session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	messages, err := chatStore.GetMessages(sessionID)
	if err != nil {
		messages = nil
	}

	var messageList []map[string]interface{}
	for _, msg := range messages {
		messageList = append(messageList, map[string]interface{}{
			"id":         msg.MessageID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         session.SessionID,
		"title":      session.Title,
		"model_id":   session.ModelID,
		"api_shape":  session.APIShape,
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
		"messages":   messageList,
	})
}

func handleUpdateChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	chatStore := database.NewChatStore()
	if err := chatStore.UpdateSessionTitle(sessionID, req.Title); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session updated",
	})
}

func handleAddChatMessage(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	chatStore := database.NewChatStore()

	if err := chatStore.AddMessage(messageID, sessionID, req.Role, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add message"})
		return
	}

	// Touch session updated_at
	chatStore.TouchSession(sessionID)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message_id": messageID,
	})
}

func handleDeleteChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	chatStore := database.NewChatStore()

	if err := chatStore.DeleteSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session deleted",
	})
}

// ─── Log streaming (SSE) ───

func handleLogsStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	sub := &logSubscriber{
		ch:   make(chan string, 64),
		done: make(chan struct{}),
	}

	logSubscribersMu.Lock()
	logSubscribers[sub] = struct{}{}
	logSubscribersMu.Unlock()

	defer func() {
		logSubscribersMu.Lock()
		delete(logSubscribers, sub)
		logSubscribersMu.Unlock()
		close(sub.done)
	}()

	flusher := c.Writer.(http.Flusher)
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	if _, err := io.WriteString(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case data := <-sub.ch:
			if _, err := io.WriteString(c.Writer, data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
