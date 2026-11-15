package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"omnillm/internal/database"
	alibabapkg "omnillm/internal/providers/alibaba"
	kimipkg "omnillm/internal/providers/kimi"
	"omnillm/internal/registry"
)

func TestNewProviderIDsAreDistinctUUIDs(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	router := newAdminTestRouter()
	ids := map[string]bool{}

	for range 2 {
		response := performJSONRequest(
			t,
			router,
			http.MethodPost,
			"/api/admin/providers/auth-and-create/alibaba",
			`{"method":"api-key","apiKey":"private-key-123456","region":"global"}`,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("create provider: %d %s", response.Code, response.Body.String())
		}
		var result struct {
			Provider struct {
				ID string `json:"id"`
			} `json:"provider"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		id, err := uuid.Parse(result.Provider.ID)
		if err != nil || id.Version() != 4 || result.Provider.ID != id.String() {
			t.Fatalf("provider ID %q is not a canonical UUID v4", result.Provider.ID)
		}
		if ids[result.Provider.ID] {
			t.Fatalf("duplicate provider ID %q", result.Provider.ID)
		}
		ids[result.Provider.ID] = true
	}
}

func TestImmediateProviderCreationUsesUUIDAcrossTypes(t *testing.T) {
	for _, row := range []struct {
		name, body string
	}{
		{"alibaba", `{"apiKey":"test-key"}`},
		{"alibaba-modelscope", `{"apiKey":"test-key"}`},
		{"azure-openai", `{"apiKey":"test-key"}`},
		{"google", `{"apiKey":"test-key"}`},
		{"kimi", `{"apiKey":"test-key"}`},
		{"codex", `{"apiKey":"test-key"}`},
		{"typesafe", `{"apiKey":"test-key"}`},
		{"openai-compatible", `{"apiKey":"test-key","endpoint":"https://api.example.test/v1"}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			if row.name == "typesafe" {
				stubDefaultTransport(t, func(request *http.Request) (*http.Response, error) {
					if request.URL.Host != "api.typesafe.ai" || request.URL.Path != "/v1/models" {
						return nil, context.Canceled
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"name":"jev-latest"}]}`)), Request: request}, nil
				})
			}
			resetAdminTestState(t)
			t.Cleanup(func() { resetAdminTestState(t) })
			response := performJSONRequest(t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/auth-and-create/"+row.name, row.body)
			if response.Code != http.StatusOK {
				t.Fatalf("create %s: %d %s", row.name, response.Code, response.Body.String())
			}
			result := decodeJSONBody[struct {
				Provider struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"provider"`
			}](t, response)
			requireProviderUUID(t, result.Provider.ID)
			if result.Provider.Type != row.name {
				t.Fatalf("provider type = %q, want %q", result.Provider.Type, row.name)
			}
		})
	}
}

func TestImmediateReauthenticationKeepsProviderIdentity(t *testing.T) {
	for _, row := range []struct {
		name, body string
	}{
		{"alibaba", `{"apiKey":"test-key"}`},
		{"alibaba-modelscope", `{"apiKey":"test-key"}`},
		{"azure-openai", `{"apiKey":"test-key"}`},
		{"google", `{"apiKey":"test-key"}`},
		{"kimi", `{"apiKey":"test-key"}`},
		{"codex", `{"apiKey":"test-key"}`},
		{"typesafe", `{"apiKey":"test-key"}`},
		{"openai-compatible", `{"apiKey":"test-key","endpoint":"https://api.example.test/v1"}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			if row.name == "typesafe" {
				stubDefaultTransport(t, func(request *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"name":"jev-latest"}]}`)), Request: request}, nil
				})
			}
			resetAdminTestState(t)
			t.Cleanup(func() { resetAdminTestState(t) })
			router := newAdminTestRouter()
			created := performJSONRequest(t, router, http.MethodPost, "/api/admin/providers/auth-and-create/"+row.name, row.body)
			if created.Code != http.StatusOK {
				t.Fatalf("create: %d %s", created.Code, created.Body.String())
			}
			result := decodeJSONBody[struct {
				Provider struct {
					ID string `json:"id"`
				} `json:"provider"`
			}](t, created)
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(row.body), &input); err != nil {
				t.Fatalf("decode input: %v", err)
			}
			input["provider"] = result.Provider.ID
			input["apiKey"] = "test-new-key"
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("encode input: %v", err)
			}
			response := performJSONRequest(t, router, http.MethodPost, "/api/admin/providers/login", string(body))
			if response.Code != http.StatusOK {
				t.Fatalf("reauthenticate: %d %s", response.Code, response.Body.String())
			}
			completed := decodeJSONBody[struct {
				ProviderID string `json:"provider_id"`
				IsNew      bool   `json:"is_new"`
			}](t, response)
			if completed.ProviderID != result.Provider.ID || completed.IsNew {
				t.Fatalf("reauthentication identity = %#v, want %q", completed, result.Provider.ID)
			}
			token, err := database.NewTokenStore().Get(result.Provider.ID)
			if err != nil || token == nil || !strings.Contains(token.TokenData, "test-new-key") {
				t.Fatalf("reauthentication credentials were not persisted: %#v, %v", token, err)
			}
			reg := registry.GetProviderRegistry()
			reg.WaitForPendingSaves()
			newID := row.name + "-custom"
			rename := performJSONRequest(t, router, http.MethodPatch, "/api/admin/providers/"+result.Provider.ID+"/identity", `{"new_id":"`+newID+`"}`)
			if rename.Code != http.StatusOK {
				t.Fatalf("rename: %d %s", rename.Code, rename.Body.String())
			}
			provider, err := reg.GetProvider(newID)
			if err != nil || provider.GetToken() != "test-new-key" {
				t.Fatalf("renamed provider did not retain credentials: %v", err)
			}
			token, err = database.NewTokenStore().Get(newID)
			if err != nil || token == nil || !strings.Contains(token.TokenData, "test-new-key") {
				t.Fatalf("renamed durable credentials: %#v, %v", token, err)
			}
		})
	}
}

