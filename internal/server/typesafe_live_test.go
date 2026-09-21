package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/providers/typesafe"
	"omnillm/internal/registry"
	"omnillm/internal/systemone"
)

// This smoke uses TestMain's temporary database and httptest's fresh loopback port.
func TestTypeSafeLiveSmoke(t *testing.T) {
	if os.Getenv("OMNILLM_TYPESAFE_LIVE") != "1" {
		t.Skip("set OMNILLM_TYPESAFE_LIVE=1 to enable the credentialed smoke")
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY is unavailable")
	}
	p := typesafe.NewProvider("typesafe-live-smoke", "TypeSafe smoke")
	reg := registry.GetProviderRegistry()
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.GetInstanceID(), ProviderID: p.GetID(), Name: p.GetName()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = reg.Remove(p.GetInstanceID())
		reg.WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(p.GetInstanceID())
	})
	if err := p.SetupAuth(&types.AuthOptions{APIKey: key}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(p, true); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddActive(p.GetInstanceID()); err != nil {
		t.Fatal(err)
	}
	reg.WaitForPendingSaves()
	// Reconstruct from durable credentials without deleting the account.
	restored := typesafe.NewProvider(p.GetInstanceID(), p.GetName())
	if err := restored.LoadFromDB(); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(restored, false); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t)
	defer srv.Close()
	request := `{"model":"typesafe-live-smoke/jev-latest","state":{"message":"Please refund the duplicate charge today."},"questions":{"refund":{"type":"noul","instructions":"Is a refund requested?"},"team":{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Payment issues","technical":"Software faults"}},"urgency":{"type":"score","instructions":"How urgent is this request?","criteria":["Routine","Time-sensitive","Immediate action needed"]}}}`
	resp := postJSON(t, srv.URL+"/v1/systemone", request, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("evaluation HTTP status %d: %s", resp.StatusCode, body)
	}
	req, err := systemone.ParseRequest([]byte(request))
	if err != nil {
		t.Fatal(err)
	}
	result, err := systemone.ValidateResponse([]byte(body), req)
	if err != nil {
		t.Fatal(err)
	}
	modelsReq := newAuthenticatedRequest(t, "GET", srv.URL+"/v1/models", nil)
	modelsResp, err := http.DefaultClient.Do(modelsReq)
	if err != nil {
		t.Fatal(err)
	}
	modelsBody := readBody(t, modelsResp)
	if modelsResp.StatusCode != 200 || !strings.Contains(modelsBody, "jev-latest") || !strings.Contains(modelsBody, "systemone") {
		t.Fatal("evaluation model metadata unavailable")
	}
	typesSeen := make(map[string]string)
	for id, raw := range result.Answers {
		var answer struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &answer)
		typesSeen[id] = answer.Type
	}
	t.Logf("gateway=%s restart=pass catalog=pass model=%s answers=%v input_tokens=%d output_tokens=%d", srv.URL, result.Model, typesSeen, result.Usage.InputTokens, result.Usage.OutputTokens)
}
