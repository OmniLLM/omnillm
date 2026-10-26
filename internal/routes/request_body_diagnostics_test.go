package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Return data and an error together, as permitted by io.Reader.
type partialFailureReader struct {
	data   string
	err    error
	cancel context.CancelFunc
}

func (r *partialFailureReader) Read(p []byte) (int, error) {
	if r.cancel != nil {
		r.cancel()
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestReadGatewayRequestBodyRetainsFailureEvidence(t *testing.T) {
	cause := fmt.Errorf("wrapped: %w", io.ErrUnexpectedEOF)
	body, err := readGatewayRequestBody(&partialFailureReader{data: "partial", err: cause})
	if body != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("body=%q error=%v", body, err)
	}
	var failure *requestBodyReadError
	if !errors.As(err, &failure) || failure.bytesRead != 7 {
		t.Fatalf("missing partial byte count: %v", err)
	}
}

func TestRequestBodyFailureDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := []struct {
		path, api, errorType string
		setup                func(*gin.RouterGroup)
	}{
		{"/chat/completions", "openai", "invalid_request_error", func(g *gin.RouterGroup) { SetupChatCompletionRoutes(g, ChatCompletionOptions{}) }},
		{"/responses", "responses", "invalid_request_error", SetupResponseRoutes},
		{"/messages", "anthropic", "invalid_request_error", SetupMessageRoutes},
		{"/messages/count_tokens", "anthropic", "invalid_request_error", SetupMessageRoutes},
		{"/systemone", "systemone", "systemone_error", SetupSystemOneRoutes},
	}
	cases := []struct {
		name, reason, level string
		err                 error
		cancel              bool
		size                int
		length              int64
		status              int
	}{
		{"truncated", "unexpected_eof", "warn", fmt.Errorf("ERROR_SECRET: %w", io.ErrUnexpectedEOF), false, 11, 100, 400},
		{"chunked", "unexpected_eof", "warn", io.ErrUnexpectedEOF, true, 11, -1, 400},
		{"canceled", "canceled", "info", fmt.Errorf("wrapped: %w", context.Canceled), true, 11, 100, 400},
		{"unrelated canceled", "read_error", "error", errors.New("ERROR_SECRET"), true, 11, 100, 400},
		{"cancellation on live context", "read_error", "error", context.Canceled, false, 11, 100, 400},
		{"oversized", "too_large", "warn", nil, false, maxGatewayRequestBodyBytes + 1, maxGatewayRequestBodyBytes + 1, 413},
	}
	old := log.Logger
	t.Cleanup(func() { log.Logger = old })
	for _, route := range routes {
		for _, tc := range cases {
			t.Run(route.path+"/"+tc.name, func(t *testing.T) {
				var output bytes.Buffer
				log.Logger = zerolog.New(&output)
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set("request_id", "upload-test"); c.Next() })
				route.setup(router.Group(""))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				reader := &partialFailureReader{data: "BODY_SECRET", err: tc.err}
				if tc.cancel {
					reader.cancel = cancel
				}
				var body io.Reader = reader
				if tc.err == nil {
					body = strings.NewReader(strings.Repeat("x", tc.size))
				}
				req := httptest.NewRequest(http.MethodPost, route.path+"?token=QUERY_SECRET", body).WithContext(ctx)
				req.ContentLength = tc.length
				req.Header.Set("Authorization", "Bearer AUTH_SECRET")
				req.Header.Set("User-Agent", strings.Repeat("x", 511)+"中文")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != tc.status || !strings.Contains(rec.Body.String(), `"type":"`+route.errorType+`"`) {
					t.Fatalf("response %d %s", rec.Code, rec.Body.String())
				}
				lines := strings.Split(strings.TrimSpace(output.String()), "\n")
				if len(lines) != 1 {
					t.Fatalf("expected one diagnostic, got %q", output.String())
				}
				var event map[string]any
				if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
					t.Fatal(err)
				}
				state := "active"
				if tc.cancel {
					state = "canceled"
				}
				want := map[string]any{"level": tc.level, "reason": tc.reason, "request_id": "upload-test", "api_shape": route.api, "method": "POST", "path": route.path, "bytes_read": float64(tc.size), "content_length": float64(tc.length), "context_state": state}
				for k, v := range want {
					if event[k] != v {
						t.Errorf("%s=%v want %v", k, event[k], v)
					}
				}
				ua, _ := event["user_agent"].(string)
				if len(ua) > 512 || !utf8.ValidString(ua) || ua != strings.Repeat("x", 511) {
					t.Errorf("invalid user agent: %q", ua)
				}
				if event["client"] == nil {
					t.Error("missing client")
				}
				for _, secret := range []string{"BODY_SECRET", "ERROR_SECRET", "QUERY_SECRET", "AUTH_SECRET"} {
					if strings.Contains(output.String(), secret) {
						t.Errorf("leaked %s", secret)
					}
				}
			})
		}
	}
}
