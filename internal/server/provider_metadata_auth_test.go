package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"omnillm/internal/routes"
)

func TestProviderMetadataRequiresAdminAuthentication(t *testing.T) {
	router := buildRouter(0, "test-admin-key", routes.ChatCompletionOptions{}, 0)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/providers/any/metadata", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", recorder.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/providers/any/metadata", nil)
	request.Header.Set("Authorization", "Bearer test-admin-key")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("authenticated status=%d, want 404 for nonexistent provider", recorder.Code)
	}
}
