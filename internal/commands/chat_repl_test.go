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

func TestHandleChatSlashCommandHelp(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	result, err := handleChatSlashCommand(cmd, &Client{}, &chatSessionState{ID: "session-1"}, "/help")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if !result.handled || result.exit {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(out.String(), "/model <id>") {
		t.Fatalf("help output missing model command:\n%s", out.String())
	}
}

func TestHandleChatSlashCommandUnknown(t *testing.T) {
	cmd := &cobra.Command{}
	_, err := handleChatSlashCommand(cmd, &Client{}, &chatSessionState{ID: "session-1"}, "/wat")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestListChatModelsSorted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "z-model", "owned_by": "provider-b", "display_name": "Zed"},
				{"id": "a-model", "owned_by": "provider-a", "display_name": "Alpha"},
			},
		})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, http: server.Client()}
	models, err := listChatModels(client)
	if err != nil {
		t.Fatalf("listChatModels returned error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "a-model" || models[1].ID != "z-model" {
		t.Fatalf("models not sorted by ID: %+v", models)
	}
}

func TestHandleChatSlashCommandModelSwitch(t *testing.T) {
	var updatedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			updatedModel = body["model_id"]
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	client := &Client{BaseURL: server.URL, http: server.Client()}

	result, err := handleChatSlashCommand(cmd, client, &chatSessionState{ID: "session-1"}, "/model qwen3")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if !result.handled || result.model != "qwen3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updatedModel != "qwen3" {
		t.Fatalf("updated model = %q, want qwen3", updatedModel)
	}
	if !strings.Contains(out.String(), "Switched model to qwen3") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
