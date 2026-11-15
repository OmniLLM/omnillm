package providermetadata

import "testing"

func TestOriginStripsUntrustedURLComponents(t *testing.T) {
	for _, test := range []struct{ name, input, want string }{
		{name: "credentials and path", input: "https://name:secret@example.test:8443/path-secret?token=secret#secret", want: "https://example.test:8443"},
		{name: "local IPv6", input: "http://[::1]:11434/v1", want: "http://[::1]:11434"},
		{name: "unsupported scheme", input: "file:///etc/passwd", want: ""},
		{name: "invalid host", input: "https://", want: ""},
		{name: "invalid port", input: "https://example.test:notaport", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := Origin(test.input); got != test.want {
				t.Errorf("Origin(%q)=%q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestSanitizeRejectsUntrustedOrInapplicableFields(t *testing.T) {
	input := Metadata{
		ID: "provider", AuthorizationEndpoint: "https://bad.example/auth?state=secret",
		TokenEndpoint: "https://bad.example/token", OAuthClientID: "SECRET_CLIENT_ID",
		APIEndpointOrigin: "https://user:password@example.test/secret?token=secret",
		Region:            "unknown-secret", Plan: "unknown-secret", APIFormat: "unknown-secret", APIVersion: "unknown-secret",
	}
	got := Sanitize("openai-compatible", input)
	if got.AuthorizationEndpoint != "" || got.TokenEndpoint != "" || got.OAuthClientID != "" ||
		got.Region != "" || got.Plan != "" || got.APIFormat != "" || got.APIVersion != "" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.APIEndpointOrigin != "https://example.test" {
		t.Fatalf("origin=%q", got.APIEndpointOrigin)
	}
}
