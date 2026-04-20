package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestUsageEndpointRequiresActiveProvider(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/usage")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestUsageEndpointReturnsProviderUsage(t *testing.T) {
	registerStubProvider(t, "gpt-4o", nil, nil)

	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/usage")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["requests"] != float64(0) {
		t.Fatalf("unexpected usage payload: %#v", payload)
	}
}
