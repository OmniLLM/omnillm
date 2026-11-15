package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	codexpkg "omnillm/internal/providers/codex"
	copilot "omnillm/internal/providers/copilot"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"
	modelscopepkg "omnillm/internal/providers/modelscope"
	openaipkg "omnillm/internal/providers/openai"
	openaicompatprovider "omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	typesafepkg "omnillm/internal/providers/typesafe"
	"omnillm/internal/registry"
	ghservice "omnillm/internal/services/github"
)

// Provider endpoints

func handleGetProviders(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()
	providers, releaseProviders, err := providerRegistry.BorrowProviders()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Provider identity is being updated; retry the request"})
		return
	}
	defer releaseProviders()

	providerList := make([]map[string]interface{}, len(providers))
	instanceStore := database.NewProviderInstanceStore()
	var wg sync.WaitGroup
	for i, provider := range providers {
		wg.Add(1)
		go func(i int, provider types.Provider) {
			defer wg.Done()
			config, configErr := loadProviderConfig(provider.GetInstanceID())
			if configErr != nil {
				log.Warn().Err(configErr).Str("provider", provider.GetInstanceID()).Msg("Failed to load provider config")
			}

			authStatus := "unauthenticated"
			if provider.GetToken() != "" {
				authStatus = "authenticated"
			}

			name := provider.GetName()
			subtitle := ""
			if dbRecord, dbErr := instanceStore.Get(provider.GetInstanceID()); dbErr == nil && dbRecord != nil {
				if dbRecord.Name != "" {
					name = dbRecord.Name
				}
				subtitle = dbRecord.Subtitle
			}

			providerInfo := map[string]interface{}{
				"id":         provider.GetInstanceID(),
				"type":       provider.GetID(),
				"name":       name,
				"alias":      subtitle,
				"subtitle":   subtitle,
				"isActive":   providerRegistry.IsActiveProvider(provider.GetInstanceID()),
				"authStatus": authStatus,
			}

			if normalizedConfig := normalizeProviderConfigForFrontend(provider.GetID(), config); normalizedConfig != nil {
				providerInfo["config"] = normalizedConfig
			}

			if authStatus == "authenticated" {
				models, err := loadProviderModels(provider, false)
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

			providerList[i] = providerInfo
		}(i, provider)
	}
	wg.Wait()

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
	resolvedID, err := resolveProviderReference(providerID)
	if err != nil {
		writeProviderReferenceError(c, providerID, err)
		return
	}
	providerID = resolvedID

	providerRegistry := registry.GetProviderRegistry()
	_, releaseProvider, err := providerRegistry.BorrowProvider(providerID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Provider is unavailable or busy. Please retry."})
		return
	}
	defer releaseProvider()
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

// handleAuthAndCreateProvider creates the provider-instance parent before
// authentication persists credentials or configuration. Failed creation rolls
// back the reserved parent and its cascading child records.
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

	var reservation *providerInstanceReservation
	switch providerType {
	case "alibaba", "alibaba-modelscope", "github-copilot", "codex", "azure-openai", "google", "kimi", "typesafe", "openai-compatible":
		var err error
		reservation, err = reserveRandomProvider(providerType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to reserve a provider ID"})
			return
		}
	}
	defer func() {
		if reservation != nil {
			cleanupProviderReservation(reservation)
		}
	}()

	switch providerType {

	// ——————————————————————————————————————————————————————————————
	case "alibaba":
		if req.Method == "oauth" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "OAuth authentication is not supported for Alibaba DashScope — please use API key authentication",
			})
			return
		}

		prov := alibabapkg.NewProvider(reservation.instanceID, "")
		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "alibaba-modelscope":
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "API key is required for ModelScope authentication",
			})
			return
		}

		prov := modelscopepkg.NewProvider(reservation.instanceID, "")
		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "github-copilot":
		cop := copilot.NewGitHubCopilotProvider(reservation.instanceID, "")

		if req.Method == "oauth" || (req.GithubToken == "" && req.Token == "") {
			deviceCode, err := cop.InitiateDeviceCodeFlow()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
				})
				return
			}

			pendingID := reservation.instanceID
			copilotCtx, copilotCancel := context.WithCancel(context.WithoutCancel(c.Request.Context()))
			flow := &authFlowState{
				ProviderID:     pendingID,
				Status:         "awaiting_user",
				InstructionURL: deviceCode.VerificationURI,
				UserCode:       deviceCode.UserCode,
				deviceCode:     deviceCode.DeviceCode,
				cancelFn:       copilotCancel,
			}
			activeAuthFlowMu.Lock()
			if activeAuthFlow != nil && activeAuthFlow.cancelFn != nil {
				activeAuthFlow.cancelFn()
			}
			activeAuthFlow = flow
			activeAuthFlowMu.Unlock()

			dc := deviceCode
			asyncReservation := reservation
			reservation = nil
			go func() {
				defer copilotCancel()
				defer cleanupProviderReservation(asyncReservation)
				if err := cop.PollAndCompleteDeviceCodeFlowContext(copilotCtx, dc); err != nil {
					if copilotCtx.Err() != nil {
						return
					}
					activeAuthFlowMu.Lock()
					if activeAuthFlow == flow {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = err.Error()
					}
					activeAuthFlowMu.Unlock()
					log.Error().Err(err).Str("type", "github-copilot").Msg("Auth-and-create: OAuth failed")
					return
				}

				canonicalID := asyncReservation.instanceID
				activeAuthFlowMu.Lock()
				if copilotCtx.Err() != nil || activeAuthFlow != flow {
					activeAuthFlowMu.Unlock()
					return
				}
				if err := createReservedProvider(asyncReservation, cop, func() error {
					if err := cop.SaveToDB(); err != nil {
						return fmt.Errorf("save GitHub Copilot token: %w", err)
					}
					return nil
				}); err != nil {
					activeAuthFlow.Status = "error"
					activeAuthFlow.Error = err.Error()
					activeAuthFlowMu.Unlock()
					log.Error().Err(err).Str("provider", canonicalID).Msg("Auth-and-create: failed to persist GitHub Copilot provider")
					return
				}
				activeAuthFlow.Status = "complete"
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

		if err := createReservedProvider(reservation, cop, func() error {
			if err := cop.SaveToDB(); err != nil {
				return fmt.Errorf("save provider credentials: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
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

	// ——————————————————————————————————————————————————————————————
	case "codex":
		if strings.TrimSpace(req.APIKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "apiKey is required for Codex authentication",
			})
			return
		}

		cdx := codexpkg.NewCodexProvider(reservation.instanceID)
		if err := createReservedProvider(reservation, cdx, func() error {
			if err := cdx.SetupAuth(&req); err != nil {
				return fmt.Errorf("Codex authentication failed: %w", err)
			}
			if err := cdx.SaveToDB(); err != nil {
				return fmt.Errorf("save Codex credentials: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         cdx.GetInstanceID(),
				"type":       cdx.GetID(),
				"name":       cdx.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "openai":
		// Credentials arrive through the ChatGPT browser OAuth flow; there is
		// no API-key path for this provider.
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Use POST /api/admin/providers/openai/start-oauth to sign in with ChatGPT",
		})

	// ——————————————————————————————————————————————————————————————
	case "antigravity":
		// The auth-and-create path is no longer used for Antigravity;
		// the frontend calls POST /providers/antigravity/start-oauth directly.
		// Return a clear error so any stale callers know which endpoint to use.
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Use POST /api/admin/providers/antigravity/start-oauth to begin Google OAuth",
		})

	// ——————————————————————————————————————————————————————————————
	case "azure-openai":
		prov := azurepkg.NewProvider(reservation.instanceID, "")

		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			if rawCfg, cfgErr := loadProviderConfig(prov.GetInstanceID()); cfgErr == nil && rawCfg != nil {
				normCfg := normalizeProviderConfigForStorage(providerType, rawCfg)
				if len(normCfg) > 0 {
					if err := database.NewProviderConfigStore().Save(prov.GetInstanceID(), normCfg); err != nil {
						return fmt.Errorf("save provider config: %w", err)
					}
				}
			}

			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		if req.Deployments != "" {
			var rawDeployments []interface{}
			if jsonErr := json.Unmarshal([]byte(req.Deployments), &rawDeployments); jsonErr == nil {
				modelStateStore := database.NewModelStateStore()
				for _, item := range rawDeployments {
					switch typed := item.(type) {
					case string:
						deployment := strings.TrimSpace(typed)
						if deployment != "" {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), deployment, true)
						}
					case map[string]interface{}:
						model, _ := typed["model"].(string)
						deployment, _ := typed["deployment"].(string)
						model = strings.TrimSpace(model)
						deployment = strings.TrimSpace(deployment)
						if model == "" {
							model = deployment
						}
						if deployment == "" {
							deployment = model
						}
						if model != "" {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), model, true)
						}
						if deployment != "" && deployment != model {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), deployment, true)
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "google":
		prov := googlepkg.NewProvider(reservation.instanceID, "")

		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "kimi":
		prov := kimipkg.NewProvider(reservation.instanceID, "")

		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	case "typesafe":
		prov := typesafepkg.NewProvider(reservation.instanceID, "")

		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "openai-compatible":
		prov := openaicompatprovider.NewProvider(reservation.instanceID, "")

		if err := createReservedProvider(reservation, prov, func() error {
			if err := prov.SetupAuth(&req); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		log.Info().Str("type", providerType).Str("instance_id", prov.GetInstanceID()).Str("name", prov.GetName()).Msg("Auth-and-create: provider registered")

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
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
	providerMetadataMu.Lock()
	defer providerMetadataMu.Unlock()

	providerRegistry := registry.GetProviderRegistry()
	if err := providerRegistry.Remove(providerID); err != nil {
		if errors.Is(err, registry.ErrProviderBusy) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Provider is busy. Please retry."})
			return
		}
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
	_, releaseProviders, err := registry.GetProviderRegistry().BorrowProviders()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Provider identity is being updated; retry the request"})
		return
	}
	defer releaseProviders()
	providerMetadataMu.Lock()
	defer providerMetadataMu.Unlock()

	instanceStore := database.NewProviderInstanceStore()
	canonicalPriorities := make(map[string]int, len(req.Priorities))
	for reference, priority := range req.Priorities {
		id, resolveErr := resolveProviderReference(reference)
		if resolveErr != nil {
			writeProviderReferenceError(c, reference, resolveErr)
			return
		}
		if _, duplicate := canonicalPriorities[id]; duplicate {
			c.JSON(http.StatusBadRequest, gin.H{"error": "multiple priority entries resolve to provider " + id})
			return
		}
		canonicalPriorities[id] = priority
	}
	for id, priority := range canonicalPriorities {
		record, err := instanceStore.Get(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve provider " + id})
			return
		}
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Provider " + id + " not found"})
			return
		}
		record.Priority = priority
		if err := instanceStore.Save(record); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update provider priority"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider priorities updated",
	})
}

