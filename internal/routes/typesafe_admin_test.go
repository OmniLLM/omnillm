package routes

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"omnillm/internal/providers/types"
	"omnillm/internal/providers/typesafe"
	"omnillm/internal/systemone"
	"reflect"
	"strings"
	"testing"

	"omnillm/internal/database"
	"omnillm/internal/registry"
)

func TestTypeSafeAdminLifecycle(t *testing.T) {
	stubDefaultTransport(t, func(r *http.Request) (*http.Response, error) {
		status := 200
		body := `{"models":[{"name":"jev-latest","description":"Jev"}]}`
		if r.Header.Get("Authorization") != "Bearer good-sentinel" {
			status = 401
			body = `{"error":"bad-sentinel"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	registry.GetProviderRegistry().WaitForPendingSaves()
	router := newAdminTestRouter()
	created := performJSONRequest(t, router, "POST", "/api/admin/providers/login", `{"type":"typesafe","apiKey":"good-sentinel"}`)
	if created.Code != 200 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var result = decodeJSONBody[struct {
		ProviderID string `json:"provider_id"`
	}](t, created)
	id := result.ProviderID
	t.Cleanup(func() {
		_ = registry.GetProviderRegistry().Remove(id)
		registry.GetProviderRegistry().WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(id)
	})
	failed := performJSONRequest(t, router, "POST", "/api/admin/providers/login", `{"provider":"`+id+`","apiKey":"bad-sentinel"}`)
	if failed.Code < 400 || strings.Contains(failed.Body.String(), "bad-sentinel") {
		t.Fatalf("reauth: %d %s", failed.Code, failed.Body.String())
	}
	token, err := database.NewTokenStore().Get(id)
	if err != nil || token == nil || !strings.Contains(token.TokenData, "good-sentinel") {
		t.Fatal("working key lost")
	}
}

func TestTypeSafeMixedEvaluationThroughGateway(t *testing.T) {
	const requestBody = `{"model":"typesafe-mixed/jev-latest","state":{"text":"refund"},"questions":{"n":{"type":"noul","instructions":"Refund?"},"c":{"type":"choice","instructions":{"question":"Team?"},"criteria":{"billing":null,"other":["Other"]}},"s":{"type":"score","instructions":"Urgency?","criteria":["Low","High"]}}}`
	const answerBody = `{"model":"jev-1.13.0","answers":{"n":{"type":"noul","noul":0.9},"c":{"type":"choice","choice":"billing","probabilities":{"billing":0.9,"other":0.1},"confidence":0.7},"s":{"type":"score","score":0.8,"legend":{"0":"Low","1":"High"},"probabilities":{"0":0.2,"1":0.8},"confidence":0.6}},"usage":{"input_tokens":100,"output_tokens":30}}`
	calls := 0
	stubDefaultTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `{"models":[{"name":"jev-latest"}]}`
		if r.URL.Path == "/v1/systemone" {
			calls++
			got, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := systemone.ParseRequest([]byte(requestBody))
			expected.Model = "jev-latest"
			encoded, _ := json.Marshal(expected)
			if string(got) != string(encoded) {
				t.Fatalf("upstream payload changed: %s", got)
			}
			body = answerBody
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	reg := registry.GetProviderRegistry()
	p := typesafe.NewProvider("typesafe-mixed", "Mixed")
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.GetInstanceID(), ProviderID: p.GetID(), Name: p.GetName()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = reg.Remove(p.GetInstanceID())
		reg.WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(p.GetInstanceID())
	})
	if err := p.SetupAuth(&types.AuthOptions{APIKey: "synthetic-key"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(p, false); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddActive(p.GetInstanceID()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	SetupSystemOneRoutes(router.Group("/v1"))
	result := performJSONRequest(t, router, "POST", "/v1/systemone", requestBody)
	if result.Code != 200 || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", result.Code, calls, result.Body.String())
	}
	var actual, expected any
	_ = json.Unmarshal(result.Body.Bytes(), &actual)
	_ = json.Unmarshal([]byte(answerBody), &expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("native response changed")
	}
}
