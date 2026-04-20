package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"omnillm/internal/routes"
)

func TestConfigFilesEndpointListsConfiguredEntries(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/api/admin/config")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Configs []struct {
			Name string `json:"name"`
		} `json:"configs"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Configs) == 0 {
		t.Fatal("expected config entries")
	}
}

func TestConfigFilesEndpointReturnsMissingFileMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	original := routes.ConfigFilePathsForTest()
	routes.SetConfigFilePathsForTest(map[string]string{
		"codex": filepath.Join(tmpDir, "config.toml"),
	})
	defer routes.SetConfigFilePathsForTest(original)

	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/api/admin/config/codex")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"exists":false`)) {
		t.Fatalf("expected missing file response, got %s", body)
	}
}
