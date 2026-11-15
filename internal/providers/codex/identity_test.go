package codex

import "testing"

func TestCloneForIdentityPreservesInMemoryCredentials(t *testing.T) {
	p := NewCodexProvider("old-id")
	p.name = "Custom name"
	p.apiKey = "unsaved-api-key"
	p.githubToken = "github-token"
	p.token = "legacy-token"
	p.expiresAt = 123
	p.baseURL = "https://example.test"
	clone := p.CloneForIdentity("new-id")
	if clone == p || clone.GetInstanceID() != "new-id" || clone.GetName() != "Custom name" {
		t.Fatalf("clone identity = %#v", clone)
	}
	if clone.apiKey != p.apiKey || clone.githubToken != p.githubToken || clone.token != p.token || clone.expiresAt != p.expiresAt || clone.baseURL != p.baseURL {
		t.Fatal("clone did not preserve Codex authentication state")
	}
}
