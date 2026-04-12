package alibaba

import (
	"testing"
	"time"
)

func TestGenerateCodeChallengeIsDeterministic(t *testing.T) {
	const verifier = "test-verifier"
	got := generateCodeChallenge(verifier)
	want := "JBbiqONGWPaAmwXk_8bT6UnlPfrn65D32eZlJS-zGG0"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestIsExpiringSoon(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		data *TokenData
		want bool
	}{
		{name: "nil", data: nil, want: false},
		{name: "api key", data: &TokenData{AuthType: "api-key", ExpiresAt: now.Add(-time.Hour).UnixMilli()}, want: false},
		{name: "never expires", data: &TokenData{AuthType: "oauth", ExpiresAt: 0}, want: false},
		{name: "already expired", data: &TokenData{AuthType: "oauth", ExpiresAt: now.Add(-time.Second).UnixMilli()}, want: true},
		{name: "still valid", data: &TokenData{AuthType: "oauth", ExpiresAt: now.Add(time.Hour).UnixMilli()}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsExpiringSoon(tt.data); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestNormaliseBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "trim and add scheme and v1", raw: " portal.qwen.ai ", want: "https://portal.qwen.ai/v1"},
		{name: "preserve existing v1", raw: "https://portal.qwen.ai/v1/", want: "https://portal.qwen.ai/v1"},
		{name: "append v1", raw: "http://example.com/base", want: "http://example.com/base/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormaliseBaseURL(tt.raw); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestMinDuration(t *testing.T) {
	if got := min(2*time.Second, 5*time.Second); got != 2*time.Second {
		t.Fatalf("expected smaller duration, got %v", got)
	}
	if got := min(7*time.Second, 3*time.Second); got != 3*time.Second {
		t.Fatalf("expected smaller duration, got %v", got)
	}
}
