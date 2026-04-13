package kimi

import (
	"testing"
)

func TestKimiProvider(t *testing.T) {
	// Basic test to ensure the provider is correctly defined

	// Test that Models slice is not empty
	if len(Models) == 0 {
		t.Error("Expected Models to be defined, but got empty slice")
	}

	// Test that model IDs are properly defined
	for _, model := range Models {
		if model.ID == "" {
			t.Errorf("Expected model ID to be defined, but got empty string for model: %v", model)
		}
		if model.Name == "" {
			t.Errorf("Expected model Name to be defined, but got empty string for model: %v", model)
		}
	}

	// Test that OAuthSupportedModels is properly initialized (even if empty)
	if OAuthSupportedModels == nil {
		t.Error("Expected OAuthSupportedModels to be initialized, but got nil")
	}
}

func TestEnsureBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.moonshot.cn", "https://api.moonshot.cn/v1"},
		{"api.moonshot.cn", "https://api.moonshot.cn/v1"},
		{"https://api.moonshot.cn/v1", "https://api.moonshot.cn/v1"},
		{"https://api.moonshot.cn/v1/", "https://api.moonshot.cn/v1"},
	}

	for _, tt := range tests {
		result := EnsureBaseURL(tt.input)
		if result != tt.expected {
			t.Errorf("EnsureBaseURL(%q) = %q; expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestShortTokenSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdef12345", "12345"},
		{"short", "short"},
		{"hi", "hi"},
		{"", "oauth"},
	}

	for _, tt := range tests {
		result := ShortTokenSuffix(tt.input)
		if result != tt.expected {
			t.Errorf("ShortTokenSuffix(%q) = %q; expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsJWT(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"fake.header.sig", true},
		{"not-a-jwt", false},
		{"", false},
		{"a.b", false},
	}

	for _, tt := range tests {
		result := IsJWT(tt.input)
		if result != tt.expected {
			t.Errorf("IsJWT(%q) = %t; expected %t", tt.input, result, tt.expected)
		}
	}
}
