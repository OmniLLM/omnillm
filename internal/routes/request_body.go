package routes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const maxGatewayRequestBodyBytes = 16 << 20

var errRequestBodyTooLarge = errors.New("request body too large")

func gatewayRequestBodyError(err error) (int, string) {
	if errors.Is(err, errRequestBodyTooLarge) {
		return http.StatusRequestEntityTooLarge, err.Error()
	}
	return http.StatusBadRequest, "Invalid request format"
}

func readGatewayRequestBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxGatewayRequestBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, &requestBodyReadError{cause: err, bytesRead: len(data)}
	}
	if len(data) > maxGatewayRequestBodyBytes {
		return nil, &requestBodyReadError{
			cause:     fmt.Errorf("%w: maximum size is %d bytes", errRequestBodyTooLarge, maxGatewayRequestBodyBytes),
			bytesRead: len(data),
		}
	}
	return data, nil
}

// requestBodyReadError retains evidence without making partial payloads available
// to callers. Unwrap preserves the existing status mapping and error matching.
type requestBodyReadError struct {
	cause     error
	bytesRead int
}

func (e *requestBodyReadError) Error() string { return e.cause.Error() }
func (e *requestBodyReadError) Unwrap() error { return e.cause }

func logRequestBodyFailure(c *gin.Context, apiShape string, err error) {
	level, reason := zerolog.ErrorLevel, "read_error"
	switch {
	case errors.Is(err, errRequestBodyTooLarge):
		level, reason = zerolog.WarnLevel, "too_large"
	case errors.Is(err, io.ErrUnexpectedEOF):
		level, reason = zerolog.WarnLevel, "unexpected_eof"
	case isClientCanceled(c, err):
		level, reason = zerolog.InfoLevel, "canceled"
	}
	contextState := "active"
	switch c.Request.Context().Err() {
	case context.Canceled:
		contextState = "canceled"
	case context.DeadlineExceeded:
		contextState = "deadline_exceeded"
	}
	bytesRead := 0
	var failure *requestBodyReadError
	if errors.As(err, &failure) {
		bytesRead = failure.bytesRead
	}
	// User-Agent is untrusted metadata; normalize invalid UTF-8 and bound its size.
	userAgent := c.GetHeader("User-Agent")
	userAgent = strings.ToValidUTF8(userAgent, "�")
	if len(userAgent) > 512 {
		end := 512
		for !utf8.RuneStart(userAgent[end]) {
			end--
		}
		userAgent = userAgent[:end]
	}
	// Never attach the arbitrary reader error or partial payload to this record.
	log.WithLevel(level).
		Str("request_id", c.GetString("request_id")).
		Str("api_shape", apiShape).
		Str("reason", reason).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Str("client", c.ClientIP()).
		Str("user_agent", userAgent).
		Int("bytes_read", bytesRead).
		Int64("content_length", c.Request.ContentLength).
		Str("context_state", contextState).
		Msg("Request body read failed")
}
