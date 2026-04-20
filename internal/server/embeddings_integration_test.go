package server

import (
	"net/http"
	"testing"
)

func TestEmbeddingsEndpointRequiresActiveProvider(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/v1/embeddings", `{"model":"text-embedding-3-small","input":"hello"}`, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}
