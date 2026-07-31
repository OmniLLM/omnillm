package routes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/registry"

	openaipkg "omnillm/internal/providers/openai"
	"omnillm/internal/providers/types"
)

// errNotOpenAIProvider is returned when a re-auth targets a provider instance
// that is not a ChatGPT-OAuth provider.
var errNotOpenAIProvider = errors.New("openai: provider instance is not a ChatGPT OAuth provider")

// ─── Pending OAuth flows ──────────────────────────────────────────────────────
//
// Unlike the antigravity flow — where the browser redirects back to the
// OmniLLM server and the frontend polls for completion — the ChatGPT OAuth
// client whitelists only http://localhost:1455/auth/callback. The redirect
// therefore lands on a short-lived listener owned by the provider package, and
// this route waits on it in a background goroutine while the frontend polls
// oauth-status.

type openaiOAuthResult struct {
	Done       bool
	Error      string
	ProviderID string
	IsNew      bool
	Expiry     time.Time
}

var (
	openaiOAuthMu      sync.Mutex
	openaiOAuthResults = map[string]*openaiOAuthResult{} // keyed by provider_id
)

func setOpenAIOAuthResult(res *openaiOAuthResult) {
	res.Expiry = time.Now().Add(5 * time.Minute)
	openaiOAuthMu.Lock()
	openaiOAuthResults[res.ProviderID] = res
	openaiOAuthMu.Unlock()
}

// ─── Route: POST /providers/openai/start-oauth ───────────────────────────────
// Body: { "provider_id": "…" }  (empty = create new)
// Returns: { "auth_url": "…", "provider_id": "…" }

func handleOpenAIStartOAuth(c *gin.Context) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	// An empty body is valid (create-new), so bind errors are non-fatal.
	_ = c.ShouldBindJSON(&req)
	req.ProviderID = strings.TrimSpace(req.ProviderID)

	isNew := req.ProviderID == ""
	providerID := req.ProviderID
	reg := registry.GetProviderRegistry()
	if isNew {
		providerID = reg.NextInstanceID(string(types.ProviderOpenAI))
	}

	pkce, err := openaipkg.GeneratePKCE()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	state, err := openaipkg.RandomState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	authURL := openaipkg.BuildAuthURL(pkce.Challenge, state)

	// Wait for the browser redirect out of band so this request can return the
	// URL immediately; the frontend polls oauth-status for the outcome.
	go completeOpenAIOAuth(providerID, isNew, pkce.Verifier, state)

	log.Info().Str("provider", providerID).Bool("is_new", isNew).Msg("OpenAI: ChatGPT OAuth flow started")

	c.JSON(http.StatusOK, gin.H{
		"auth_url":    authURL,
		"provider_id": providerID,
		"state":       state,
	})
}

// completeOpenAIOAuth blocks on the local callback, exchanges the code, and
// registers or refreshes the provider.
func completeOpenAIOAuth(providerID string, isNew bool, verifier, state string) {
	fail := func(err error) {
		log.Error().Err(err).Str("provider", providerID).Msg("OpenAI: OAuth failed")
		setOpenAIOAuthResult(&openaiOAuthResult{Done: true, ProviderID: providerID, Error: err.Error()})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	cb, err := openaipkg.ListenForCallback(ctx, state, 5*time.Minute)
	if err != nil {
		fail(err)
		return
	}

	tokens, err := openaipkg.ExchangeCode(cb.Code, verifier)
	if err != nil {
		fail(err)
		return
	}

	reg := registry.GetProviderRegistry()

	if isNew {
		prov := openaipkg.NewProvider(providerID, "")
		if err := prov.ApplyTokens(tokens); err != nil {
			fail(err)
			return
		}
		if err := reg.Register(prov, true); err != nil {
			fail(err)
			return
		}
	} else {
		prov, err := reg.GetProvider(providerID)
		if err != nil {
			fail(err)
			return
		}
		op, ok := prov.(*openaipkg.Provider)
		if !ok {
			fail(errNotOpenAIProvider)
			return
		}
		if err := op.ApplyTokens(tokens); err != nil {
			fail(err)
			return
		}
	}

	log.Info().Str("provider", providerID).Msg("OpenAI: ChatGPT OAuth completed successfully")
	setOpenAIOAuthResult(&openaiOAuthResult{Done: true, ProviderID: providerID, IsNew: isNew})
}

// ─── Route: GET /providers/openai/oauth-status ───────────────────────────────
// Public so the frontend can poll before admin auth is established.

// HandleOpenAIOAuthStatusPublic is the exported handler registered by server.go
// on the public router group.
var HandleOpenAIOAuthStatusPublic = handleOpenAIOAuthStatus

func handleOpenAIOAuthStatus(c *gin.Context) {
	providerID := strings.TrimSpace(c.Query("provider_id"))
	if providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id is required"})
		return
	}

	openaiOAuthMu.Lock()
	result, ok := openaiOAuthResults[providerID]
	if ok && result.Done {
		delete(openaiOAuthResults, providerID) // consume once
	}
	openaiOAuthMu.Unlock()

	if !ok {
		c.JSON(http.StatusOK, gin.H{"done": false})
		return
	}
	if result.Error != "" {
		c.JSON(http.StatusOK, gin.H{"done": true, "error": result.Error})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"done":        true,
		"provider_id": result.ProviderID,
		"is_new":      result.IsNew,
	})
}
