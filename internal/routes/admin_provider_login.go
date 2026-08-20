package routes

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
)

type providerLoginFlow struct {
	Kind       string
	ProviderID string
	IsNew      bool
	ExpiresAt  time.Time
}

var providerLoginFlows = struct {
	sync.Mutex
	values map[string]providerLoginFlow
}{values: make(map[string]providerLoginFlow)}

var supportedProviderLoginTypes = map[string]struct{}{
	"github-copilot": {}, "openai-compatible": {}, "alibaba": {},
	"azure-openai": {}, "google": {}, "antigravity": {}, "openai": {},
	"kimi": {}, "codex": {}, "modelscope": {},
}

func newProviderLoginFlow(flow providerLoginFlow) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	flow.ExpiresAt = time.Now().Add(10 * time.Minute)
	providerLoginFlows.Lock()
	providerLoginFlows.values[id] = flow
	providerLoginFlows.Unlock()
	return id, nil
}

func delegateProviderHandler(c *gin.Context, params gin.Params, body map[string]any, handler gin.HandlerFunc) (int, map[string]any) {
	payload, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	delegated, _ := gin.CreateTestContext(recorder)
	delegated.Request = c.Request.Clone(c.Request.Context())
	delegated.Request.Body = http.NoBody
	if len(payload) > 0 {
		delegated.Request.Body = io.NopCloser(bytes.NewReader(payload))
		delegated.Request.Header.Set("Content-Type", "application/json")
	}
	delegated.Params = params
	handler(delegated)
	result := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &result)
	return recorder.Code, result
}

func handleProviderLogin(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	typeName, _ := req["type"].(string)
	providerRef, _ := req["provider"].(string)
	subject, _ := req["subject"].(string)
	typeName, providerRef, subject = strings.TrimSpace(typeName), strings.TrimSpace(providerRef), strings.TrimSpace(subject)
	selected := 0
	for _, value := range []string{typeName, providerRef, subject} {
		if value != "" {
			selected++
		}
	}
	if selected != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "exactly one of type, provider, or subject is required"})
		return
	}

	isNew := typeName != ""
	providerID := ""
	if providerRef != "" {
		var err error
		providerID, err = resolveProviderReference(providerRef)
		if err != nil {
			writeProviderReferenceError(c, providerRef, err)
			return
		}
	} else if subject != "" {
		resolved, err := resolveProviderReference(subject)
		if err == nil {
			providerID = resolved
		} else if _, supported := supportedProviderLoginTypes[subject]; supported && errors.Is(err, database.ErrProviderReferenceNotFound) {
			typeName, isNew = subject, true
		} else {
			writeProviderReferenceError(c, subject, err)
			return
		}
	}

	if providerID != "" {
		record, err := database.NewProviderInstanceStore().Get(providerID)
		if err != nil || record == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider_not_found"})
			return
		}
		typeName = record.ProviderID
	}
	if _, supported := supportedProviderLoginTypes[typeName]; !supported {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider type", "type": typeName})
		return
	}

	delete(req, "type")
	delete(req, "provider")
	delete(req, "subject")
	if typeName == "openai" || typeName == "antigravity" {
		req["provider_id"] = providerID
		handler := handleOpenAIStartOAuth
		if typeName == "antigravity" {
			handler = handleAntigravityStartOAuth
		}
		status, result := delegateProviderHandler(c, nil, req, handler)
		if status >= 400 {
			c.JSON(status, result)
			return
		}
		resolvedID, _ := result["provider_id"].(string)
		flowID, err := newProviderLoginFlow(providerLoginFlow{Kind: typeName, ProviderID: resolvedID, IsNew: isNew})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create login flow"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"status": "pending", "flow_id": flowID, "provider_id": resolvedID, "is_new": isNew, "authorization_url": result["auth_url"]})
		return
	}

	params := gin.Params{{Key: "id", Value: providerID}}
	handler := handleProviderAuth
	if isNew {
		params = gin.Params{{Key: "type", Value: typeName}}
		handler = handleAuthAndCreateProvider
	}
	status, result := delegateProviderHandler(c, params, req, handler)
	if status >= 400 {
		c.JSON(status, result)
		return
	}
	if requires, _ := result["requiresAuth"].(bool); requires {
		activeAuthFlowMu.RLock()
		flowProviderID := ""
		if activeAuthFlow != nil {
			flowProviderID = activeAuthFlow.ProviderID
		}
		activeAuthFlowMu.RUnlock()
		flowID, err := newProviderLoginFlow(providerLoginFlow{Kind: "device", ProviderID: flowProviderID, IsNew: isNew})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create login flow"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"status": "pending", "flow_id": flowID, "provider_id": flowProviderID, "is_new": isNew, "authorization_url": result["verification_uri"], "user_code": result["user_code"]})
		return
	}
	resolvedID := providerID
	if provider, ok := result["provider"].(map[string]any); ok {
		resolvedID, _ = provider["id"].(string)
	}
	c.JSON(http.StatusOK, gin.H{"status": "complete", "provider_id": resolvedID, "is_new": isNew})
}