func TestModelScopeLoginCreatesUUIDProvider(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPost, "/api/admin/providers/login",
		`{"type":"alibaba-modelscope","apiKey":"test-modelscope-key"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("login: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
	}](t, response)
	requireProviderUUID(t, result.ProviderID)
}

func TestKimiOAuthTokenKeepsExistingProviderID(t *testing.T) {
	const id = "legacy-kimi-provider"
	store := database.NewProviderInstanceStore()
	if _, err := store.Create(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "kimi", Name: "Kimi"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(id) })
	gotID, _, _, err := kimipkg.SaveOAuthToken(id, &kimipkg.TokenData{
		AuthType: "oauth", AccessToken: "not-a-jwt-token", ResourceURL: "https://api.moonshot.cn",
	})
	if err != nil || gotID != id {
		t.Fatalf("save OAuth token: id=%q err=%v", gotID, err)
	}
	token, err := database.NewTokenStore().Get(id)
	if err != nil || token == nil || !strings.Contains(token.TokenData, "not-a-jwt-token") {
		t.Fatalf("saved OAuth token: %#v, %v", token, err)
	}
}

func TestProviderIdentityChangePreservesOwnedState(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	const oldID = "old-provider"
	const newID = "new-provider"
	const virtualID = "identity-test-vm"

	provider := alibabapkg.NewProvider(oldID, "Original provider")
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{
		InstanceID: oldID, ProviderID: "alibaba", Name: "Original provider",
		Subtitle: "original-alias", Priority: 7, Activated: true,
	}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := database.NewTokenStore().Save(oldID, map[string]string{"access_token": "secret"}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if err := database.NewModelStateStore().SetEnabled(oldID, "old-provider/native", false); err != nil {
		t.Fatalf("save model state: %v", err)
	}
	if err := database.NewVirtualModelStore().Create(&database.VirtualModelRecord{
		VirtualModelID: virtualID, Name: "Identity test", APIShape: "openai",
		LbStrategy: database.LbStrategyPriority, Enabled: true,
	}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}
	t.Cleanup(func() { _ = database.NewVirtualModelStore().Delete(virtualID) })
	if err := database.NewVirtualModelUpstreamStore().SetForVModel(virtualID, []database.VirtualModelUpstreamRecord{{
		ProviderID: oldID, ModelID: "old-provider/native", Weight: 1,
	}}); err != nil {
		t.Fatalf("save upstream: %v", err)
	}
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.SetActive(oldID); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	reg.WaitForPendingSaves()

	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/"+oldID+"/identity", `{"new_id":"new-provider"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("rename provider: %d %s", response.Code, response.Body.String())
	}
	reg.WaitForPendingSaves()
	if reg.IsRegistered(oldID) || !reg.IsActiveProvider(newID) {
		t.Fatal("registry did not transfer active provider identity")
	}
	active, err := reg.GetActive()
	if err != nil || active.GetInstanceID() != newID {
		t.Fatalf("primary provider after rename: %v, %v", active, err)
	}
	if active.GetToken() != "secret" {
		t.Fatal("runtime provider credentials were not restored")
	}
	oldRecord, err := store.Get(oldID)
	if err != nil || oldRecord != nil {
		t.Fatalf("old provider still exists: %#v, %v", oldRecord, err)
	}
	if _, err := resolveProviderReference(oldID); err == nil {
		t.Fatal("old ID remained an automatic provider redirect")
	}
	if resolved, err := resolveProviderReference("original-alias"); err != nil || resolved != newID {
		t.Fatalf("alias after rename = %q, %v", resolved, err)
	}
	record, err := store.Get(newID)
	if err != nil || record == nil || record.Priority != 7 || record.Subtitle != "original-alias" || !record.Activated {
		t.Fatalf("new provider metadata: %#v, %v", record, err)
	}
	token, err := database.NewTokenStore().Get(newID)
	if err != nil || token == nil || !strings.Contains(token.TokenData, "secret") {
		t.Fatalf("new provider token: %#v, %v", token, err)
	}
	state, err := database.NewModelStateStore().Get(newID, "old-provider/native")
	if err != nil || state == nil || state.Enabled {
		t.Fatalf("new provider model state: %#v, %v", state, err)
	}
	upstreams, err := database.NewVirtualModelUpstreamStore().GetForVModel(virtualID)
	if err != nil || len(upstreams) != 1 || upstreams[0].ProviderID != newID || upstreams[0].ModelID != "old-provider/native" {
		t.Fatalf("virtual-model upstreams: %#v, %v", upstreams, err)
	}
}

