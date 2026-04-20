package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
)

func SetupVirtualModelRoutes(router *gin.RouterGroup) {
	router.GET("/vmodels", handleListVirtualModels)
	router.POST("/vmodels", handleCreateVirtualModel)
	router.GET("/vmodels/:id", handleGetVirtualModel)
	router.PUT("/vmodels/:id", handleUpdateVirtualModel)
	router.DELETE("/vmodels/:id", handleDeleteVirtualModel)
}

// ─── request / response types ─────────────────────────────────────────────────

type upstreamInput struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"  binding:"required"`
	Weight     int    `json:"weight"`
	Priority   int    `json:"priority"`
}

type vmodelPayload struct {
	VirtualModelID string          `json:"virtual_model_id" binding:"required"`
	Name           string          `json:"name"             binding:"required"`
	Description    string          `json:"description"`
	APIShape       string          `json:"api_shape"`
	LbStrategy     string          `json:"lb_strategy"      binding:"required"`
	Enabled        *bool           `json:"enabled"`
	Upstreams      []upstreamInput `json:"upstreams"`
}

type vmodelResponse struct {
	database.VirtualModelRecord
	Upstreams []database.VirtualModelUpstreamRecord `json:"upstreams"`
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func vmWithUpstreams(vm *database.VirtualModelRecord) (*vmodelResponse, error) {
	us := database.NewVirtualModelUpstreamStore()
	upstreams, err := us.GetForVModel(vm.VirtualModelID)
	if err != nil {
		return nil, err
	}
	if upstreams == nil {
		upstreams = []database.VirtualModelUpstreamRecord{}
	}
	return &vmodelResponse{VirtualModelRecord: *vm, Upstreams: upstreams}, nil
}

func saveUpstreams(virtualModelID string, inputs []upstreamInput) error {
	us := database.NewVirtualModelUpstreamStore()
	records := make([]database.VirtualModelUpstreamRecord, 0, len(inputs))
	for _, u := range inputs {
		w := u.Weight
		if w < 1 {
			w = 1
		}
		records = append(records, database.VirtualModelUpstreamRecord{
			VirtualModelID: virtualModelID,
			ProviderID:     u.ProviderID,
			ModelID:        u.ModelID,
			Weight:         w,
			Priority:       u.Priority,
		})
	}
	return us.SetForVModel(virtualModelID, records)
}

// ─── handlers ─────────────────────────────────────────────────────────────────

func handleListVirtualModels(c *gin.Context) {
	store := database.NewVirtualModelStore()
	vmodels, err := store.GetAll()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list virtual models")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list virtual models"})
		return
	}

	result := make([]vmodelResponse, 0, len(vmodels))
	for i := range vmodels {
		resp, err := vmWithUpstreams(&vmodels[i])
		if err != nil {
			log.Warn().Err(err).Str("id", vmodels[i].VirtualModelID).Msg("Failed to load upstreams")
			continue
		}
		result = append(result, *resp)
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func handleGetVirtualModel(c *gin.Context) {
	id := c.Param("id")
	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get virtual model"})
		return
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}
	resp, err := vmWithUpstreams(vm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load upstreams"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleCreateVirtualModel(c *gin.Context) {
	var payload vmodelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiShape := payload.APIShape
	if apiShape == "" {
		apiShape = "openai"
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	store := database.NewVirtualModelStore()
	// Check for duplicate
	existing, _ := store.Get(payload.VirtualModelID)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Virtual model ID already exists"})
		return
	}

	record := &database.VirtualModelRecord{
		VirtualModelID: payload.VirtualModelID,
		Name:           payload.Name,
		Description:    payload.Description,
		APIShape:       apiShape,
		LbStrategy:     database.LbStrategy(payload.LbStrategy),
		Enabled:        enabled,
	}
	if err := store.Create(record); err != nil {
		log.Error().Err(err).Msg("Failed to create virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create virtual model"})
		return
	}

	if err := saveUpstreams(payload.VirtualModelID, payload.Upstreams); err != nil {
		log.Error().Err(err).Msg("Failed to save upstreams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save upstreams"})
		return
	}

	vm, _ := store.Get(payload.VirtualModelID)
	resp, _ := vmWithUpstreams(vm)
	c.JSON(http.StatusCreated, resp)
}

func handleUpdateVirtualModel(c *gin.Context) {
	id := c.Param("id")
	var payload vmodelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil || vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}

	vm.Name = payload.Name
	vm.Description = payload.Description
	if payload.APIShape != "" {
		vm.APIShape = payload.APIShape
	}
	vm.LbStrategy = database.LbStrategy(payload.LbStrategy)
	if payload.Enabled != nil {
		vm.Enabled = *payload.Enabled
	}

	if err := store.Update(vm); err != nil {
		log.Error().Err(err).Msg("Failed to update virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update virtual model"})
		return
	}

	if err := saveUpstreams(id, payload.Upstreams); err != nil {
		log.Error().Err(err).Msg("Failed to save upstreams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save upstreams"})
		return
	}

	updated, _ := store.Get(id)
	resp, _ := vmWithUpstreams(updated)
	c.JSON(http.StatusOK, resp)
}

func handleDeleteVirtualModel(c *gin.Context) {
	id := c.Param("id")
	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil || vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}
	if err := store.Delete(id); err != nil {
		log.Error().Err(err).Msg("Failed to delete virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete virtual model"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
