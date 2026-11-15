package routes

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	alibabapkg "omnillm/internal/providers/alibaba"
	antigravitypkg "omnillm/internal/providers/antigravity"
	azurepkg "omnillm/internal/providers/azure"
	codexpkg "omnillm/internal/providers/codex"
	"omnillm/internal/providers/copilot"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"
	modelscopepkg "omnillm/internal/providers/modelscope"
	openaipkg "omnillm/internal/providers/openai"
	openaicompatprovider "omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	typesafepkg "omnillm/internal/providers/typesafe"
	"omnillm/internal/registry"
)

var providerIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type identityReloadableProvider interface {
	types.Provider
	LoadFromDB() error
	SetInstanceID(string)
}

// prepareProviderIdentity creates an unpublished replacement before the DB
// transaction. OAuth providers clone in-memory credentials without refreshing.
func prepareProviderIdentity(record *database.ProviderInstanceRecord, current types.Provider, newID string) (types.Provider, error) {
	switch provider := current.(type) {
	case *openaipkg.Provider:
		clone := provider.CloneForIdentity(newID)
		clone.SetName(record.Name)
		return clone, nil
	case *copilot.GitHubCopilotProvider:
		clone := provider.CloneForIdentity(newID)
		clone.SetName(record.Name)
		return clone, nil
	case *codexpkg.CodexProvider:
		clone := provider.CloneForIdentity(newID)
		clone.SetName(record.Name)
		return clone, nil
	}

	var replacement identityReloadableProvider
	switch record.ProviderID {
	case "alibaba":
		replacement = alibabapkg.NewProvider(record.InstanceID, record.Name)
	case "alibaba-modelscope":
		replacement = modelscopepkg.NewProvider(record.InstanceID, record.Name)
	case "antigravity":
		replacement = antigravitypkg.NewProvider(record.InstanceID, record.Name)
	case "azure-openai":
		replacement = azurepkg.NewProvider(record.InstanceID, record.Name)
	case "google":
		replacement = googlepkg.NewProvider(record.InstanceID, record.Name)
	case "kimi":
		replacement = kimipkg.NewProvider(record.InstanceID, record.Name)
	case "openai-compatible":
		replacement = openaicompatprovider.NewProvider(record.InstanceID, record.Name)
	case "typesafe":
		replacement = typesafepkg.NewProvider(record.InstanceID, record.Name)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", record.ProviderID)
	}
	if err := replacement.LoadFromDB(); err != nil {
		return nil, fmt.Errorf("prepare replacement provider: %w", err)
	}
	replacement.SetName(record.Name)
	replacement.SetInstanceID(newID)
	return replacement, nil
}

func handleChangeProviderIdentity(c *gin.Context) {
	oldID := c.Param("id")
	var req struct {
		NewID *string `json:"new_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.NewID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_id is required"})
		return
	}
	reg := registry.GetProviderRegistry()
	if *req.NewID == oldID {
		if !reg.IsRegistered(oldID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "id": oldID})
		return
	}
	newID := strings.TrimSpace(*req.NewID)
	if !providerIdentityPattern.MatchString(newID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID must be 1–64 letters, digits, periods, underscores, or hyphens and start with a letter or digit"})
		return
	}
	if newID == oldID {
		if !reg.IsRegistered(oldID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "id": oldID})
		return
	}

	lease, err := reg.BeginIdentityChange(oldID)
	if err != nil {
		if errors.Is(err, registry.ErrProviderBusy) {
			c.JSON(http.StatusConflict, gin.H{"error": "Provider is busy. Please retry after active requests or authentication finish."})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		}
		return
	}
	defer lease.Release()
	store := database.NewProviderInstanceStore()
	record, err := store.Get(oldID)
	if err != nil || record == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read provider state"})
		return
	}
	current, err := reg.GetProvider(oldID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read provider state"})
		return
	}
	if record.ProviderID != current.GetID() {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Provider state is inconsistent"})
		return
	}
	replacement, err := prepareProviderIdentity(record, current, newID)
	if err != nil {
		log.Error().Err(err).Str("provider", oldID).Msg("Failed to prepare provider identity change")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to prepare provider. Check its credentials and configuration."})
		return
	}

	providerMetadataMu.Lock()
	defer providerMetadataMu.Unlock()
	records, err := store.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check provider IDs"})
		return
	}
	for _, other := range records {
		if other.InstanceID == oldID {
			continue
		}
		if other.InstanceID == newID || (other.Subtitle != "" && strings.EqualFold(other.Subtitle, newID)) || (other.Name != "" && strings.EqualFold(other.Name, newID)) {
			c.JSON(http.StatusConflict, gin.H{"error": "ID conflicts with another provider ID, alias, or name"})
			return
		}
	}
	if err := lease.Commit(c.Request.Context(), replacement); err != nil {
		if errors.Is(err, registry.ErrProviderIdentityConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "ID conflicts with another provider ID, alias, or name"})
			return
		}
		if target, lookupErr := store.Get(newID); lookupErr == nil && target != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "ID conflicts with another provider ID, alias, or name"})
			return
		}
		log.Error().Err(err).Str("provider", oldID).Msg("Failed to change provider identity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to change provider ID; no changes were applied"})
		return
	}
	lease.Release()
	c.JSON(http.StatusOK, gin.H{"success": true, "id": newID})
}
