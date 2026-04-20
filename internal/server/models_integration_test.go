package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
)

func TestModelsEndpointIncludesEnabledVirtualModels(t *testing.T) {
	registerStubModelsProvider(t, []providertypes.Model{
		{
			ID:       "provider-model",
			Name:     "Provider Model",
			Provider: "stub-provider",
			MaxTokens: 8192,
		},
	}, true)

	store := database.NewVirtualModelStore()
	if err := store.Create(&database.VirtualModelRecord{
		VirtualModelID: "vmodel-1",
		Name:           "Virtual Model",
		Description:    "Aggregated route",
		APIShape:       "responses",
		LbStrategy:     database.LbStrategyPriority,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/v1/models")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	foundProvider := false
	foundVirtual := false
	for _, model := range payload.Data {
		if model["id"] == "provider-model" {
			foundProvider = true
		}
		if model["id"] == "vmodel-1" {
			foundVirtual = true
			if model["owned_by"] != "virtual" {
				t.Fatalf("unexpected virtual model owner: %#v", model)
			}
			if model["api_shape"] != "responses" {
				t.Fatalf("unexpected virtual model api shape: %#v", model)
			}
		}
	}

	if !foundProvider {
		t.Fatal("expected provider model in response")
	}
	if !foundVirtual {
		t.Fatal("expected virtual model in response")
	}
}
