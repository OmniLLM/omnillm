package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

const showProvidersFixture = `[
  {"id":"first","type":"google","name":"Shared Name","alias":"primary","subtitle":"primary","authStatus":"authenticated","isActive":true,"totalModelCount":3,"enabledModelCount":2,"config":{"api_key":"secret-config"},"extra":"secret-extra"},
  {"id":"second","type":"openai","name":"Other Name","alias":"first","authStatus":"unauthenticated","isActive":false,"totalModelCount":0,"enabledModelCount":0},
  {"id":"third","type":"google","name":"Shared Name","alias":"duplicate"},
  {"id":"fourth","type":"google","name":"Fourth","alias":"DUPLICATE"},
  {"id":"fifth","type":"google","name":"PRIMARY","alias":"fifth-alias"},
  {"id":"sixth","type":"google","name":"Sixth","subtitle":"legacy"}
]`

const showModelsFixture = `{"models":[
  {"id":"vendor/model","name":"Example Model","description":"A useful model","enabled":true,"max_tokens":32000,"output_tokens":4096,"capabilities":{"vision":true,"tools":false}},
  {"id":"model","name":"Other model","enabled":false}
],"total":2}`

func showChild(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, command := range parent.Commands() {
		if command.Name() == name {
			return command
		}
	}
	t.Fatalf("%s is missing %q", parent.Name(), name)
	return nil
}

func runShowForTest(t *testing.T, command *cobra.Command, serverURL, output string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", serverURL, "")
	root.PersistentFlags().String("api-key", "test-admin-key", "")
	root.PersistentFlags().String("output", output, "")
	copyCommand := &cobra.Command{Use: command.Use, Short: command.Short, Args: command.Args, RunE: command.RunE}
	root.AddCommand(copyCommand)
	var result bytes.Buffer
	root.SetOut(&result)
	root.SetErr(&result)
	root.SetArgs(append([]string{"show"}, args...))
	err := root.Execute()
	return result.String(), err
}

func TestProviderShowReferenceResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/providers" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-admin-key" {
			t.Errorf("unexpected authorization header")
		}
		fmt.Fprint(w, showProvidersFixture)
	}))
	defer server.Close()

	command := showChild(t, ProviderCmd, "show")
	tests := []struct {
		name      string
		reference string
		wantID    string
		wantError string
	}{
		{name: "exact id wins over alias", reference: "first", wantID: "first"},
		{name: "id trims whitespace", reference: " first ", wantID: "first"},
		{name: "alias wins over name", reference: " PRIMARY ", wantID: "first"},
		{name: "legacy subtitle alias", reference: "LEGACY", wantID: "sixth"},
		{name: "case insensitive name", reference: " other NAME ", wantID: "second"},
		{name: "ambiguous alias", reference: "duplicate", wantError: "fourth, third"},
		{name: "ambiguous name", reference: "shared name", wantError: "first, third"},
		{name: "missing", reference: "absent", wantError: "not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := runShowForTest(t, command, server.URL, "json", test.reference)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error=%v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("show provider: %v", err)
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("invalid JSON %q: %v", output, err)
			}
			if result["id"] != test.wantID {
				t.Errorf("id=%v, want %q", result["id"], test.wantID)
			}
		})
	}
}

func TestProviderShowSafeOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, showProvidersFixture)
	}))
	defer server.Close()
	command := showChild(t, ProviderCmd, "show")
	for _, mode := range []string{"table", "json"} {
		t.Run(mode, func(t *testing.T) {
			output, err := runShowForTest(t, command, server.URL, mode, "primary")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"first", "google", "Shared Name", "primary", "authenticated", "true", "2", "3"} {
				if !strings.Contains(output, want) {
					t.Errorf("output %q missing %q", output, want)
				}
			}
			for _, forbidden := range []string{"secret-config", "secret-extra", "api_key", `"config"`, `"extra"`} {
				if strings.Contains(output, forbidden) {
					t.Errorf("output contains forbidden %q", forbidden)
				}
			}
			if mode == "json" {
				var result map[string]any
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatal(err)
				}
				if result["isActive"] != true || result["enabledModelCount"] != float64(2) || result["totalModelCount"] != float64(3) {
					t.Errorf("missing status/counts: %v", result)
				}
			}
		})
	}
}

func TestProviderShowRejectsNonScalarSafeFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"id":"first","name":{"api_key":"nested-secret"},"isActive":{"token":"nested-secret"},"totalModelCount":{"token":"nested-secret"}}]`)
	}))
	defer server.Close()
	command := showChild(t, ProviderCmd, "show")
	for _, mode := range []string{"table", "json"} {
		output, err := runShowForTest(t, command, server.URL, mode, "first")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output, "nested-secret") {
			t.Errorf("%s output leaked nested field: %q", mode, output)
		}
	}
}

func TestModelShowDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/providers/primary/models" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, showModelsFixture)
	}))
	defer server.Close()
	command := showChild(t, ModelCmd, "show")
	for _, mode := range []string{"table", "json"} {
		t.Run(mode, func(t *testing.T) {
			output, err := runShowForTest(t, command, server.URL, mode, "primary", "vendor/model")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"vendor/model", "Example Model", "A useful model", "32000", "4096", "vision", "true"} {
				if !strings.Contains(output, want) {
					t.Errorf("output %q missing %q", output, want)
				}
			}
			if strings.Contains(output, "Other model") {
				t.Errorf("output includes unselected model: %q", output)
			}
			if mode == "json" {
				var result map[string]any
				if err := json.Unmarshal([]byte(output), &result); err != nil || result["id"] != "vendor/model" {
					t.Fatalf("selected model JSON=%q err=%v", output, err)
				}
			}
		})
	}
	_, err := runShowForTest(t, command, server.URL, "table", "primary", "Vendor/model")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing model error=%v", err)
	}
}

func TestShowValidationAndServerErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, `{"error":"discovery unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()
	for _, parent := range []*cobra.Command{ProviderCmd, ModelCmd} {
		command := showChild(t, parent, "show")
		if _, err := runShowForTest(t, command, server.URL, "table"); err == nil {
			t.Errorf("%s show should reject missing arguments", parent.Name())
		}
		before := requests.Load()
		args := []string{"primary"}
		if parent == ModelCmd {
			args = append(args, "model")
		}
		if _, err := runShowForTest(t, command, server.URL, "table", append(args, "extra")...); err == nil {
			t.Errorf("%s show should reject extra arguments", parent.Name())
		}
		if requests.Load() != before {
			t.Errorf("%s show sent a request for invalid arguments", parent.Name())
		}
		if _, err := runShowForTest(t, command, server.URL, "table", args...); err == nil || !strings.Contains(err.Error(), "discovery unavailable") {
			t.Errorf("%s show server error=%v", parent.Name(), err)
		}
	}
}

func TestModelShowAmbiguousProviderIncludesMatchingIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"ambiguous_provider_reference","message":"provider reference is ambiguous; matching instance IDs: first, third","matching_ids":["first","third"]}`)
	}))
	defer server.Close()
	command := showChild(t, ModelCmd, "show")
	_, err := runShowForTest(t, command, server.URL, "table", "ambiguous", "model")
	if err == nil || !strings.Contains(err.Error(), "first, third") {
		t.Fatalf("ambiguous provider error=%v", err)
	}
}

func TestShowHelpAndLegacyModel(t *testing.T) {
	for _, parent := range []*cobra.Command{ProviderCmd, ModelCmd} {
		command := showChild(t, parent, "show")
		output, err := runShowForTest(t, command, "http://127.0.0.1:1", "table", "--help")
		if err != nil || !strings.Contains(output, command.Short) || !strings.Contains(output, "<reference>") {
			t.Errorf("%s show help=%q err=%v", parent.Name(), output, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, showModelsFixture)
	}))
	defer server.Close()
	t.Setenv("OMNILLM_SERVER", server.URL)
	var output bytes.Buffer
	source := &cobra.Command{Use: "model"}
	source.SetOut(&output)
	target := showChild(t, ModelCmd, "show")
	t.Cleanup(func() { target.SetOut(nil) })
	if err := executeLegacyModelCommand(source, ModelCmd, []string{"show", "primary", "vendor/model"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "vendor/model") {
		t.Fatalf("legacy output=%q", output.String())
	}
}

func TestLegacyModelShowWithGlobalFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, showModelsFixture)
	}))
	defer server.Close()
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "flags before legacy command", args: []string{"--server", server.URL, "--output", "json", "model", "show", "primary", "vendor/model"}},
		{name: "flags before show", args: []string{"model", "--server", server.URL, "--output", "json", "show", "primary", "vendor/model"}},
		{name: "flags after show", args: []string{"model", "show", "primary", "vendor/model", "--server", server.URL, "--output", "json"}},
		{name: "equals and short flags", args: []string{"model", "--server=" + server.URL, "-o=json", "show", "primary", "vendor/model"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
			root.PersistentFlags().String("server", "", "")
			root.PersistentFlags().String("api-key", "", "")
			root.PersistentFlags().StringP("output", "o", "table", "")
			provider := &cobra.Command{Use: "provider"}
			model := &cobra.Command{Use: "model"}
			original := showChild(t, ModelCmd, "show")
			model.AddCommand(&cobra.Command{Use: original.Use, Short: original.Short, Args: original.Args, RunE: original.RunE})
			provider.AddCommand(model)
			root.AddCommand(provider)
			legacy := &cobra.Command{
				Use: "model", DisableFlagParsing: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					return executeLegacyModelCommand(cmd, model, args)
				},
			}
			root.AddCommand(legacy)
			var output, diagnostics bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&diagnostics)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute legacy show: %v", err)
			}
			var result map[string]any
			if err := json.Unmarshal(output.Bytes(), &result); err != nil || result["id"] != "vendor/model" {
				t.Fatalf("legacy JSON output=%q err=%v", output.String(), err)
			}
		})
	}
}

func TestLegacyModelMetadataPreservesLocalFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/metadata" || r.URL.Query().Get("refresh") != "1" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"data":[],"count":0}`)
	}))
	defer server.Close()
	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", "", "")
	root.PersistentFlags().String("output", "table", "")
	provider := &cobra.Command{Use: "provider"}
	model := &cobra.Command{Use: "model"}
	metadata := &cobra.Command{Use: modelMetadataCmd.Use, Args: modelMetadataCmd.Args, RunE: modelMetadataCmd.RunE}
	metadata.Flags().Bool("refresh", false, "")
	model.AddCommand(metadata)
	provider.AddCommand(model)
	root.AddCommand(provider)
	root.AddCommand(&cobra.Command{
		Use: "model", DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLegacyModelCommand(cmd, model, args)
		},
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--server", server.URL, "model", "metadata", "--refresh", "--output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute legacy metadata: %v", err)
	}
}
