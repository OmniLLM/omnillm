package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"omnillm/internal/routes"
	"os"
	"path/filepath"
	"testing"
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

func TestConfigImportAcceptsFileAtLimit(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "config.toml")
	original := routes.ConfigFilePathsForTest()
	routes.SetConfigFilePathsForTest(map[string]string{"codex": target})
	routes.ConfigureSecurityOptions(routes.SecurityOptions{EnableConfigEdit: true})
	defer func() {
		routes.SetConfigFilePathsForTest(original)
		routes.ConfigureSecurityOptions(routes.SecurityOptions{})
	}()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "config.toml")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := io.CopyN(part, zeroReader{}, 4<<20); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/config/codex/import", &body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-api-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post import: %v", err)
	}
	responseBody := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, responseBody)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Size() != 4<<20 {
		t.Fatalf("expected %d bytes, got %d", 4<<20, info.Size())
	}
}

func TestConfigImportRejectsOversizedUploadWithoutOverwriting(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	original := routes.ConfigFilePathsForTest()
	routes.SetConfigFilePathsForTest(map[string]string{"codex": target})
	routes.ConfigureSecurityOptions(routes.SecurityOptions{EnableConfigEdit: true})
	defer func() {
		routes.SetConfigFilePathsForTest(original)
		routes.ConfigureSecurityOptions(routes.SecurityOptions{})
	}()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "config.toml")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := io.CopyN(part, zeroReader{}, 4<<20+1); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/config/codex/import", &body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-api-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post import: %v", err)
	}
	responseBody := readBody(t, resp)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", resp.StatusCode, responseBody)
	}
	stored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(stored) != "original" {
		t.Fatalf("oversized import modified config: %q", stored)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
