package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/systemone"
)

func TestAuthenticationLifecycle(t *testing.T) {
	if err := database.InitializeDatabase(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.GetDatabase().Close() })
	const id = "typesafe-test"
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{
		InstanceID: id, ProviderID: "typesafe", Name: "My account",
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer good-sentinel" {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, "bad-secret")
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"jev-latest","description":"Jev"}]}`)
	}))
	defer server.Close()
	p := NewProvider(id, "My account")
	p.baseURL = server.URL + "/v1"
	if err := p.SetupAuth(&types.AuthOptions{APIKey: "good-sentinel"}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetupAuth(&types.AuthOptions{APIKey: "bad-secret"}); err == nil || strings.Contains(err.Error(), "bad-secret") {
		t.Fatalf("unsafe error: %v", err)
	}
	restored := NewProvider(id, "My account")
	if err := restored.LoadFromDB(); err != nil {
		t.Fatal(err)
	}
	if restored.GetToken() != "good-sentinel" || restored.GetName() != "My account" {
		t.Fatal("reauth overwrote prior state")
	}
	models, err := p.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(models.Data) != 1 || models.Data[0].Capabilities["generation"] != false {
		t.Fatalf("bad catalog: %+v", models)
	}
}

func TestEvaluatePreservesNativeResult(t *testing.T) {
	request, err := systemone.ParseRequest([]byte(`{"model":"jev-latest","state":{"text":"hello"},"questions":{"n":{"type":"noul","instructions":"Greeting?"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer sentinel" {
			t.Error("incorrect request")
		}
		var got systemone.Request
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Model != request.Model {
			t.Error("incorrect body")
		}
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{"n":{"type":"noul","noul":0.91}},"usage":{"input_tokens":12,"output_tokens":3}}`)
	}))
	defer server.Close()
	p := NewProvider("test", "")
	p.baseURL = server.URL + "/v1"
	p.token = "sentinel"
	result, err := p.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "jev-1.13.0" || result.Usage.InputTokens != 12 || !strings.Contains(string(result.Answers["n"]), "0.91") {
		t.Fatalf("bad result: %+v", result)
	}
	if _, err := p.CreateChatCompletions(nil); err == nil {
		t.Fatal("chat accepted")
	}
	if _, err := p.CreateEmbeddings(nil); err == nil {
		t.Fatal("embeddings accepted")
	}
}

func TestUpstreamFailuresAndCancellation(t *testing.T) {
	request, _ := systemone.ParseRequest([]byte(`{"model":"jev-latest","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`))
	for _, status := range []int{401, 422, 429, 529, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"secret":"sentinel"}`)
			}))
			defer server.Close()
			p := NewProvider("test", "")
			p.baseURL = server.URL
			p.token = "sentinel"
			_, err := p.Evaluate(context.Background(), request)
			if err == nil || strings.Contains(err.Error(), "sentinel") || calls != 1 {
				t.Fatalf("unsafe failure: %v calls=%d", err, calls)
			}
			var upstream *systemone.UpstreamError
			if status != 200 && (!errors.As(err, &upstream) || upstream.StatusCode() != status || upstream.RetryAfter != "7") {
				t.Fatalf("lost status: %v", err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	p := NewProvider("test", "")
	p.baseURL = server.URL
	p.token = "sentinel"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Evaluate(ctx, request); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestTimeoutAndRedirectDoNotLeakCredentials(t *testing.T) {
	request, _ := systemone.ParseRequest([]byte(`{"model":"jev-latest","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`))
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	p := NewProvider("test", "")
	p.baseURL = redirect.URL
	p.token = "sentinel"
	if _, err := p.Evaluate(context.Background(), request); err == nil || calls != 0 {
		t.Fatal("credential-bearing redirect followed")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(50 * time.Millisecond)
	}))
	defer slow.Close()
	p.baseURL = slow.URL
	p.client.Timeout = 5 * time.Millisecond
	if _, err := p.Evaluate(context.Background(), request); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout not enforced: %v", err)
	}
}
