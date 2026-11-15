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

func providerStateTestRoot(serverURL, output string) (*cobra.Command, *bytes.Buffer) {
	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", serverURL, "")
	root.PersistentFlags().String("api-key", "test-admin-key", "")
	root.PersistentFlags().String("output", output, "")
	provider := &cobra.Command{Use: "provider", RunE: ProviderCmd.RunE}
	for _, command := range []*cobra.Command{providerActivateCmd, providerDeactivateCmd} {
		provider.AddCommand(&cobra.Command{
			Use: command.Use, Aliases: command.Aliases, Short: command.Short,
			Args: command.Args, RunE: command.RunE, ValidArgsFunction: command.ValidArgsFunction,
		})
	}
	root.AddCommand(provider)
	var result bytes.Buffer
	root.SetOut(&result)
	root.SetErr(&result)
	return root, &result
}

func TestProviderStateHelp(t *testing.T) {
	root, output := providerStateTestRoot("", "table")
	root.SetArgs([]string{"provider", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"enable", "disable"} {
		if !strings.Contains(output.String(), "  "+name+" ") {
			t.Errorf("help missing %q: %s", name, output)
		}
	}
	for _, name := range []string{"activate", "deactivate"} {
		if strings.Contains(output.String(), "  "+name+" ") {
			t.Errorf("help lists legacy command %q: %s", name, output)
		}
	}
}

func TestProviderStateCommands(t *testing.T) {
	for _, test := range []struct {
		name, command, path, message string
	}{
		{name: "enable", command: "enable", path: "activate", message: "enabled"},
		{name: "disable", command: "disable", path: "deactivate", message: "disabled"},
	} {
		for _, mode := range []string{"table", "json"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if r.Method != http.MethodPost || r.URL.Path != "/api/admin/providers/my-alias/"+test.path {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					if r.Header.Get("Authorization") != "Bearer test-admin-key" {
						t.Error("missing admin authorization")
					}
					fmt.Fprint(w, `{"success":true}`)
				}))
				defer server.Close()
				root, output := providerStateTestRoot(server.URL, mode)
				root.SetArgs([]string{"provider", test.command, "my-alias"})
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				if requests != 1 {
					t.Fatalf("got %d requests, want one", requests)
				}
				if mode == "json" {
					var result map[string]bool
					if err := json.Unmarshal(output.Bytes(), &result); err != nil || !result["success"] {
						t.Fatalf("unexpected JSON %q: %v", output, err)
					}
				} else if !strings.Contains(output.String(), test.message) {
					t.Errorf("output %q does not contain %q", output, test.message)
				}
			})
		}
	}
}

func TestProviderStateValidationAndCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/providers" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `[{"id":"first"},{"id":"second"}]`)
	}))
	defer server.Close()
	for _, name := range []string{"enable", "disable"} {
		t.Run(name, func(t *testing.T) {
			root, _ := providerStateTestRoot(server.URL, "table")
			command, _, err := root.Find([]string{"provider", name})
			if err != nil || command.ValidArgsFunction == nil {
				t.Fatalf("completion unavailable: %v", err)
			}
			values, directive := command.ValidArgsFunction(command, nil, "")
			if len(values) != 2 || values[0] != "first" || values[1] != "second" || directive != cobra.ShellCompDirectiveNoFileComp {
				t.Fatalf("completion = %v, %v", values, directive)
			}
			root.SetArgs([]string{"provider", name})
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "provider ID is required") {
				t.Fatalf("missing reference error = %v", err)
			}
			root, _ = providerStateTestRoot(server.URL, "table")
			root.SetArgs([]string{"provider", name, "one", "two"})
			if err := root.Execute(); err == nil {
				t.Fatal("expected too many arguments to fail")
			}
		})
	}
}

func TestProviderLegacyStateCommandsRejected(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, name := range []string{"activate", "deactivate"} {
		t.Run(name, func(t *testing.T) {
			root, _ := providerStateTestRoot(server.URL, "table")
			root.SetArgs([]string{"provider", name, "my-alias"})
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("expected unknown command error, got %v", err)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("legacy command sent %d requests", got)
			}
		})
	}
}
