package shared

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
)

// IsBenignStreamEndError reports whether err is the expected consequence of a
// stream being torn down deliberately rather than a genuine upstream failure.
//
// Every SSE parser closes the response body from a context.AfterFunc when the
// request context is cancelled (client disconnect, tool-loop turn completing,
// caller abandoning the read). That close races with the in-flight Read, so the
// scanner surfaces an error such as "http2: response body closed" or
// "use of closed network connection". Those are not upstream problems and must
// not be logged at error level or forwarded to the client as a stream_error.
func IsBenignStreamEndError(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
		return true
	}

	// The body was closed by our own AfterFunc because the caller went away.
	if ctx != nil && ctx.Err() != nil {
		return true
	}

	// net/http and golang.org/x/net/http2 report these as unexported error
	// values, so string matching is the only option available.
	msg := err.Error()
	for _, benign := range []string{
		"http2: response body closed",
		"http: read on closed response body",
		"use of closed network connection",
	} {
		if strings.Contains(msg, benign) {
			return true
		}
	}

	return false
}
