package commands

import (
	"strings"
	"testing"
)

func TestProviderEntityChoices(t *testing.T) {
	data := []byte(`[
		{"id":"id-first","name":"Shared Name","alias":"primary","subtitle":"ignored","config":{"token":"SECRET"}},
		{"id":"id-second","name":"Shared Name","alias":"  ","subtitle":"legacy"},
		{"id":"id-third","name":"","alias":""},
		{"id":"id-fourth","name":{"secret":"SECRET"}},
		{"id":""}, {"name":"missing"}, {"id":42}
	]`)
	choices, err := providerEntityChoices(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 4 {
		t.Fatalf("got %d choices, want 4", len(choices))
	}
	for i, want := range []struct{ id, name, alias string }{
		{id: "id-first", name: "Shared Name", alias: "primary"},
		{id: "id-second", name: "Shared Name", alias: "legacy"},
		{id: "id-third"},
		{id: "id-fourth"},
	} {
		got := choices[i]
		if got.id != want.id || got.name != want.name || got.alias != want.alias {
			t.Errorf("choice %d = %#v, want %#v", i, got, want)
		}
		label := got.label()
		if !strings.Contains(label, want.id) || strings.Contains(label, "SECRET") {
			t.Errorf("unsafe or missing ID in label %q", label)
		}
	}
	if _, err := providerEntityChoices([]byte(`{`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestVirtualModelEntityChoices(t *testing.T) {
	choices, err := virtualModelEntityChoices([]byte(`{"data":[{"virtual_model_id":"vm/a","name":"My Model"},{"virtual_model_id":"vm-b"},{"name":"missing"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 2 || choices[0].id != "vm/a" || choices[0].name != "My Model" || choices[1].id != "vm-b" {
		t.Fatalf("unexpected virtual-model choices: %#v", choices)
	}
	if !strings.Contains(choices[0].label(), "My Model") || !strings.Contains(choices[0].label(), "vm/a") {
		t.Errorf("unexpected label %q", choices[0].label())
	}
	if choices[1].label() != "vm-b" {
		t.Errorf("ID-only fallback = %q", choices[1].label())
	}
	if _, err := virtualModelEntityChoices([]byte(`{`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestEntityChoiceSearchAndSafeLabel(t *testing.T) {
	choice := entityChoice{id: "Id-123\x1b[2J", name: "Friendly\nName", alias: "Short\u009b"}
	label := choice.label()
	for _, value := range []string{"Id-123", "Friendly", "Name", "Short", `\x1b`, `\n`, `\u009b`} {
		if !strings.Contains(label, value) {
			t.Errorf("label %q missing %q", label, value)
		}
	}
	if strings.ContainsAny(label, "\x1b\n\u009b") {
		t.Fatalf("unsafe control character in label %q", label)
	}
	for _, query := range []string{"ID-123", "FRIENDLY", "sHoRt", `\X1B`} {
		if !choice.matches(query) {
			t.Errorf("choice did not match %q", query)
		}
	}
	if choice.matches("unrelated") {
		t.Error("choice matched unrelated text")
	}
	if choice.id != "Id-123\x1b[2J" {
		t.Errorf("rendering changed original ID: %q", choice.id)
	}
}

func TestResolveEntityFromListEmptyAndSingle(t *testing.T) {
	if _, err := resolveEntityFromList("select", nil); err == nil {
		t.Fatal("empty list must return an error")
	}
	want := "original/id"
	got, err := resolveEntityFromList("select", []entityChoice{{id: want, name: "Name"}})
	if err != nil || got != want {
		t.Fatalf("single choice: id=%q err=%v", got, err)
	}
}
