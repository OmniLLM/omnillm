package shared

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestIsBenignStreamEndError(t *testing.T) {
	live := context.Background()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"nil error", live, nil, true},
		{"http2 body closed", live, errors.New("http2: response body closed"), true},
		{"closed network conn", live, errors.New("use of closed network connection"), true},
		{"read on closed body", live, errors.New("http: read on closed response body"), true},
		{"net.ErrClosed", live, net.ErrClosed, true},
		{"io.ErrClosedPipe", live, io.ErrClosedPipe, true},
		{"context canceled", live, context.Canceled, true},
		{"any error once ctx cancelled", cancelled, errors.New("boom"), true},
		{"genuine upstream reset", live, errors.New("stream error: stream ID 33; INTERNAL_ERROR; received from peer"), false},
		{"genuine read failure", live, io.ErrUnexpectedEOF, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBenignStreamEndError(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("IsBenignStreamEndError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDefaultHTTPTransportConfiguresHTTP2Keepalive(t *testing.T) {
	tr, h2 := defaultHTTPTransport()
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("expected ForceAttemptHTTP2")
	}
	if h2 == nil {
		t.Fatal("expected an HTTP/2 transport")
	}
	if h2.ReadIdleTimeout != 30*time.Second {
		t.Fatalf("ReadIdleTimeout = %v, want 30s", h2.ReadIdleTimeout)
	}
	if h2.PingTimeout != 15*time.Second {
		t.Fatalf("PingTimeout = %v, want 15s", h2.PingTimeout)
	}
}