func TestProviderIdentityChangeRejectsInvalidOrConflictingTargets(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	reg := registry.GetProviderRegistry()
	store := database.NewProviderInstanceStore()
	for _, record := range []*database.ProviderInstanceRecord{
		{InstanceID: "identity-source", ProviderID: "alibaba", Name: "Source"},
		{InstanceID: "identity-other", ProviderID: "alibaba", Name: "Other name", Subtitle: "other-alias"},
	} {
		if err := store.Save(record); err != nil {
			t.Fatalf("save provider: %v", err)
		}
		if err := reg.Register(alibabapkg.NewProvider(record.InstanceID, record.Name), false); err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}

	router := newAdminTestRouter()
	for _, target := range []string{"", "-bad", "bad/id", strings.Repeat("a", 65), "identity-other", "other-alias", "Other name"} {
		t.Run(target, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"new_id": target})
			if err != nil {
				t.Fatalf("marshal target: %v", err)
			}
			response := performJSONRequest(
				t, router, http.MethodPatch,
				"/api/admin/providers/identity-source/identity", string(body),
			)
			if response.Code != http.StatusBadRequest && response.Code != http.StatusConflict {
				t.Fatalf("target %q: expected client error, got %d: %s", target, response.Code, response.Body.String())
			}
			if !reg.IsRegistered("identity-source") || !reg.IsRegistered("identity-other") {
				t.Fatal("rejected rename changed registry")
			}
		})
	}
}

func TestProviderIdentityChangeRejectsRegisteredTargetWithoutDatabaseRow(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: "memory-source", ProviderID: "alibaba", Name: "Source"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	reg := registry.GetProviderRegistry()
	for _, id := range []string{"memory-source", "memory-target"} {
		if err := reg.Register(alibabapkg.NewProvider(id, id), false); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/memory-source/identity", `{"new_id":"memory-target"}`,
	)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected target conflict, got %d: %s", response.Code, response.Body.String())
	}
	if !reg.IsRegistered("memory-source") || !reg.IsRegistered("memory-target") {
		t.Fatal("target conflict changed the registry")
	}
}

