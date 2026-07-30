package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// parseRequestMessage returns a user-facing message for a request body parse
// error.  JSON syntax/EOF errors produce the generic "Invalid request format"
// string (same as the old json.Valid guard) so callers get a clean message
// without the unmarshal detail.  Semantic errors (wrong field types, missing
// required fields) pass through so they are actionable.
func parseRequestMessage(err error) string {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "Invalid request format"
	}
	msg := err.Error()
	if strings.Contains(msg, "unexpected end of JSON") ||
		strings.Contains(msg, "invalid character") {
		return "Invalid request format"
	}
	return "Failed to parse request: " + msg
}

func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	var authErr interface{ IsAuthenticationError() bool }
	if errors.As(err, &authErr) && authErr.IsAuthenticationError() {
		return true
	}

	var statusErr interface{ StatusCode() int }
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "token expired") ||
		strings.Contains(msg, "unauthorized")
}

// isClientCanceled reports whether err is the result of the *client* going away
// (browser tab closed, editor cancelled the request, Ctrl-C in a CLI) rather
// than an actual provider failure.  Both conditions matter: the error itself
// must be a cancellation, and the inbound request context must actually be
// done.  Checking only the error would misclassify an upstream-initiated
// cancellation; checking only the context would misclassify an unrelated
// provider error that happened to race with a disconnect.
func isClientCanceled(c *gin.Context, err error) bool {
	if err == nil || c == nil || c.Request == nil {
		return false
	}
	if !errors.Is(err, context.Canceled) {
		return false
	}
	return errors.Is(c.Request.Context().Err(), context.Canceled)
}

func shouldFallbackToNonStreaming(err error) bool {
	return err != nil && !isAuthenticationError(err)
}

func providerFailureStatus(err error) int {
	if isAuthenticationError(err) {
		return http.StatusUnauthorized
	}
	return http.StatusBadGateway
}

func providerFailureType(defaultType string, err error) string {
	if isAuthenticationError(err) {
		return "authentication_error"
	}
	return defaultType
}
