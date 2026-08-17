package responsecache

import (
	"testing"

	"omnillm/internal/cif"
)

func f64(v float64) *float64 { return &v }

func mustKey(t testing.TB, req *cif.CanonicalRequest) string {
	t.Helper()
	key, err := Key(req)
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if key == "" {
		t.Fatal("Key returned an empty key without error")
	}
	return key
}

func baseReq() *cif.CanonicalRequest {
	sys := "you are helpful"
	return &cif.CanonicalRequest{
		Model:  "gpt-x",
		System: cif.SystemBlocksFromText(sys),
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
		Temperature: f64(0),
	}
}

func TestKeyAcceptsSamplingAndStreamingVariants(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*cif.CanonicalRequest)
	}{
		{"temp0 non-stream", func(r *cif.CanonicalRequest) {}},
		{"streaming", func(r *cif.CanonicalRequest) { r.Stream = true }},
		{"temp nil", func(r *cif.CanonicalRequest) { r.Temperature = nil }},
		{"temp > 0", func(r *cif.CanonicalRequest) { r.Temperature = f64(0.7) }},
		{"top_p < 1", func(r *cif.CanonicalRequest) { r.TopP = f64(0.5) }},
		{"top_p == 1", func(r *cif.CanonicalRequest) { r.TopP = f64(1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := baseReq()
			tt.mut(r)
			mustKey(t, r)
		})
	}
}

func TestKeyStability(t *testing.T) {
	a := mustKey(t, baseReq())
	b := mustKey(t, baseReq())
	if a != b {
		t.Errorf("identical requests produced different keys: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d", len(a))
	}
}

func TestKeySensitivity(t *testing.T) {
	base := mustKey(t, baseReq())
	cases := []struct {
		name string
		mut  func(*cif.CanonicalRequest)
	}{
		{"messages", func(r *cif.CanonicalRequest) {
			r.Messages = []cif.CIFMessage{cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "bye"}}}}
		}},
		{"model", func(r *cif.CanonicalRequest) { r.Model = "gpt-y" }},
		{"system", func(r *cif.CanonicalRequest) { r.System[0].Text = "different" }},
		{"tools", func(r *cif.CanonicalRequest) {
			r.Tools = []cif.CIFTool{{Name: "lookup", ParametersSchema: map[string]interface{}{"type": "object"}}}
		}},
		{"tool choice", func(r *cif.CanonicalRequest) { r.ToolChoice = map[string]interface{}{"type": "required"} }},
		{"temperature", func(r *cif.CanonicalRequest) { r.Temperature = f64(0.8) }},
		{"top p", func(r *cif.CanonicalRequest) { r.TopP = f64(0.5) }},
		{"max tokens", func(r *cif.CanonicalRequest) { value := 100; r.MaxTokens = &value }},
		{"stop", func(r *cif.CanonicalRequest) { r.Stop = []string{"done"} }},
		{"response format", func(r *cif.CanonicalRequest) { r.ResponseFormat = map[string]interface{}{"type": "json_object"} }},
		{"previous response", func(r *cif.CanonicalRequest) { value := "resp-previous"; r.PreviousResponseID = &value }},
		{"thinking budget", func(r *cif.CanonicalRequest) {
			value := 1024
			r.Extensions = &cif.Extensions{ThinkingBudgetTokens: &value}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := baseReq()
			tc.mut(req)
			if got := mustKey(t, req); got == base {
				t.Fatalf("%s should change the key", tc.name)
			}
		})
	}
}

func TestKeyIgnoresOperationalAndPromptCacheControls(t *testing.T) {
	base := baseReq()
	base.Tools = []cif.CIFTool{{Name: "lookup", ParametersSchema: map[string]interface{}{"type": "object"}}}
	base.Messages = []cif.CIFMessage{
		cif.CIFSystemMessage{Role: "system", Content: []cif.CIFSystemBlock{{Type: "text", Text: "nested system"}}},
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "text"},
			cif.CIFImagePart{Type: "image", MediaType: "image/png", URL: stringPointer("https://example.invalid/image.png")},
			cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call", ToolName: "lookup", Content: "result"},
		}},
		cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "thought"},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "lookup", ToolArguments: map[string]interface{}{}},
		}},
	}
	want := mustKey(t, base)

	changed := baseReq()
	*changed = *base
	changed.System = append([]cif.CIFSystemBlock(nil), base.System...)
	changed.Tools = append([]cif.CIFTool(nil), base.Tools...)
	changed.Messages = []cif.CIFMessage{
		cif.CIFSystemMessage{Role: "system", Content: []cif.CIFSystemBlock{{Type: "text", Text: "nested system", CacheControl: testCacheControl(cif.CIFCacheTTL5m)}}},
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: "text", CacheControl: testCacheControl(cif.CIFCacheTTL1h)},
			cif.CIFImagePart{Type: "image", MediaType: "image/png", URL: stringPointer("https://example.invalid/image.png"), CacheControl: testCacheControl(cif.CIFCacheTTL5m)},
			cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call", ToolName: "lookup", Content: "result", CacheControl: testCacheControl(cif.CIFCacheTTL1h)},
		}},
		cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{
			cif.CIFThinkingPart{Type: "thinking", Thinking: "thought", CacheControl: testCacheControl(cif.CIFCacheTTL5m)},
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "lookup", ToolArguments: map[string]interface{}{}, CacheControl: testCacheControl(cif.CIFCacheTTL1h)},
		}},
	}
	changed.System[0].CacheControl = testCacheControl(cif.CIFCacheTTL1h)
	changed.Tools[0].CacheControl = testCacheControl(cif.CIFCacheTTL5m)
	promptKey, retention, shape := "provider-key", "24h", "responses"
	truth := true
	changed.PromptCache = &cif.CIFPromptCacheRequest{Automatic: testCacheControl(cif.CIFCacheTTL1h), Key: &promptKey, Retention: &retention}
	changed.Stream = true
	changed.IncomingHeaders = map[string]string{"x-request-id": "id"}
	changed.Extensions = &cif.Extensions{
		InboundAPIShape:            &shape,
		RequiresDummyToolInjection: &truth,
		ForceChatCompletions:       &truth,
		DisableAuthRetry:           &truth,
		DisableStreamingFallback:   &truth,
	}

	if got := mustKey(t, changed); got != want {
		t.Fatalf("operational/prompt cache metadata changed key: got %s, want %s", got, want)
	}
	if base.System[0].CacheControl != nil || base.Tools[0].CacheControl != nil {
		t.Fatal("key projection mutated the source request")
	}
}

