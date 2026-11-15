package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "antigravity-test-*")
	if err != nil {
		panic(err)
	}
	if err := database.InitializeDatabase(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func refreshProvider(t *testing.T, expiry bool) *Provider {
	t.Helper()
	p := NewProvider(t.Name(), "Antigravity")
	err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{InstanceID: p.instanceID, ProviderID: "antigravity", Name: "Antigravity"})
	if err != nil {
		t.Fatal(err)
	}
	d := map[string]any{"access_token": "old", "refresh_token": "refresh", "client_id": "client", "client_secret": "secret", "project_id": "project", "email": "test@example.test", "custom": "keep"}
	if expiry {
		d["expires_at"] = time.Now().Add(time.Hour).Unix()
	}
	if err := database.NewTokenStore().Save(p.instanceID, d); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutomaticRefreshConcurrentAndDurable(t *testing.T) {
	p := refreshProvider(t, false)
	var calls atomic.Int32
	withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResponse(`{"access_token":"new","expires_in":3600}`), nil
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			d, err := p.credentials(t.Context(), "")
			if err != nil || d["access_token"] != "new" {
				t.Errorf("refresh: %v", err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("exchanges=%d", calls.Load())
	}
	record, err := database.NewTokenStore().Get(p.instanceID)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(record.TokenData), &d); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"refresh_token": "refresh", "project_id": "project", "email": "test@example.test", "custom": "keep"} {
		if d[k] != v {
			t.Fatalf("lost %s", k)
		}
	}
	restarted := NewProvider(p.instanceID, "")
	if _, err := restarted.credentials(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("restart refreshed fresh token")
	}
}

func TestRefreshDoesNotOverwriteOAuth(t *testing.T) {
	p := refreshProvider(t, false)
	withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
		err := database.NewTokenStore().Save(p.instanceID, map[string]any{"access_token": "oauth-new", "expires_at": time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		return jsonResponse(`{"access_token":"stale-refresh","expires_in":3600}`), nil
	})
	d, err := p.credentials(t.Context(), "")
	if err != nil || d["access_token"] != "oauth-new" {
		t.Fatalf("newer OAuth overwritten: %v", err)
	}
}

func TestAuthenticationRetryBounded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		want    int
		success bool
	}{{"recovered", 401, 2, true}, {"rejected", 401, 2, false}, {"forbidden", 403, 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			p := refreshProvider(t, true)
			var refreshes, calls atomic.Int32
			withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
				refreshes.Add(1)
				return jsonResponse(`{"access_token":"new","expires_in":3600}`), nil
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if tc.success && n == 2 {
					if r.Header.Get("Authorization") != "Bearer new" {
						t.Error("retry used old token")
					}
					w.Write([]byte("data: [DONE]\n\n"))
					return
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			p.baseURL = server.URL
			ch, err := p.GetAdapter().ExecuteStream(t.Context(), &cif.CanonicalRequest{Model: "claude-sonnet-4-6"})
			if tc.success {
				if err != nil {
					t.Fatal(err)
				}
				for range ch {
				}
			} else if err == nil {
				t.Fatal("wanted failure")
			}
			if int(calls.Load()) != tc.want {
				t.Fatalf("generation attempts=%d", calls.Load())
			}
			if tc.status == 403 && refreshes.Load() != 0 {
				t.Fatal("refreshed on 403")
			}
			if refreshes.Load() > 1 {
				t.Fatal("unbounded refresh")
			}
		})
	}
}

