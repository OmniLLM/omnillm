package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestProviderCanonicalCommandHierarchy(t *testing.T) {
	want := map[string]bool{"login": false, "model": false}
	for _, command := range ProviderCmd.Commands() {
		if _, ok := want[command.Name()]; ok {
			want[command.Name()] = true
		}
	}
	for name, present := range want {
		if !present {
			t.Fatalf("provider command missing %q", name)
		}
	}
	if !providerAddCmd.Hidden || !AuthCmd.Hidden || !LegacyModelCmd.Hidden {
		t.Fatal("legacy provider add, auth, and model commands must be hidden")
	}
	if providerLoginCmd.Flags().Lookup("new") == nil {
		t.Fatal("provider login missing --new")
	}
	if providerRenameCmd.Flags().Lookup("alias") == nil {
		t.Fatal("provider rename missing --alias")
	}
}

func TestLegacyModelHelp(t *testing.T) {
	for _, helpArg := range []string{"--help", "-h"} {
		t.Run(helpArg, func(t *testing.T) {
			var output bytes.Buffer
			LegacyModelCmd.SetOut(&output)
			if err := executeLegacyModelCommand(LegacyModelCmd, ModelCmd, []string{helpArg}); err != nil {
				t.Fatalf("execute legacy model help: %v", err)
			}
			if !strings.Contains(output.String(), "Manage models for a provider") {
				t.Fatalf("legacy model help output = %q", output.String())
			}
		})
	}
}

func TestLoginProviderUsesNormalizedEndpoint(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/admin/providers/login" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"complete","provider_id":"provider-id","is_new":false}`))
	}))
	defer server.Close()
	t.Setenv("OMNILLM_SERVER", server.URL)

	command := &cobra.Command{Use: "test"}
	addProviderAuthFlags(command)
	var output bytes.Buffer
	command.SetOut(&output)
	if err := loginProvider(command, "Friendly Provider", false); err != nil {
		t.Fatalf("loginProvider: %v", err)
	}
	if requestBody["subject"] != "Friendly Provider" {
		t.Fatalf("subject=%v", requestBody["subject"])
	}
	if requestBody["type"] != nil {
		t.Fatalf("unexpected type=%v", requestBody["type"])
	}
	if !bytes.Contains(output.Bytes(), []byte("provider-id")) {
		t.Fatalf("output=%q", output.String())
	}
}

