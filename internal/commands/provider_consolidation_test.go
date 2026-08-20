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
