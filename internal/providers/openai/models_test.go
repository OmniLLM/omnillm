package openai

import (
	"strings"
	"testing"
)

// codexCatalogFixture mirrors the shape returned by
// GET /backend-api/codex/models?client_version=… (trimmed to the fields the
// parser reads).
const codexCatalogFixture = `{"models":[
	{"slug":"gpt-5.6-luna","display_name":"GPT-5.6-Luna","context_window":272000,"max_context_window":1000000,"visibility":"list"},
	{"slug":"gpt-5.4-mini","display_name":"GPT-5.4-Mini","context_window":272000,"visibility":"list"},
	{"slug":"codex-auto-review","display_name":"Codex Auto Review","context_window":272000,"visibility":"hide"}
]}`

func TestParseModelsResponseReadsCodexSlugShape(t *testing.T) {
	models, err := parseModelsResponse(strings.NewReader(codexCatalogFixture), "openai")
	if err != nil {
		t.Fatalf("parseModelsResponse: %v", err)
	}

	// codex-auto-review is dropped twice over: hidden, and a codex ID.
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2: %+v", len(models), models)
	}
	if models[0].ID != "gpt-5.6-luna" {
		t.Errorf("models[0].ID = %q, want gpt-5.6-luna", models[0].ID)
	}
	if models[0].Name != "GPT-5.6-Luna" {
		t.Errorf("models[0].Name = %q, want GPT-5.6-Luna", models[0].Name)
	}
	// context_window, not the 128k default, must win.
	if models[0].MaxTokens != 272000 {
		t.Errorf("models[0].MaxTokens = %d, want 272000", models[0].MaxTokens)
	}
	if models[0].Provider != "openai" {
		t.Errorf("models[0].Provider = %q, want openai", models[0].Provider)
	}
}

func TestParseModelsResponseHidesNonListedModels(t *testing.T) {
	models, err := parseModelsResponse(strings.NewReader(
		`{"models":[{"slug":"gpt-5.4","visibility":"list"},{"slug":"gpt-secret","visibility":"hide"}]}`), "openai")
	if err != nil {
		t.Fatalf("parseModelsResponse: %v", err)
	}
	for _, m := range models {
		if m.ID == "gpt-secret" {
			t.Error("hidden model leaked into the catalog")
		}
	}
}

// A response missing `visibility` entirely (older backends, or the platform
// shape) must not have every entry filtered out.
func TestParseModelsResponseKeepsEntriesWithoutVisibility(t *testing.T) {
	models, err := parseModelsResponse(strings.NewReader(
		`{"data":[{"id":"gpt-5.4","display_name":"GPT-5.4"}]}`), "openai")
	if err != nil {
		t.Fatalf("parseModelsResponse: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5.4" {
		t.Fatalf("got %+v, want a single gpt-5.4 entry", models)
	}
	// Upstream reported no limits, so the entry falls back to models.dev
	// metadata and, failing that, the built-in default. Either way it must be
	// populated with something usable.
	if models[0].MaxTokens <= 0 {
		t.Errorf("MaxTokens = %d, want a positive limit", models[0].MaxTokens)
	}
}

func TestParseModelsResponseErrorsWhenEmpty(t *testing.T) {
	if _, err := parseModelsResponse(strings.NewReader(`{"models":[]}`), "openai"); err == nil {
		t.Fatal("expected an error for an empty catalog, got nil")
	}
}
