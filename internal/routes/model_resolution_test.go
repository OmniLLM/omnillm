package routes

import (
	"testing"

	"omnillm/internal/database"
)

func TestResolveRequestedModelsProviderQualification(t *testing.T) {
	instanceID := "routing-instance-namespaced"
	subtitle := "RoutingSubtitleNamespaced"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{
		InstanceID: instanceID,
		ProviderID: "stub-provider",
		Name:       instanceID,
		Subtitle:   subtitle,
		Activated:  true,
	}); err != nil {
		t.Fatalf("save provider instance: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(instanceID) })

	tests := []struct {
		name         string
		model        string
		wantAttempts int
		wantFallback string
		wantProvider string
	}{
		{
			name:         "instance qualifier fallback",
			model:        instanceID + "/kimi/kimi-k3",
			wantAttempts: 2,
			wantFallback: "kimi/kimi-k3",
			wantProvider: instanceID,
		},
		{
			name:         "subtitle qualifier fallback",
			model:        subtitle + "/kimi/kimi-k3",
			wantAttempts: 2,
			wantFallback: "kimi/kimi-k3",
			wantProvider: instanceID,
		},
		{
			name:         "case insensitive subtitle fallback",
			model:        "routingsubtitlenamespaced/kimi/kimi-k3",
			wantAttempts: 2,
			wantFallback: "kimi/kimi-k3",
			wantProvider: instanceID,
		},
		{
			name:         "native namespace only",
			model:        "kimi/kimi-k3",
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := resolveRequestedModels("test-request", tt.model)
			if len(attempts) != tt.wantAttempts {
				t.Fatalf("attempt count = %d, want %d", len(attempts), tt.wantAttempts)
			}
			if got := attempts[0].RequestedModel; got != tt.model {
				t.Errorf("native RequestedModel = %q, want %q", got, tt.model)
			}
			if attempts[0].ProviderID != "" {
				t.Errorf("native ProviderID = %q, want empty", attempts[0].ProviderID)
			}
			if tt.wantAttempts == 1 {
				return
			}
			fallback := attempts[1]
			if fallback.RequestedModel != tt.wantFallback {
				t.Errorf("fallback RequestedModel = %q, want %q", fallback.RequestedModel, tt.wantFallback)
			}
			if fallback.ProviderID != tt.wantProvider {
				t.Errorf("fallback ProviderID = %q, want %q", fallback.ProviderID, tt.wantProvider)
			}
		})
	}
}
