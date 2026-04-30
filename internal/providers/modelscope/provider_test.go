package modelscope

import (
	"omnillm/internal/cif"
	"testing"
)

func TestGetID(t *testing.T) {
	p := NewProvider("ms-test-1", "Test")
	if got := p.GetID(); got != "alibaba-modelscope" {
		t.Errorf("GetID() = %q, want %q", got, "alibaba-modelscope")
	}
}

func TestGetInstanceID(t *testing.T) {
	p := NewProvider("ms-test-1", "Test")
	if got := p.GetInstanceID(); got != "ms-test-1" {
		t.Errorf("GetInstanceID() = %q, want %q", got, "ms-test-1")
	}
}

func TestDefaultBaseURL(t *testing.T) {
	p := NewProvider("ms-test-1", "Test")
	if got := p.baseURL; got != BaseURL {
		t.Errorf("default baseURL = %q, want %q", got, BaseURL)
	}
}

func TestEnsureBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", BaseURL},
		{"https://api-inference.modelscope.cn/v1", "https://api-inference.modelscope.cn/v1"},
		{"https://api-inference.modelscope.cn/v1/", "https://api-inference.modelscope.cn/v1"},
		{"api-inference.modelscope.cn", "https://api-inference.modelscope.cn/v1"},
		{"http://localhost:8080", "http://localhost:8080/v1"},
	}
	for _, tc := range tests {
		got := ensureBaseURL(tc.raw)
		if got != tc.want {
			t.Errorf("ensureBaseURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestChatURL(t *testing.T) {
	got := chatURL(BaseURL)
	want := "https://api-inference.modelscope.cn/v1/chat/completions"
	if got != want {
		t.Errorf("chatURL(%q) = %q, want %q", BaseURL, got, want)
	}
}

func TestBuildRequest_NoEnableThinking(t *testing.T) {
	p := NewProvider("ms-test-1", "Test")
	a := &Adapter{provider: p}

	// Build a request with no tools (the case where DashScope injects enable_thinking).
	request := &cif.CanonicalRequest{
		Model: "deepseek-ai/DeepSeek-V4-Flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hello"}},
			},
		},
	}

	cr, err := a.buildRequest(request, false)
	if err != nil {
		t.Fatalf("buildRequest() error: %v", err)
	}

	if cr.Extras != nil {
		if _, ok := cr.Extras["enable_thinking"]; ok {
			t.Fatal("buildRequest() should NOT set enable_thinking for ModelScope")
		}
	}
}

func TestBuildRequest_NoEnableThinkingWithTools(t *testing.T) {
	p := NewProvider("ms-test-1", "Test")
	a := &Adapter{provider: p}

	desc := "Echo input"
	request := &cif.CanonicalRequest{
		Model: "deepseek-ai/DeepSeek-V4-Flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "call echo"}},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:             "echo",
				Description:      &desc,
				ParametersSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"text": map[string]interface{}{"type": "string"}}},
			},
		},
	}

	cr, err := a.buildRequest(request, false)
	if err != nil {
		t.Fatalf("buildRequest() error: %v", err)
	}

	if cr.Extras != nil {
		if _, ok := cr.Extras["enable_thinking"]; ok {
			t.Fatal("buildRequest() should NOT set enable_thinking for ModelScope even with tools")
		}
	}

	if len(cr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cr.Tools))
	}
}

func TestRemapModel_PassThrough(t *testing.T) {
	a := &Adapter{provider: NewProvider("ms-test-1", "Test")}
	tests := []string{
		"deepseek-ai/DeepSeek-V4-Flash",
		"Qwen/Qwen3-32B",
		"ZhipuAI/GLM-5.1",
	}
	for _, model := range tests {
		if got := a.RemapModel(model); got != model {
			t.Errorf("RemapModel(%q) = %q, want pass-through", model, got)
		}
	}
}