func TestProviderIdentityChangeAcceptsUnchangedLegacyID(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })
	const id = "legacy:id"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: id, ProviderID: "alibaba", Name: "Legacy"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := registry.GetProviderRegistry().Register(alibabapkg.NewProvider(id, "Legacy"), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch, "/api/admin/providers/legacy:id/identity", `{"new_id":"legacy:id"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("unchanged legacy ID: %d %s", response.Code, response.Body.String())
	}
	result := decodeJSONBody[struct {
		ID string `json:"id"`
	}](t, response)
	if result.ID != id {
		t.Fatalf("result ID = %q, want %q", result.ID, id)
	}
}

func TestProviderIdentityChangePreservesVirtualModelExecution(t *testing.T) {
	resetAdminTestState(t)
	const oldID = "old-routing-provider"
	const newID = "new-routing-provider"
	const virtualID = "identity-routing-vm"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "alibaba", Name: "Alibaba"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := registry.GetProviderRegistry().Register(alibabapkg.NewProvider(oldID, "Alibaba"), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := database.NewVirtualModelStore().Create(&database.VirtualModelRecord{
		VirtualModelID: virtualID, Name: "Identity routing", APIShape: "openai",
		LbStrategy: database.LbStrategyPriority, Enabled: true,
	}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}
	t.Cleanup(func() {
		_ = database.NewVirtualModelStore().Delete(virtualID)
		resetAdminTestState(t)
	})
	if err := saveUpstreams(virtualID, []upstreamInput{{ProviderID: oldID, ModelID: "plain-model", Weight: 1}}); err != nil {
		t.Fatalf("save upstream: %v", err)
	}
	before, err := resolveRequestedModels("test", virtualID)
	if err != nil || len(before) != 1 || before[0].RequestedModel != "plain-model" || before[0].ProviderID != oldID {
		t.Fatalf("routing before rename: %#v, %v", before, err)
	}
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/"+oldID+"/identity", `{"new_id":"new-routing-provider"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("rename provider: %d %s", response.Code, response.Body.String())
	}
	after, err := resolveRequestedModels("test", virtualID)
	if err != nil || len(after) != 1 || after[0].RequestedModel != "plain-model" || after[0].ProviderID != newID {
		t.Fatalf("routing after rename: %#v, %v", after, err)
	}
	response = performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/"+newID+"/identity", `{"new_id":"new-routing-provider-2"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("rename provider again: %d %s", response.Code, response.Body.String())
	}
	// Drop in-memory snapshots to exercise the same durable reads as a restart.
	database.GetModelResolutionCache().InvalidateInstances()
	database.GetModelResolutionCache().InvalidateVMs()
	after, err = resolveRequestedModels("test", virtualID)
	if err != nil || len(after) != 1 || after[0].RequestedModel != "plain-model" || after[0].ProviderID != "new-routing-provider-2" {
		t.Fatalf("routing after second rename and reload: %#v, %v", after, err)
	}
	upstreams, err := database.NewVirtualModelUpstreamStore().GetForVModel(virtualID)
	if err != nil || len(upstreams) != 1 || upstreams[0].ModelID != oldID+"/plain-model" {
		t.Fatalf("stored upstream model changed: %#v, %v", upstreams, err)
	}
	hintKey := fmt.Sprintf("internal.provider-identity.upstream.%d", upstreams[0].ID)
	if err := database.NewConfigStore().Set(hintKey, `{"provider_id":"wrong","model_id":"wrong","prefix":"wrong"}`); err != nil {
		t.Fatalf("corrupt hint: %v", err)
	}
	database.GetModelResolutionCache().InvalidateVMs()
	if _, err := resolveRequestedModels("test", virtualID); err == nil {
		t.Fatal("mismatched upstream identity hint must fail closed")
	}
}

func TestProviderIdentityChangePreservesNamespacedNativeModel(t *testing.T) {
	resetAdminTestState(t)
	const oldID = "namespace-provider-old"
	const newID = "namespace-provider-new"
	const virtualID = "namespace-identity-vm"
	store := database.NewProviderInstanceStore()
	if err := store.Save(&database.ProviderInstanceRecord{InstanceID: oldID, ProviderID: "alibaba", Name: "Alibaba"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := registry.GetProviderRegistry().Register(alibabapkg.NewProvider(oldID, "Alibaba"), false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := database.NewVirtualModelStore().Create(&database.VirtualModelRecord{
		VirtualModelID: virtualID, Name: "Namespace routing", APIShape: "openai",
		LbStrategy: database.LbStrategyPriority, Enabled: true,
	}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}
	t.Cleanup(func() {
		_ = database.NewVirtualModelStore().Delete(virtualID)
		resetAdminTestState(t)
	})
	if err := saveUpstreams(virtualID, []upstreamInput{{ProviderID: oldID, ModelID: "native/model", Weight: 1}}); err != nil {
		t.Fatalf("save upstream: %v", err)
	}
	response := performJSONRequest(
		t, newAdminTestRouter(), http.MethodPatch,
		"/api/admin/providers/"+oldID+"/identity", `{"new_id":"namespace-provider-new"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("rename provider: %d %s", response.Code, response.Body.String())
	}
	attempts, err := resolveRequestedModels("test", virtualID)
	if err != nil || len(attempts) != 1 || attempts[0].ProviderID != newID || attempts[0].RequestedModel != "native/model" {
		t.Fatalf("native model routing after rename: %#v, %v", attempts, err)
	}
}
