package shared

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
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
	tr := DefaultHTTPTransport()
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("expected ForceAttemptHTTP2")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("expected ConfigureTransports to install a TLS config for h2")
	}
	var hasH2 bool
	for _, p := range tr.TLSClientConfig.NextProtos {
		if p == "h2" {
			hasH2 = true
		}
	}
	if !hasH2 {
		t.Fatalf("expected h2 in NextProtos, got %v", tr.TLSClientConfig.NextProtos)
	}
}
