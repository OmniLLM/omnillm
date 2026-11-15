package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runProviderMetadataShow(t *testing.T, serverURL, mode string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", serverURL, "")
	root.PersistentFlags().String("api-key", "test-key", "")
	root.PersistentFlags().String("output", mode, "")
	root.AddCommand(&cobra.Command{Use: providerShowCmd.Use, Args: providerShowCmd.Args, RunE: providerShowCmd.RunE})
	var output, diagnostics bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&diagnostics)
	root.SetArgs([]string{"show", "primary"})
	err := root.Execute()
	return output.String(), diagnostics.String(), err
}

func TestProviderShowDisplaysSafeMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authorization")
		}
		switch r.URL.Path {
		case "/api/admin/providers":
			fmt.Fprint(w, `[{"id":"provider-1","type":"antigravity","alias":"primary","config":{"client_secret":"LIST_SECRET"}}]`)
		case "/api/admin/providers/provider-1/metadata":
			fmt.Fprint(w, `{"id":"provider-1","authorization_endpoint":"https://accounts.google.com/o/oauth2/v2/auth","token_endpoint":"https://oauth2.googleapis.com/token","oauth_client_id":"public-client.apps.googleusercontent.com","api_endpoint_origin":"https://api.example.test","unknown":"UNKNOWN_SECRET","client_secret":"CLIENT_SECRET","access_token":"ACCESS_SECRET"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	for _, mode := range []string{"table", "json"} {
		t.Run(mode, func(t *testing.T) {
			output, diagnostics, err := runProviderMetadataShow(t, server.URL, mode)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"accounts.google.com/o/oauth2/v2/auth", "public-client.apps.googleusercontent.com", "api.example.test"} {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q: %s", want, output)
				}
			}
			for _, secret := range []string{"LIST_SECRET", "UNKNOWN_SECRET", "CLIENT_SECRET", "ACCESS_SECRET"} {
				if strings.Contains(output+diagnostics, secret) {
					t.Errorf("secret %q was exposed", secret)
				}
			}
			if mode == "json" {
				var details map[string]any
				if err := json.Unmarshal([]byte(output), &details); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if details["oauth_client_id"] != "public-client.apps.googleusercontent.com" {
					t.Errorf("client ID missing from JSON")
				}
			}
		})
	}
}

func TestProviderShowMetadataFailureAndLegacyServer(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body       string
		wantError  bool
		wantNotice bool
	}{
		{name: "older server", status: http.StatusNotFound, body: "404 page not found", wantNotice: true},
		{name: "provider disappeared", status: http.StatusNotFound, body: `{"error":"provider_not_found"}`, wantError: true},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"TOKEN_SECRET"}`, wantError: true},
		{name: "server error", status: http.StatusInternalServerError, body: `TOKEN_SECRET`, wantError: true},
		{name: "invalid response", status: http.StatusOK, body: `{"oops":`, wantError: true},
		{name: "mismatched provider", status: http.StatusOK, body: `{"id":"other-provider"}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/admin/providers" {
					fmt.Fprint(w, `[{"id":"provider-1","type":"google","alias":"primary"}]`)
					return
				}
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			output, diagnostics, err := runProviderMetadataShow(t, server.URL, "json")
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v, wantError=%v", err, test.wantError)
			}
			if strings.Contains(fmt.Sprint(err)+output+diagnostics, "TOKEN_SECRET") {
				t.Fatal("server error leaked secret")
			}
			if test.wantNotice && (!strings.Contains(diagnostics, "metadata") || !json.Valid([]byte(output))) {
				t.Fatalf("legacy output=%q diagnostics=%q", output, diagnostics)
			}
		})
	}
}

func TestProviderShowSanitizesUntrustedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/providers" {
			fmt.Fprint(w, `[{"id":"provider-1","type":"antigravity","alias":"primary"}]`)
			return
		}
		fmt.Fprint(w, `{"id":"provider-1","authorization_endpoint":"https://accounts.google.com/o/oauth2/v2/auth?state=STATE_SECRET","token_endpoint":"https://user:USERINFO_SECRET@oauth2.googleapis.com/token","oauth_client_id":"public-client.apps.googleusercontent.com","api_endpoint_origin":"https://user:USERINFO_SECRET@api.example.test/PATH_SECRET?token=QUERY_SECRET#FRAGMENT_SECRET","unknown":"UNKNOWN_SECRET"}`)
	}))
	defer server.Close()
	for _, mode := range []string{"table", "json"} {
		t.Run(mode, func(t *testing.T) {
			output, diagnostics, err := runProviderMetadataShow(t, server.URL, mode)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, "https://api.example.test") {
				t.Fatalf("sanitized endpoint origin missing: %s", output)
			}
			if strings.Contains(output+diagnostics, "SECRET") {
				t.Fatalf("metadata exposed secret: %s %s", output, diagnostics)
			}
		})
	}
}
