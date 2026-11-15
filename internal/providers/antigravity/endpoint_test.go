package antigravity

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"omnillm/internal/cif"
)

func TestGenerationEndpointAndEffortSelection(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		wantURL string
		model   string
	}{
		{
			name:    "empty default",
			wantURL: "https://daily-cloudcode-pa.googleapis.com",
			model:   "gemini-3.8-flash-low",
		},
		{
			name:    "provider default",
			baseURL: NewProvider("instance", "Antigravity").GetBaseURL(),
			wantURL: "https://daily-cloudcode-pa.googleapis.com",
			model:   "gemini-3.8-flash-medium",
		},
		{
			name:    "explicit endpoint",
			baseURL: "http://upstream.test",
			wantURL: "http://upstream.test",
			model:   "gemini-3.8-flash-high",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []struct {
				name      string
				streaming bool
			}{
				{name: "streaming", streaming: true},
				{name: "collected"},
			} {
				t.Run(mode.name, func(t *testing.T) {
					old := antigravityStreamClient
					t.Cleanup(func() { antigravityStreamClient = old })
					calls := 0
					antigravityStreamClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						calls++
						want := tc.wantURL + "/v1internal:streamGenerateContent?alt=sse"
						if r.URL.String() != want {
							t.Errorf("endpoint = %q, want %q", r.URL.String(), want)
						}
						var payload struct {
							Model   string `json:"model"`
							Project string `json:"project"`
						}
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Fatal(err)
						}
						if payload.Model != tc.model || payload.Project != "existing-project" {
							t.Fatalf("model or project changed: %+v", payload)
						}
						if r.Header.Get("Authorization") != "Bearer existing-token" {
							t.Fatal("credential changed")
						}
						return jsonResponse(buildSSEEvent(map[string]any{
							"response": map[string]any{
								"candidates": []map[string]any{{
									"content": map[string]any{
										"role":  "model",
										"parts": []map[string]any{{"text": "OK"}},
									},
									"finishReason": "STOP",
								}},
							},
						})), nil
					})}
					request := &cif.CanonicalRequest{Model: tc.model}
					var text strings.Builder
					var stop cif.CIFStopReason
					if mode.streaming {
						ch, err := Stream(
							t.Context(),
							"existing-token",
							tc.baseURL,
							"existing-project",
							request,
						)
						if err != nil {
							t.Fatal(err)
						}
						for event := range ch {
							switch e := event.(type) {
							case cif.CIFContentDelta:
								if delta, ok := e.Delta.(cif.TextDelta); ok {
									text.WriteString(delta.Text)
								}
							case cif.CIFStreamEnd:
								stop = e.StopReason
							}
						}
					} else {
						response, err := Execute(
							t.Context(),
							"existing-token",
							tc.baseURL,
							"existing-project",
							request,
						)
						if err != nil {
							t.Fatal(err)
						}
						for _, part := range response.Content {
							if part, ok := part.(cif.CIFTextPart); ok {
								text.WriteString(part.Text)
							}
						}
						stop = response.StopReason
					}
					if text.String() != "OK" || stop != cif.StopReasonEndTurn || calls != 1 {
						t.Fatalf("text=%q stop=%q generation calls=%d", text.String(), stop, calls)
					}
				})
			}
		})
	}
}

func TestGemini38Catalog(t *testing.T) {
	t.Parallel()
	models := GetModels("existing-instance").Data
	for _, tc := range []struct {
		name string
		id   string
	}{
		{name: "Gemini 3.8 Flash (Low)", id: "gemini-3.8-flash-low"},
		{name: "Gemini 3.8 Flash (Medium)", id: "gemini-3.8-flash-medium"},
		{name: "Gemini 3.8 Flash (High)", id: "gemini-3.8-flash-high"},
		{name: "Gemini 2.5 Flash", id: "gemini-2.5-flash"},
		{name: "Claude Sonnet 4.6 (Thinking)", id: "claude-sonnet-4-6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches := 0
			for _, model := range models {
				if model.ID == tc.id {
					matches++
					if model.Name != tc.name || model.Provider != "existing-instance" {
						t.Errorf("unexpected catalog metadata: %+v", model)
					}
				}
			}
			if matches != 1 {
				t.Fatalf("catalog entries for %q = %d, want 1", tc.id, matches)
			}
		})
	}
	if got := RemapModel("gemini-3.8-flash"); got != "gemini-3.8-flash" {
		t.Fatalf("bare model acquired an implicit effort tier: %q", got)
	}
}

func TestDiscoverProjectDailyAndFallbacks(t *testing.T) {
	hosts := []string{
		"daily-cloudcode-pa.googleapis.com",
		"cloudcode-pa.googleapis.com",
		"daily-cloudcode-pa.sandbox.googleapis.com",
	}
	for _, tc := range []struct {
		name       string
		successAt  int
		wantCalls  int
		wantResult string
	}{
		{name: "daily", successAt: 0, wantCalls: 1, wantResult: "discovered-project"},
		{name: "production fallback", successAt: 1, wantCalls: 2, wantResult: "discovered-project"},
		{name: "sandbox fallback", successAt: 2, wantCalls: 3, wantResult: "discovered-project"},
		{name: "all unavailable", successAt: -1, wantCalls: 3, wantResult: DefaultProjectID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := []string{}
			withOAuthHTTPClient(t, func(r *http.Request) (*http.Response, error) {
				seen = append(seen, r.URL.Host)
				if r.URL.Path != "/v1internal:loadCodeAssist" {
					t.Fatalf("unexpected discovery path: %q", r.URL.Path)
				}
				if tc.successAt >= 0 && r.URL.Host == hosts[tc.successAt] {
					return jsonResponse(`{"cloudaicompanionProject":{"id":"discovered-project"}}`), nil
				}
				response := jsonResponse(`{}`)
				response.StatusCode = http.StatusServiceUnavailable
				return response, nil
			})
			if got := DiscoverProject("token"); got != tc.wantResult {
				t.Errorf("project = %q, want %q", got, tc.wantResult)
			}
			if !slices.Equal(seen, hosts[:tc.wantCalls]) {
				t.Errorf("discovery order = %v, want %v", seen, hosts[:tc.wantCalls])
			}
		})
	}
}
