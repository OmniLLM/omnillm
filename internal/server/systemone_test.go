package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/routes"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"omnillm/internal/systemone"
)

type evaluationStub struct {
	types.Provider
	id      string
	calls   atomic.Int32
	failure error
	block   func(context.Context)
}

func (p *evaluationStub) GetID() string         { return "typesafe" }
func (p *evaluationStub) GetInstanceID() string { return p.id }
func (p *evaluationStub) GetName() string       { return "TypeSafe test" }
func (p *evaluationStub) SetName(string)        {}
func (p *evaluationStub) GetModels() (*types.ModelsResponse, error) {
	return &types.ModelsResponse{Data: []types.Model{{ID: "jev-latest", Name: "Jev", Capabilities: map[string]any{"generation": false, "evaluation": true}}}}, nil
}
func (p *evaluationStub) Evaluate(ctx context.Context, req *systemone.Request) (*systemone.Response, error) {
	p.calls.Add(1)
	if p.block != nil {
		p.block(ctx)
		return nil, ctx.Err()
	}
	if p.failure != nil {
		return nil, p.failure
	}
	return &systemone.Response{Model: req.Model, Answers: map[string]json.RawMessage{"n": json.RawMessage(`{"type":"noul","noul":0.9}`)}, Usage: systemone.Usage{InputTokens: 10, OutputTokens: 2}}, nil
}
func registerEvaluationStub(t *testing.T) *evaluationStub {
	t.Helper()
	p := &evaluationStub{id: fmt.Sprintf("typesafe-test-%d", stubProviderCounter.Add(1))}
	reg := registry.GetProviderRegistry()
	if err := reg.Register(p, true); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddActive(p.id); err != nil {
		t.Fatal(err)
	}
	// Requests and model-state writes depend on the persisted provider record.
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		_ = reg.Remove(p.id)
		reg.WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(p.id)
	})
	return p
}
func TestSystemOneNativeAndUnsupportedShapes(t *testing.T) {
	p := registerEvaluationStub(t)
	srv := newTestServer(t)
	defer srv.Close()
	for _, model := range []string{"jev-latest", p.id + "/jev-1.13.0"} {
		resp := postJSON(t, srv.URL+"/v1/systemone", fmt.Sprintf(`{"model":%q,"state":{"text":"hello"},"questions":{"n":{"type":"noul","instructions":"Greeting?"}}}`, model), nil)
		body := readBody(t, resp)
		if resp.StatusCode != 200 || !strings.Contains(body, `"noul":0.9`) {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
	}
	calls := p.calls.Load()
	for _, row := range []struct{ path, body string }{
		{"/v1/chat/completions", `{"model":%q,"messages":[{"role":"user","content":"hi"}]}`},
		{"/v1/messages", `{"model":%q,"max_tokens":12,"messages":[{"role":"user","content":"hi"}]}`},
		{"/v1/responses", `{"model":%q,"input":"hi"}`},
		{"/v1/embeddings", `{"model":%q,"input":"hi"}`},
	} {
		resp := postJSON(t, srv.URL+row.path, fmt.Sprintf(row.body, p.id+"/jev-latest"), nil)
		body := readBody(t, resp)
		if resp.StatusCode < 400 || !strings.Contains(body, "unsupported") {
			t.Errorf("%s status=%d body=%s", row.path, resp.StatusCode, body)
		}
	}
	if p.calls.Load() != calls {
		t.Fatal("generation reached evaluation")
	}
}
func TestSystemOneValidationAndAuthentication(t *testing.T) {
	p := registerEvaluationStub(t)
	srv := newTestServer(t)
	defer srv.Close()
	for _, body := range []string{`{}`, `{"model":"jev-latest","state":"x","questions":{},"tools":[]}`} {
		resp := postJSON(t, srv.URL+"/v1/systemone", body, nil)
		readBody(t, resp)
		if resp.StatusCode != 400 {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}
	resp, err := http.Post(srv.URL+"/v1/systemone", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	readBody(t, resp)
	if resp.StatusCode != 401 {
		t.Fatalf("auth status %d", resp.StatusCode)
	}
	if p.calls.Load() != 0 {
		t.Fatal("invalid request dispatched")
	}
}

func TestSystemOneDisabledModelAndMetering(t *testing.T) {
	p := registerEvaluationStub(t)
	srv := newTestServer(t)
	defer srv.Close()
	body := `{"model":"jev-latest","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`
	if err := database.NewModelStateStore().SetEnabled(p.id, "jev-latest", false); err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, srv.URL+"/v1/systemone", body, nil)
	readBody(t, resp)
	if resp.StatusCode != 404 || p.calls.Load() != 0 {
		t.Fatal("disabled model executed")
	}
	if err := database.NewModelStateStore().SetEnabled(p.id, "jev-latest", true); err != nil {
		t.Fatal(err)
	}
	resp = postJSON(t, srv.URL+"/v1/systemone", body, nil)
	readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("enabled model status %d", resp.StatusCode)
	}
	deadline := time.Now().Add(time.Second)
	for {
		rows, total, err := database.GetDatabase().ListMeteringRecords(database.MeteringFilter{ProviderID: p.id, APIShape: "systemone"}, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total > 0 {
			if total != 1 || rows[0].InputTokens != 10 || rows[0].OutputTokens != 2 || rows[0].CacheReadInputTokens != nil {
				t.Fatalf("incorrect metering: %+v", rows)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("usage not recorded")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSystemOneMixedVirtualModel(t *testing.T) {
	p := registerEvaluationStub(t)
	chatID := registerStubProvider(t, "chat-only", func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
		return &cif.CanonicalResponse{ID: "chat-result", Model: req.Model, Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ok"}}, StopReason: cif.StopReasonEndTurn}, nil
	}, nil)
	vmID := p.id + "-virtual"
	store := database.NewVirtualModelStore()
	if err := store.Create(&database.VirtualModelRecord{VirtualModelID: vmID, Name: vmID, Enabled: true, APIShape: "openai", LbStrategy: database.LbStrategyPriority}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Delete(vmID) })
	upstreams := []database.VirtualModelUpstreamRecord{
		{VirtualModelID: vmID, ProviderID: p.id, ModelID: "jev-latest", Priority: 0, Weight: 1},
		{VirtualModelID: vmID, ProviderID: chatID, ModelID: "chat-only", Priority: 1, Weight: 1},
	}
	if err := database.NewVirtualModelUpstreamStore().SetForVModel(vmID, upstreams); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t)
	defer srv.Close()
	resp := postJSON(t, srv.URL+"/v1/chat/completions", fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]}`, vmID), nil)
	body := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("chat %d %s", resp.StatusCode, body)
	}
	resp = postJSON(t, srv.URL+"/v1/systemone", fmt.Sprintf(`{"model":%q,"state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`, vmID), nil)
	body = readBody(t, resp)
	if resp.StatusCode != 200 || p.calls.Load() != 1 {
		t.Fatalf("evaluation %d %s", resp.StatusCode, body)
	}
}

func TestSystemOneFailoverAndStatus(t *testing.T) {
	first := registerEvaluationStub(t)
	second := registerEvaluationStub(t)
	first.failure = &systemone.UpstreamError{Status: 529, RetryAfter: "3"}
	second.failure = &systemone.UpstreamError{Status: 429, RetryAfter: "7"}
	for index, p := range []*evaluationStub{first, second} {
		if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.id, ProviderID: "typesafe", Name: p.id, Priority: index, Activated: true}); err != nil {
			t.Fatal(err)
		}
	}
	srv := newTestServer(t)
	defer srv.Close()
	body := `{"model":"jev-latest","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`
	resp := postJSON(t, srv.URL+"/v1/systemone", body, nil)
	readBody(t, resp)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "7" || first.calls.Load() != 1 || second.calls.Load() != 1 {
		t.Fatal("failover lost status or retried an instance")
	}
	second.failure = nil
	resp = postJSON(t, srv.URL+"/v1/systemone", body, nil)
	readBody(t, resp)
	if resp.StatusCode != 200 || first.calls.Load() != 2 || second.calls.Load() != 2 {
		t.Fatal("eligible failover failed")
	}
}

func TestSystemOneConcurrencyAndCancellation(t *testing.T) {
	p := registerEvaluationStub(t)
	entered := make(chan struct{})
	finished := make(chan struct{})
	p.block = func(ctx context.Context) { close(entered); <-ctx.Done(); close(finished) }
	router := buildRouter(0, "test-api-key", routes.ChatCompletionOptions{}, 1)
	srv := httptest.NewServer(router)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := `{"model":"jev-latest","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`
	req := newAuthenticatedRequest(t, "POST", srv.URL+"/v1/systemone", strings.NewReader(body)).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("evaluation did not start")
	}
	resp := postJSON(t, srv.URL+"/v1/systemone", body, nil)
	readBody(t, resp)
	if resp.StatusCode != 503 || resp.Header.Get("Retry-After") != "1" {
		t.Fatal("concurrency limit bypassed")
	}
	unauth, err := http.Post(srv.URL+"/v1/systemone", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	readBody(t, unauth)
	if unauth.StatusCode != 401 {
		t.Fatal("limiter ran before authentication")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("upstream context not canceled")
	}
	<-done
	if p.calls.Load() != 1 {
		t.Fatal("cancellation retried evaluation")
	}
}
