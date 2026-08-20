package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
	"omnillm/internal/registry"
)

func TestResolveProviderReferenceParam(t *testing.T) {
	store := database.NewProviderInstanceStore()
	record := &database.ProviderInstanceRecord{InstanceID: "reference-route-id", ProviderID: "stub", Name: "Reference Route Name", Subtitle: "route-alias"}
	if err := store.Save(record); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(record.InstanceID) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/providers/:id", resolveProviderReferenceParam, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})
	for _, reference := range []string{record.InstanceID, "ROUTE-ALIAS", "reference route name"} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/providers/"+strings.ReplaceAll(reference, " ", "%20"), nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), record.InstanceID) {
			t.Fatalf("reference %q: status=%d body=%s", reference, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRenameProviderAliasCompatibility(t *testing.T) {
	store := database.NewProviderInstanceStore()
	record := &database.ProviderInstanceRecord{InstanceID: "reference-rename-id", ProviderID: "stub", Name: "Reference Rename", Subtitle: "old"}
	if err := store.Save(record); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(record.InstanceID) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PATCH("/providers/:id/name", resolveProviderReferenceParam, handleRenameProvider)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/providers/old/name", strings.NewReader(`{"alias":"new-alias"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"alias":"new-alias"`) || !strings.Contains(recorder.Body.String(), `"subtitle":"new-alias"`) {
		t.Fatalf("rename status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/providers/new-alias/name", strings.NewReader(`{"alias":"one","subtitle":"two"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("conflicting fields status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestResolveProviderReferenceParamRejectsAmbiguousAlias(t *testing.T) {
	store := database.NewProviderInstanceStore()
	for _, id := range []string{"ambiguous-route-b", "ambiguous-route-a"} {
		if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "stub", Name: id, Subtitle: "ambiguous-route"}); err != nil {
			t.Fatalf("save provider: %v", err)
		}
		defer store.Delete(id)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/providers/:id", resolveProviderReferenceParam, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/ambiguous-route", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"matching_ids":["ambiguous-route-a","ambiguous-route-b"]`) {
		t.Fatalf("ambiguous status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProviderLoginCompletesImmediateNewProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/providers/login", handleProviderLogin)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/providers/login", strings.NewReader(`{"type":"google","apiKey":"test-key"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"complete"`) || !strings.Contains(recorder.Body.String(), `"is_new":true`) {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.ProviderID == "" {
		t.Fatalf("decode login result: %v, provider=%q", err, result.ProviderID)
	}
	t.Cleanup(func() {
		providerRegistry := registry.GetProviderRegistry()
		_ = providerRegistry.Remove(result.ProviderID)
		providerRegistry.WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(result.ProviderID)
	})
}

func TestProviderLoginCancelConsumesFlow(t *testing.T) {
	flowID, err := newProviderLoginFlow(providerLoginFlow{Kind: "openai", ProviderID: "cancel-provider", IsNew: true})
	if err != nil {
		t.Fatalf("new flow: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/providers/login/:flow_id", handleProviderLoginCancel)
	router.GET("/providers/login/:flow_id", handleProviderLoginStatus)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/providers/login/"+flowID, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"canceled"`) {
		t.Fatalf("cancel status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/login/"+flowID, nil))
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"status":"expired"`) {
		t.Fatalf("consumed status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
