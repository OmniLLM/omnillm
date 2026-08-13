package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerationRoutesAcceptBodiesAtLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		setup func(*gin.RouterGroup)
		path  string
	}{
		{name: "chat", path: "/chat/completions", setup: func(group *gin.RouterGroup) { SetupChatCompletionRoutes(group, ChatCompletionOptions{}) }},
		{name: "responses", path: "/responses", setup: SetupResponseRoutes},
		{name: "messages", path: "/messages", setup: SetupMessageRoutes},
		{name: "count_tokens", path: "/messages/count_tokens", setup: SetupMessageRoutes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			group := router.Group("")
			tc.setup(group)
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(strings.Repeat("x", maxGatewayRequestBodyBytes)))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code == http.StatusRequestEntityTooLarge {
				t.Fatalf("body at limit was rejected: %s", recorder.Body.String())
			}
		})
	}
}

func TestGenerationRoutesRejectOversizedBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		setup func(*gin.RouterGroup)
		path  string
	}{
		{name: "chat", path: "/chat/completions", setup: func(group *gin.RouterGroup) { SetupChatCompletionRoutes(group, ChatCompletionOptions{}) }},
		{name: "responses", path: "/responses", setup: SetupResponseRoutes},
		{name: "messages", path: "/messages", setup: SetupMessageRoutes},
		{name: "count_tokens", path: "/messages/count_tokens", setup: SetupMessageRoutes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			group := router.Group("")
			tc.setup(group)
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(strings.Repeat("x", maxGatewayRequestBodyBytes+1)))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected 413, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"type":"invalid_request_error"`) {
				t.Fatalf("expected dialect error envelope, got %s", recorder.Body.String())
			}
		})
	}
}
