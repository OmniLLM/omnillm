package alibaba

import (
	"testing"
)

func TestTokenDataFields(t *testing.T) {
	td := TokenData{
		AuthType:    "api-key",
		AccessToken: "sk-test",
		BaseURL:     "https://example.com/v1",
	}
	if td.AuthType != "api-key" {
		t.Errorf("AuthType = %q", td.AuthType)
	}
	if td.AccessToken != "sk-test" {
		t.Errorf("AccessToken = %q", td.AccessToken)
	}
}
