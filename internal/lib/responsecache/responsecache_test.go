package responsecache

import (
	"testing"

	"omnillm/internal/cif"
)

func f64(v float64) *float64 { return &v }

func baseReq() *cif.CanonicalRequest {
	sys := "you are helpful"
	return &cif.CanonicalRequest{
		Model:       "gpt-x",
		SystemPrompt: &sys,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
		Temperature: f64(0),
	}
}

func TestCacheable(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*cif.CanonicalRequest)
		want bool
	}{
		{"temp0 non-stream", func(r *cif.CanonicalRequest) {}, true},
		{"streaming now cacheable", func(r *cif.CanonicalRequest) { r.Stream = true }, true},
		{"temp nil", func(r *cif.CanonicalRequest) { r.Temperature = nil }, false},
		{"temp > 0", func(r *cif.CanonicalRequest) { r.Temperature = f64(0.7) }, false},
		{"top_p < 1", func(r *cif.CanonicalRequest) { r.TopP = f64(0.5) }, false},
		{"top_p == 1", func(r *cif.CanonicalRequest) { r.TopP = f64(1) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := baseReq()
			tt.mut(r)
			if got := Cacheable(r); got != tt.want {
				t.Errorf("Cacheable() = %v, want %v", got, tt.want)
			}
		})
	}
	if Cacheable(nil) {
		t.Error("Cacheable(nil) should be false")
	}
}

func TestKeyStability(t *testing.T) {
	a := Key(baseReq())
	b := Key(baseReq())
	if a != b {
		t.Errorf("identical requests produced different keys: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d", len(a))
	}
}

func TestKeySensitivity(t *testing.T) {
	base := Key(baseReq())

	// Different user message ⇒ different key.
	r := baseReq()
	r.Messages = []cif.CIFMessage{
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "bye"}}},
	}
	if Key(r) == base {
		t.Error("different message content should change the key")
	}

	// Different model ⇒ different key.
	r = baseReq()
	r.Model = "gpt-y"
	if Key(r) == base {
		t.Error("different model should change the key")
	}

	// Different max_tokens ⇒ different key.
	r = baseReq()
	mt := 100
	r.MaxTokens = &mt
	if Key(r) == base {
		t.Error("different max_tokens should change the key")
	}
}

func TestKeyIgnoresTransport(t *testing.T) {
	base := Key(baseReq())

	// Headers and UserID must not affect the key.
	r := baseReq()
	uid := "user-123"
	r.UserID = &uid
	r.IncomingHeaders = map[string]string{"x-request-id": "abc"}
	if Key(r) != base {
		t.Error("transport metadata (userId/headers) must not affect the cache key")
	}
}

func TestParseBypass(t *testing.T) {
	cases := map[string]BypassMode{
		"":        BypassNone,
		"bypass":  BypassRead,
		"REFRESH": BypassRead,
		"no-cache": BypassRead,
		"off":     BypassAll,
		"disable": BypassAll,
		"garbage": BypassNone,
	}
	for in, want := range cases {
		if got := ParseBypass(in); got != want {
			t.Errorf("ParseBypass(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	sig := "sig-abc"
	in := 12
	out := 34
	resp := &cif.CanonicalResponse{
		ID:         "resp-1",
		Model:      "gpt-x",
		StopReason: cif.StopReasonToolUse,
		Usage:      &cif.CIFUsage{InputTokens: in, OutputTokens: out},
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "hello world"},
			cif.CIFThinkingPart{Type: "thinking", Thinking: "hmm", Signature: &sig},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "tc1", ToolName: "search", ToolArguments: map[string]interface{}{"q": "go"}},
		},
	}
	encoded, err := encodeResponse(resp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeResponse(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ID != resp.ID || decoded.Model != resp.Model || decoded.StopReason != resp.StopReason {
		t.Fatalf("scalar mismatch: %+v", decoded)
	}
	if decoded.Usage == nil || decoded.Usage.InputTokens != in || decoded.Usage.OutputTokens != out {
		t.Fatalf("usage mismatch: %+v", decoded.Usage)
	}
	if len(decoded.Content) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(decoded.Content))
	}
	txt, ok := decoded.Content[0].(cif.CIFTextPart)
	if !ok || txt.Text != "hello world" {
		t.Fatalf("text part not reconstructed: %#v", decoded.Content[0])
	}
	think, ok := decoded.Content[1].(cif.CIFThinkingPart)
	if !ok || think.Thinking != "hmm" || think.Signature == nil || *think.Signature != sig {
		t.Fatalf("thinking part not reconstructed: %#v", decoded.Content[1])
	}
	tc, ok := decoded.Content[2].(cif.CIFToolCallPart)
	if !ok || tc.ToolName != "search" || tc.ToolArguments["q"] != "go" {
		t.Fatalf("tool_call part not reconstructed: %#v", decoded.Content[2])
	}
}