func TestKeyIncludesForwardedUserIdentity(t *testing.T) {
	first := baseReq()
	first.UserID = stringPointer("user-a")
	second := baseReq()
	second.UserID = stringPointer("user-b")
	if mustKey(t, first) == mustKey(t, second) {
		t.Fatal("different forwarded user identities produced the same key")
	}
}

func TestKeyFailsClosedOnMarshal(t *testing.T) {
	req := baseReq()
	req.ResponseFormat = map[string]interface{}{"unsupported": make(chan int)}
	key, err := Key(req)
	if err == nil {
		t.Fatal("Key should fail when semantic material cannot be marshaled")
	}
	if key != "" {
		t.Fatalf("Key returned usable key %q on marshal failure", key)
	}
	if key, err := Key(nil); err == nil || key != "" {
		t.Fatalf("Key(nil) = %q, %v; want empty key and error", key, err)
	}
}

func testCacheControl(ttl cif.CIFCacheTTL) *cif.CIFCacheControl {
	return &cif.CIFCacheControl{Type: "ephemeral", TTL: &ttl}
}

func TestParseBypass(t *testing.T) {
	cases := map[string]BypassMode{
		"":         BypassNone,
		"bypass":   BypassRead,
		"REFRESH":  BypassRead,
		"no-cache": BypassRead,
		"off":      BypassAll,
		"disable":  BypassAll,
		"garbage":  BypassNone,
	}
	for in, want := range cases {
		if got := ParseBypass(in); got != want {
			t.Errorf("ParseBypass(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	sig := "sig-abc"
	rawInput := "line one\nline two"
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
			cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "tc2", ToolName: "shell", ToolArguments: map[string]interface{}{"input": rawInput}, ToolKind: cif.CIFToolKindCustom, RawInput: &rawInput, Namespace: "computer"},
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
	if len(decoded.Content) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(decoded.Content))
	}
	txt, ok := decoded.Content[0].(cif.CIFTextPart)
	if !ok || txt.Text != "hello world" {
		t.Fatalf("text part not reconstructed: %#v", decoded.Content[0])
	}
	think, ok := decoded.Content[1].(cif.CIFThinkingPart)
	if !ok || think.Thinking != "hmm" || think.Signature == nil || *think.Signature != sig {
		t.Fatalf("thinking part not reconstructed: %#v", decoded.Content[1])
	}
	function, ok := decoded.Content[2].(cif.CIFToolCallPart)
	if !ok || function.ToolName != "search" || function.ToolArguments["q"] != "go" || function.ToolKind != "" || function.RawInput != nil || function.Namespace != "" {
		t.Fatalf("function tool_call part not reconstructed: %#v", decoded.Content[2])
	}
	custom, ok := decoded.Content[3].(cif.CIFToolCallPart)
	if !ok || custom.ToolCallID != "tc2" || custom.ToolName != "shell" || custom.ToolKind != cif.CIFToolKindCustom || custom.RawInput == nil || *custom.RawInput != rawInput || custom.Namespace != "computer" || custom.ToolArguments["input"] != rawInput {
		t.Fatalf("custom tool_call part not reconstructed: %#v", decoded.Content[3])
	}
}

func TestEncodeDecodePreservesPresentEmptyRawInput(t *testing.T) {
	empty := ""
	encoded, err := encodeResponse(&cif.CanonicalResponse{
		Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call", ToolName: "shell", ToolKind: cif.CIFToolKindCustom, RawInput: &empty}},
	})
	if err != nil {
		t.Fatalf("encodeResponse: %v", err)
	}
	decoded, err := decodeResponse(encoded)
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	call := decoded.Content[0].(cif.CIFToolCallPart)
	if call.RawInput == nil || *call.RawInput != "" {
		t.Fatalf("RawInput = %#v; want present empty value", call.RawInput)
	}
}

func TestDecodeLegacyToolCall(t *testing.T) {
	decoded, err := decodeResponse(`{"content":[{"type":"tool_call","toolCallId":"legacy","toolName":"lookup","toolArguments":{"q":"go"}}]}`)
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	call := decoded.Content[0].(cif.CIFToolCallPart)
	if call.ToolCallID != "legacy" || call.ToolName != "lookup" || call.ToolArguments["q"] != "go" {
		t.Fatalf("legacy call lost existing fields: %#v", call)
	}
	if call.ToolKind != "" || call.RawInput != nil || call.Namespace != "" {
		t.Fatalf("legacy call gained custom metadata: %#v", call)
	}
}
