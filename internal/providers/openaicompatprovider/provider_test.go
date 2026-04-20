package openaicompatprovider

import (
	"omnillm/internal/providers/types"
	"testing"
)

func TestGetID(t *testing.T) {
	p := NewProvider("openai-compatible-localhost", "Test")
	if got := p.GetID(); got != "openai-compatible" {
		t.Fatalf("GetID() = %q, want %q", got, "openai-compatible")
	}
}

func TestGetInstanceID(t *testing.T) {
	p := NewProvider("openai-compatible-localhost-abc123", "")
	if got := p.GetInstanceID(); got != "openai-compatible-localhost-abc123" {
		t.Fatalf("GetInstanceID() = %q", got)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"https://api.example.com/v1/", "https://api.example.com/v1"},
		{"api.example.com/v1", "https://api.example.com/v1"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeBaseURL(tt.raw); got != tt.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestCanonicalInstanceID(t *testing.T) {
	tests := []struct {
		endpoint string
		apiKey   string
		wantPfx  string
	}{
		{"http://localhost:11434/v1", "", "openai-compatible-localhost"},
		{"https://api.example.com/v1", "sk-abcdef", "openai-compatible-api-example-com-abcdef"},
	}
	for _, tt := range tests {
		got := CanonicalInstanceID(tt.endpoint, tt.apiKey)
		if got != tt.wantPfx {
			t.Errorf("CanonicalInstanceID(%q, %q) = %q, want %q", tt.endpoint, tt.apiKey, got, tt.wantPfx)
		}
	}
}

func TestDeriveDisplayName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"http://localhost:11434/v1", "OpenAI-Compatible (localhost:11434)"},
		{"https://api.openai.com/v1", "OpenAI-Compatible (api.openai.com)"},
	}
	for _, tt := range tests {
		if got := deriveDisplayName(tt.url); got != tt.want {
			t.Errorf("deriveDisplayName(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestBuildHeaders_NoToken(t *testing.T) {
	h := buildHeaders("", false)
	if _, ok := h["Authorization"]; ok {
		t.Error("expected no Authorization header for empty token")
	}
	if h["Content-Type"] != "application/json" {
		t.Error("expected Content-Type: application/json")
	}
}

func TestBuildHeaders_WithToken(t *testing.T) {
	h := buildHeaders("sk-test", false)
	if h["Authorization"] != "Bearer sk-test" {
		t.Errorf("Authorization = %q", h["Authorization"])
	}
}

func TestBuildHeaders_Stream(t *testing.T) {
	h := buildHeaders("sk-test", true)
	if h["Accept"] != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", h["Accept"])
	}
}

func TestSetupAuth_MissingEndpoint(t *testing.T) {
	p := NewProvider("test-instance", "")
	err := p.SetupAuth(&types.AuthOptions{APIKey: "sk-test"})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestGetAdapter_NotNil(t *testing.T) {
	p := NewProvider("test-instance", "Test")
	p.baseURL = "http://localhost:11434/v1"
	p.configLoaded = true
	a := p.GetAdapter()
	if a == nil {
		t.Fatal("GetAdapter() returned nil")
	}
	if a.GetProvider() != p {
		t.Fatal("GetAdapter().GetProvider() != p")
	}
}

func TestRemapModel(t *testing.T) {
	p := NewProvider("test", "")
	p.baseURL = "http://localhost:11434/v1"
	p.configLoaded = true
	a := p.GetAdapter()
	if got := a.RemapModel("  llama3  "); got != "llama3" {
		t.Errorf("RemapModel = %q, want %q", got, "llama3")
	}
}