func TestRefreshErrorsAreSanitizedAndCancellable(t *testing.T) {
	for _, body := range []string{`{"error":"invalid_grant","error_description":"SECRET"}`, `{"access_token":"SECRET"`, `{"unexpected":"SECRET"}`} {
		t.Run(body, func(t *testing.T) {
			withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) { return jsonResponse(body), nil })
			_, err := refreshAccessToken(context.Background(), "client", "secret", "refresh")
			if err == nil || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
	p := refreshProvider(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.credentials(ctx, ""); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestExpiryRotationAndExplicitRefresh(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expiry   int64
		explicit bool
		want     int32
	}{{"fresh", time.Now().Add(time.Hour).Unix(), false, 0}, {"near", time.Now().Add(30 * time.Second).Unix(), false, 1}, {"expired", time.Now().Add(-time.Hour).Unix(), false, 1}, {"explicit", time.Now().Add(time.Hour).Unix(), true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			p := refreshProvider(t, true)
			store := database.NewTokenStore()
			rec, _ := store.Get(p.instanceID)
			var d map[string]any
			json.Unmarshal([]byte(rec.TokenData), &d)
			d["expires_at"] = tc.expiry
			if err := store.Save(p.instanceID, d); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return jsonResponse(`{"access_token":"new","refresh_token":"rotated","expires_in":3600}`), nil
			})
			var err error
			if tc.explicit {
				err = p.RefreshToken()
			} else {
				_, err = p.credentials(t.Context(), "")
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != tc.want {
				t.Fatalf("exchanges=%d", calls.Load())
			}
			if tc.want > 0 {
				rec, _ = store.Get(p.instanceID)
				json.Unmarshal([]byte(rec.TokenData), &d)
				if d["refresh_token"] != "rotated" {
					t.Fatal("rotation lost")
				}
			}
		})
	}
}

func TestRefreshCancellationDuringExchangeAndWait(t *testing.T) {
	p := refreshProvider(t, false)
	entered := make(chan struct{})
	withOAuthHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.credentials(ctx, ""); done <- err }()
	<-entered
	waiter, cancelWait := context.WithCancel(t.Context())
	cancelWait()
	if _, err := p.credentials(waiter, ""); err == nil {
		t.Fatal("waiting ignored cancellation")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("exchange ignored cancellation")
	}
}

func TestNonStreamingRecoveryAndNoRetryAfterStreamStarts(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "collected"
		if streaming {
			name = "partial-stream"
		}
		t.Run(name, func(t *testing.T) {
			p := refreshProvider(t, true)
			var calls, refreshes atomic.Int32
			withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
				refreshes.Add(1)
				return jsonResponse(`{"access_token":"new","expires_in":3600}`), nil
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if !streaming && n == 1 {
					w.WriteHeader(401)
					return
				}
				w.Write([]byte(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}]}}` + "\n\n"))
			}))
			defer server.Close()
			p.baseURL = server.URL
			if streaming {
				ch, err := p.GetAdapter().ExecuteStream(t.Context(), &cif.CanonicalRequest{Model: "claude-sonnet-4-6"})
				if err != nil {
					t.Fatal(err)
				}
				for range ch {
				}
				if calls.Load() != 1 || refreshes.Load() != 0 {
					t.Fatal("replayed stream")
				}
			} else {
				_, err := p.GetAdapter().Execute(t.Context(), &cif.CanonicalRequest{Model: "claude-sonnet-4-6"})
				if err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 2 || refreshes.Load() != 1 {
					t.Fatal("missing recovery")
				}
			}
		})
	}
}

func TestRefreshHTTPFailurePreservesCredentials(t *testing.T) {
	p := refreshProvider(t, false)
	withOAuthHTTPClient(t, func(*http.Request) (*http.Response, error) {
		r := jsonResponse(`{"access_token":"SECRET"}`)
		r.StatusCode = 503
		return r, nil
	})
	if _, err := p.credentials(t.Context(), ""); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("failure=%v", err)
	}
	rec, _ := database.NewTokenStore().Get(p.instanceID)
	if !strings.Contains(rec.TokenData, `"access_token":"old"`) {
		t.Fatal("failed refresh changed credentials")
	}
}

func TestMissingRefreshCredentialsAndDeletedRecord(t *testing.T) {
	p := refreshProvider(t, true)
	if err := database.NewTokenStore().Save(p.instanceID, map[string]any{"access_token": "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.credentials(t.Context(), "old"); err == nil || !strings.Contains(err.Error(), "re-authenticate") {
		t.Fatalf("missing grant: %v", err)
	}
	if err := database.NewTokenStore().Delete(p.instanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.credentials(t.Context(), ""); err == nil {
		t.Fatal("used stale credentials after deletion")
	}
}