func handleListProviderModels(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, releaseProvider, err := providerRegistry.BorrowProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	defer releaseProvider()

	models, err := loadProviderModels(provider, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"total":  len(models),
	})
}

func handleRefreshProviderModels(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Force refresh: bypass cache and call external API
	models, err := loadProviderModels(provider, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"total":  len(models),
		"cached": false,
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
	provider, releaseProvider, err := providerRegistry.BorrowProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	defer releaseProvider()

	cop, isCopilot := provider.(*copilot.GitHubCopilotProvider)

	// Codex: API-key auth only.
	if _, isCodex := provider.(*codexpkg.CodexProvider); isCodex {
		if strings.TrimSpace(req.APIKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "apiKey is required for Codex authentication",
			})
			return
		}
	}

	// OpenAI (ChatGPT OAuth): re-auth must go through the browser flow.
	if _, isOpenAI := provider.(*openaipkg.Provider); isOpenAI {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Use POST /api/admin/providers/openai/start-oauth to re-authenticate with ChatGPT",
		})
		return
	}

	// Alibaba: API-key only — OAuth is not supported.
	if aliProv, ok := provider.(*alibabapkg.Provider); ok {
		if req.Method == "oauth" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "OAuth authentication is not supported for Alibaba DashScope — please use API key authentication",
			})
			return
		}
		if err := aliProv.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Authentication failed: %v", err)})
			return
		}
		if err := providerRegistry.TouchProvider(providerID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Failed to update provider: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         aliProv.GetInstanceID(),
				"type":       aliProv.GetID(),
				"name":       aliProv.GetName(),
				"authStatus": "authenticated",
			},
		})
		return
	}

	// ModelScope: API-key only.
	if msProv, ok := provider.(*modelscopepkg.Provider); ok {
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "API key is required for ModelScope authentication",
			})
			return
		}
		if err := msProv.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Authentication failed: %v", err)})
			return
		}
		if err := providerRegistry.TouchProvider(providerID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Failed to update provider: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         msProv.GetInstanceID(),
				"type":       msProv.GetID(),
				"name":       msProv.GetName(),
				"authStatus": "authenticated",
			},
		})
		return
	}

	// Handle OAuth device code flow for GitHub Copilot
	if isCopilot && (req.Method == "oauth" || (req.GithubToken == "" && req.Token == "")) {
		_, releaseFlow, err := providerRegistry.BorrowProvider(providerID)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Provider is busy. Please retry."})
			return
		}
		// Start device code flow in a goroutine, return immediately with requiresAuth
		deviceCode, err := cop.InitiateDeviceCodeFlow()
		if err != nil {
			releaseFlow()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
			})
			return
		}

		// Set auth flow state to awaiting_user
		copilotCtx, copilotCancel := context.WithCancel(context.WithoutCancel(c.Request.Context()))
		flow := &authFlowState{
			ProviderID:     providerID,
			Status:         "awaiting_user",
			InstructionURL: deviceCode.VerificationURI,
			UserCode:       deviceCode.UserCode,
			deviceCode:     deviceCode.DeviceCode,
			cancelFn:       copilotCancel,
		}
		activeAuthFlowMu.Lock()
		if activeAuthFlow != nil && activeAuthFlow.cancelFn != nil {
			activeAuthFlow.cancelFn()
		}
		activeAuthFlow = flow
		activeAuthFlowMu.Unlock()

		// Start polling for the access token in a goroutine
		dc := deviceCode
		prov := cop
		reg := providerRegistry
		go func() {
			defer copilotCancel()
			defer releaseFlow()
			if err := prov.PollAndCompleteDeviceCodeFlowContext(copilotCtx, dc); err != nil {
				// Ignore if cancelled
				if copilotCtx.Err() != nil {
					return
				}
				activeAuthFlowMu.Lock()
				if activeAuthFlow == flow {
					activeAuthFlow.Status = "error"
					activeAuthFlow.Error = err.Error()
				}
				activeAuthFlowMu.Unlock()
				log.Error().Err(err).Str("provider", providerID).Msg("OAuth device code flow failed")
				return
			}
			activeAuthFlowMu.Lock()
			if copilotCtx.Err() != nil || activeAuthFlow != flow {
				activeAuthFlowMu.Unlock()
				return
			}
			// Serialize final persistence with cancellation.
			if err := prov.SaveToDB(); err != nil {
				activeAuthFlow.Status = "error"
				activeAuthFlow.Error = "Failed to save provider credentials"
				activeAuthFlowMu.Unlock()
				log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token")
				return
			}

			// Persist updated provider name to DB
			if err := reg.TouchProvider(providerID); err != nil {
				log.Warn().Err(err).Str("provider", providerID).Msg("Failed to update provider in registry")
			}

			activeAuthFlow.Status = "complete"
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
	} else if codex, ok := provider.(*codexpkg.CodexProvider); ok {
		if err := codex.SaveToDB(); err != nil {
			log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token to database")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save provider credentials"})
			return
		}
	}

	if provider.GetID() == "azure-openai" {
		if rawCfg, cfgErr := loadProviderConfig(providerID); cfgErr == nil && rawCfg != nil {
			normCfg := normalizeProviderConfigForStorage(provider.GetID(), rawCfg)
			if len(normCfg) > 0 {
				_ = database.NewProviderConfigStore().Save(providerID, normCfg)
			}
		}
	}

	// Invalidate model cache on re-auth
	cacheStore := database.NewProviderModelsCacheStore()
	if err := cacheStore.Delete(providerID); err != nil {
		log.Warn().Err(err).Str("provider", providerID).Msg("Failed to invalidate model cache")
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

	// ——————————————————————————————————————————————————————————————
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

	// ——————————————————————————————————————————————————————————————
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
	if err := cop.PollAndCompleteDeviceCodeFlowContext(c.Request.Context(), deviceCodeResp); err != nil {
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
	if provider.GetID() == "openai-compatible" {
		normalizedConfig = mergeOpenAICompatibleConfig(previousConfig, config, normalizedConfig)
		if mode, _ := normalizedConfig["prompt_cache_mode"].(string); mode == openaicompatprovider.PromptCacheModeAnthropicInline && strings.EqualFold(strings.TrimSuffix(provider.GetBaseURL(), "/v1"), "https://api.openai.com") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "anthropic_inline prompt cache mode is not valid for api.openai.com"})
			return
		}
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(providerID, normalizedConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist provider config"})
		return
	}
	if compatible, ok := provider.(*openaicompatprovider.Provider); ok {
		compatible.ApplyConfig(normalizedConfig)
	}

	if provider.GetID() == "azure-openai" {
		oldModels := azurepkg.ModelIDs(previousConfig)
		newModels := azurepkg.ModelIDs(normalizedConfig)
		if len(oldModels) > 0 {
			newSet := make(map[string]struct{}, len(newModels))
			for _, model := range newModels {
				newSet[model] = struct{}{}
			}

			modelStateStore := database.NewModelStateStore()
			modelConfigStore := database.NewModelConfigStore()
			for _, model := range oldModels {
				if _, keep := newSet[model]; keep {
					continue
				}
				_ = modelStateStore.Delete(providerID, model)
				_ = modelConfigStore.Delete(providerID, model)
			}
		}
	}

	log.Info().
		Str("provider", providerID).
		Msg("Provider config update requested")

	// Invalidate model cache when config changes (may affect available models)
	modelsCacheStore := database.NewProviderModelsCacheStore()
	if err := modelsCacheStore.Delete(providerID); err != nil {
		log.Warn().Err(err).Str("provider", providerID).Msg("Failed to invalidate model cache")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration updated for %s", providerID),
		"config":  normalizeProviderConfigForFrontend(provider.GetID(), normalizedConfig),
	})
}

