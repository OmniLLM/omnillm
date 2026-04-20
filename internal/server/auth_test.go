package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractBearerToken(t *testing.T) {
	token, ok := extractBearerToken("Bearer secret")
	if !ok || token != "secret" {
		t.Fatalf("expected bearer token, got %q ok=%v", token, ok)
	}
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newAuthConfig("secret").middleware())
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}

func TestAuthMiddlewareRejectsSSEQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newAuthConfig("secret").middleware())
	r.GET("/stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/stream?api_key=secret", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	// Query parameter auth is no longer accepted to prevent credential leakage in logs
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}
