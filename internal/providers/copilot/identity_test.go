package copilot

import "testing"

func TestCloneForIdentityPreservesInMemoryCredentials(t *testing.T) {
	p := NewGitHubCopilotProvider("old-id", "Custom name")
	p.token = "copilot-token"
	p.githubToken = "github-token"
	p.expiresAt = 123
	p.baseURL = "https://example.test"
	p.shapeCache = modelShapeCache{"model": shapeResponses}
	clone := p.CloneForIdentity("new-id")
	if clone == p || clone.GetInstanceID() != "new-id" || clone.GetName() != "Custom name" {
		t.Fatalf("clone identity = %#v", clone)
	}
	if clone.token != p.token || clone.githubToken != p.githubToken || clone.expiresAt != p.expiresAt || clone.baseURL != p.baseURL {
		t.Fatal("clone did not preserve Copilot credentials and endpoint")
	}
	clone.shapeCache["model"] = shapeChat
	if p.shapeCache["model"] != shapeResponses {
		t.Fatal("clone shared mutable shape metadata")
	}
}