func handleRenameProvider(c *gin.Context) {
	providerID := c.Param("id")

	var req struct {
		Name     *string `json:"name"`
		Alias    *string `json:"alias"`
		Subtitle *string `json:"subtitle"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if req.Alias != nil && req.Subtitle != nil && strings.TrimSpace(*req.Alias) != strings.TrimSpace(*req.Subtitle) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias and subtitle must match when both are provided"})
		return
	}
	if req.Alias != nil {
		req.Subtitle = req.Alias
	}
	if req.Name == nil && req.Subtitle == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one of 'name' or 'alias' must be provided"})
		return
	}
	providerMetadataMu.Lock()
	defer providerMetadataMu.Unlock()

	instanceStore := database.NewProviderInstanceStore()
	record, err := instanceStore.Get(providerID)
	if err != nil || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider instance not found in database"})
		return
	}

	var newName *string
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
			return
		}
		newName = &trimmed
		record.Name = trimmed
		providerRegistry := registry.GetProviderRegistry()
		if provider, provErr := providerRegistry.GetProvider(providerID); provErr == nil {
			provider.SetName(trimmed)
		}
		log.Info().Str("provider", providerID).Str("name", trimmed).Msg("Provider renamed")
	}

	var newSubtitle *string
	if req.Subtitle != nil {
		trimmed := strings.TrimSpace(*req.Subtitle)
		newSubtitle = &trimmed
		record.Subtitle = trimmed
		log.Info().Str("provider", providerID).Str("subtitle", trimmed).Msg("Provider subtitle updated")
	}

	if err := instanceStore.UpdateMetadata(providerID, newName, newSubtitle); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist provider metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "name": record.Name, "alias": record.Subtitle, "subtitle": record.Subtitle})
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