func handleProviderLoginStatus(c *gin.Context) {
	flowID := c.Param("flow_id")
	providerLoginFlows.Lock()
	flow, ok := providerLoginFlows.values[flowID]
	if ok && time.Now().After(flow.ExpiresAt) {
		delete(providerLoginFlows.values, flowID)
		ok = false
	}
	providerLoginFlows.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"status": "expired", "error": "login flow not found or expired"})
		return
	}

	status, errorMessage := "pending", ""
	switch flow.Kind {
	case "device":
		activeAuthFlowMu.RLock()
		if activeAuthFlow != nil && (activeAuthFlow.ProviderID == flow.ProviderID || flow.ProviderID == "") {
			status, errorMessage, flow.ProviderID = activeAuthFlow.Status, activeAuthFlow.Error, activeAuthFlow.ProviderID
		}
		activeAuthFlowMu.RUnlock()
	case "openai":
		openaiOAuthMu.Lock()
		if result := openaiOAuthResults[flow.ProviderID]; result != nil && result.Done {
			status, errorMessage = "complete", result.Error
		}
		openaiOAuthMu.Unlock()
	case "antigravity":
		agOAuthResultsMu.Lock()
		if result := agOAuthResults[flow.ProviderID]; result != nil && result.Done {
			status, errorMessage = "complete", result.Error
		}
		agOAuthResultsMu.Unlock()
	}
	if errorMessage != "" {
		status = "error"
	}
	if status == "awaiting_user" {
		status = "pending"
	}
	if status == "complete" || status == "error" {
		providerLoginFlows.Lock()
		delete(providerLoginFlows.values, flowID)
		providerLoginFlows.Unlock()
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "provider_id": flow.ProviderID, "is_new": flow.IsNew, "error": errorMessage})
}

func handleProviderLoginCancel(c *gin.Context) {
	flowID := c.Param("flow_id")
	providerLoginFlows.Lock()
	flow, ok := providerLoginFlows.values[flowID]
	if ok {
		delete(providerLoginFlows.values, flowID)
	}
	providerLoginFlows.Unlock()
	if !ok || time.Now().After(flow.ExpiresAt) {
		c.JSON(http.StatusNotFound, gin.H{"status": "expired", "error": "login flow not found or expired"})
		return
	}
	if flow.Kind == "device" {
		activeAuthFlowMu.Lock()
		if activeAuthFlow != nil && (flow.ProviderID == "" || activeAuthFlow.ProviderID == flow.ProviderID) {
			if activeAuthFlow.cancelFn != nil {
				activeAuthFlow.cancelFn()
			}
			activeAuthFlow = nil
		}
		activeAuthFlowMu.Unlock()
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled", "provider_id": flow.ProviderID, "is_new": flow.IsNew})
}
