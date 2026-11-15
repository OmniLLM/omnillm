package server

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"omnillm/internal/cif"
)

type fallbackPolicyError struct {
	allowed bool
}

func (e fallbackPolicyError) Error() string { return "synthetic upstream request rejected" }
func (e fallbackPolicyError) NonStreamingFallbackAllowed() bool {
	return e.allowed
}

func TestStreamingRoutesHonorWrappedFallbackPolicy(t *testing.T) {
	for _, shape := range []struct {
		name     string
		endpoint string
		body     string
	}{
		{
			name: "chat", endpoint: "/v1/chat/completions",
			body: `{"model":"%s","stream":true,"messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "messages", endpoint: "/v1/messages",
			body: `{"model":"%s","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "responses", endpoint: "/v1/responses",
			body: `{"model":"%s","stream":true,"input":"hello"}`,
		},
	} {
		for _, mode := range []struct {
			name    string
			allowed bool
		}{
			{name: "disallowed"},
			{name: "allowed", allowed: true},
		} {
			t.Run(shape.name+"/"+mode.name, func(t *testing.T) {
				var streamCalls, collectedCalls atomic.Int32
				model := "fallback-policy-" + shape.name + "-" + mode.name
				registerStubProvider(
					t,
					model,
					func(_ context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
						collectedCalls.Add(1)
						return &cif.CanonicalResponse{
							ID: "fallback-result", Model: request.Model,
							Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "OK"}},
							StopReason: cif.StopReasonEndTurn,
						}, nil
					},
					func(context.Context, *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
						streamCalls.Add(1)
						return nil, fmt.Errorf("adapter: %w", fallbackPolicyError{allowed: mode.allowed})
					},
				)
				srv := newTestServer(t)
				defer srv.Close()
				response := postJSON(
					t,
					srv.URL+shape.endpoint,
					fmt.Sprintf(shape.body, model),
					nil,
				)
				body := readBody(t, response)
				wantStatus := http.StatusBadGateway
				var wantCollected int32
				if mode.allowed {
					wantStatus = http.StatusOK
					wantCollected = 1
				}
				if response.StatusCode != wantStatus {
					t.Fatalf("status=%d, want %d: %s", response.StatusCode, wantStatus, body)
				}
				if streamCalls.Load() != 1 || collectedCalls.Load() != wantCollected {
					t.Fatalf("stream calls=%d collected calls=%d", streamCalls.Load(), collectedCalls.Load())
				}
			})
		}
	}
}