func TestProviderTypeForLoginSubjectUsesIDAliasAndName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"id":"google-one","type":"google","name":"Friendly Google","alias":"short","subtitle":"short"}]`))
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, http: server.Client()}
	for _, subject := range []string{"google-one", "SHORT", "friendly google"} {
		providerType, err := providerTypeForLoginSubject(client, subject)
		if err != nil || providerType != "google" {
			t.Fatalf("subject %q: type=%q err=%v", subject, providerType, err)
		}
	}
}

func TestResolveProviderRenameFieldsPromptsForNameAndAlias(t *testing.T) {
	command := &cobra.Command{Use: "test"}
	command.SetIn(strings.NewReader("  Friendly Copilot  \n  copilot-jzhu  \n"))
	var prompts bytes.Buffer
	command.SetErr(&prompts)

	name, alias, err := resolveProviderRenameFields(command, "", "", "", true)
	if err != nil {
		t.Fatalf("resolveProviderRenameFields: %v", err)
	}
	if name != "Friendly Copilot" || alias != "copilot-jzhu" {
		t.Fatalf("name=%q alias=%q", name, alias)
	}
	for _, label := range []string{"New display name", "New provider alias"} {
		if !strings.Contains(prompts.String(), label) {
			t.Fatalf("prompt output %q missing %q", prompts.String(), label)
		}
	}
}

func TestResolveProviderRenameFieldsRejectsEmptyPromptValues(t *testing.T) {
	command := &cobra.Command{Use: "test"}
	command.SetIn(strings.NewReader("  \n\t\n"))
	command.SetErr(&bytes.Buffer{})

	_, _, err := resolveProviderRenameFields(command, "", "", "", true)
	if err == nil || !strings.Contains(err.Error(), "at least one of --name or --alias is required") {
		t.Fatalf("error=%v", err)
	}
}

func TestResolveProviderRenameFieldsUsesFlagsWithoutPrompting(t *testing.T) {
	command := &cobra.Command{Use: "test"}
	command.SetIn(strings.NewReader("unexpected prompt input\n"))
	var prompts bytes.Buffer
	command.SetErr(&prompts)

	name, alias, err := resolveProviderRenameFields(command, "Explicit Name", "explicit-alias", "", true)
	if err != nil {
		t.Fatalf("resolveProviderRenameFields: %v", err)
	}
	if name != "Explicit Name" || alias != "explicit-alias" {
		t.Fatalf("name=%q alias=%q", name, alias)
	}
	if prompts.Len() != 0 {
		t.Fatalf("unexpected prompt output %q", prompts.String())
	}
}

func TestResolveProviderRenameFieldsPreservesSubtitleCompatibility(t *testing.T) {
	t.Run("uses subtitle as alias", func(t *testing.T) {
		name, alias, err := resolveProviderRenameFields(&cobra.Command{}, "", "", "legacy-alias", false)
		if err != nil {
			t.Fatalf("resolveProviderRenameFields: %v", err)
		}
		if name != "" || alias != "legacy-alias" {
			t.Fatalf("name=%q alias=%q", name, alias)
		}
	})

	t.Run("rejects conflicting alias and subtitle", func(t *testing.T) {
		_, _, err := resolveProviderRenameFields(&cobra.Command{}, "", "new-alias", "legacy-alias", false)
		if err == nil || !strings.Contains(err.Error(), "--alias and --subtitle must match") {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestResolveProviderRenameFieldsNonInteractiveRequiresFlags(t *testing.T) {
	command := &cobra.Command{Use: "test"}
	command.SetIn(strings.NewReader("must not be read\n"))
	var prompts bytes.Buffer
	command.SetErr(&prompts)

	_, _, err := resolveProviderRenameFields(command, "", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "at least one of --name or --alias is required") {
		t.Fatalf("error=%v", err)
	}
	if prompts.Len() != 0 {
		t.Fatalf("unexpected prompt output %q", prompts.String())
	}
}

func TestRunProviderRenameInteractiveSubmitsPromptedFields(t *testing.T) {
	var requestCount int
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Method != http.MethodPatch {
			t.Errorf("method=%q", request.Method)
		}
		if request.URL.Path != "/api/admin/providers/copilot-jzhu-abk/name" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"copilot-jzhu-abk"}`))
	}))
	defer server.Close()
	t.Setenv("OMNILLM_SERVER", server.URL)

	command := newProviderRenameTestCommand()
	command.SetIn(strings.NewReader("Friendly Copilot\ncopilot-jzhu\n"))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})

	if err := runProviderRename(command, []string{"copilot-jzhu-abk"}, true); err != nil {
		t.Fatalf("runProviderRename: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count=%d", requestCount)
	}
	if requestBody["name"] != "Friendly Copilot" || requestBody["alias"] != "copilot-jzhu" {
		t.Fatalf("request body=%v", requestBody)
	}
	if !strings.Contains(output.String(), "Provider 'copilot-jzhu-abk' renamed") {
		t.Fatalf("output=%q", output.String())
	}
}

func TestRunProviderRenameEmptyInteractiveValuesSendsNoRequest(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		http.Error(writer, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("OMNILLM_SERVER", server.URL)

	command := newProviderRenameTestCommand()
	command.SetIn(strings.NewReader("\n\n"))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	err := runProviderRename(command, []string{"copilot-jzhu-abk"}, true)
	if err == nil || !strings.Contains(err.Error(), "at least one of --name or --alias is required") {
		t.Fatalf("error=%v", err)
	}
	if requestCount != 0 {
		t.Fatalf("request count=%d", requestCount)
	}
}

func newProviderRenameTestCommand() *cobra.Command {
	command := &cobra.Command{Use: "rename"}
	command.Flags().String("name", "", "")
	command.Flags().String("alias", "", "")
	command.Flags().String("subtitle", "", "")
	return command
}
