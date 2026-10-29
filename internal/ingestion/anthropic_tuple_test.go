package ingestion

import (
	"encoding/json"
	"reflect"
	"testing"
)

func schemaJSON(t *testing.T, value string) map[string]interface{} {
	t.Helper()
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(value), &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestAnthropicLegacyTupleNormalization(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"RGB", `{"type":"array","items":[{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255}]}`, `{"type":"array","prefixItems":[{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255},{"type":"integer","minimum":0,"maximum":255}]}`},
		{"closed", `{"items":[true],"additionalItems":false}`, `{"prefixItems":[true],"items":false}`},
		{"open", `{"items":[false],"additionalItems":true}`, `{"prefixItems":[false],"items":true}`},
		{"trailing schema", `{"items":[{"type":"string"}],"additionalItems":{"items":[true]}}`, `{"prefixItems":[{"type":"string"}],"items":{"prefixItems":[true]}}`},
		{"modern", `{"prefixItems":[{"type":"string"}],"items":false}`, `{"prefixItems":[{"type":"string"}],"items":false}`},
		{"ordinary array", `{"items":{"type":"string"}}`, `{"items":{"type":"string"}}`},
		{"mixed dialect", `{"items":[true],"prefixItems":[false],"additionalItems":false}`, `{"items":[true],"prefixItems":[false],"additionalItems":false}`},
		{"literal values", `{"default":{"items":[true]},"const":{"items":[false]},"enum":[{"items":[true]}],"examples":[{"items":[false]}]}`, `{"default":{"items":[true]},"const":{"items":[false]},"enum":[{"items":[true]}],"examples":[{"items":[false]}]}`},
		{"keyword property names", `{"properties":{"items":{"type":"array","items":[true]},"prefixItems":{"type":"string"},"nullable":{"type":"boolean"},"$schema":{"type":"string"},"patternProperties":{"type":"string"}}}`, `{"properties":{"items":{"type":"array","prefixItems":[true]},"prefixItems":{"type":"string"},"nullable":{"type":"boolean"},"$schema":{"type":"string"},"patternProperties":{"type":"string"}}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := schemaJSON(t, tt.source)
			before, _ := json.Marshal(source)
			got := normalizeSchema(source)
			if want := schemaJSON(t, tt.want); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
			after, _ := json.Marshal(source)
			if string(before) != string(after) {
				t.Fatal("source schema mutated")
			}
		})
	}
}

func TestAnthropicNestedTupleSchemas(t *testing.T) {
	for _, keyword := range []string{"properties", "$defs", "definitions", "dependentSchemas", "dependencies"} {
		t.Run(keyword, func(t *testing.T) {
			source := schemaJSON(t, `{"`+keyword+`":{"items":{"items":[true]}}}`)
			want := schemaJSON(t, `{"`+keyword+`":{"items":{"prefixItems":[true]}}}`)
			if got := normalizeSchema(source); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems", "items"} {
		t.Run(keyword, func(t *testing.T) {
			source := schemaJSON(t, `{"`+keyword+`":[{"items":[true]}]}`)
			outKey := keyword
			if keyword == "items" {
				outKey = "prefixItems"
			}
			want := schemaJSON(t, `{"`+outKey+`":[{"prefixItems":[true]}]}`)
			if got := normalizeSchema(source); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
	for _, keyword := range []string{"items", "additionalItems", "additionalProperties", "unevaluatedItems", "unevaluatedProperties", "contains", "propertyNames", "not", "if", "then", "else", "contentSchema"} {
		t.Run(keyword+" schema", func(t *testing.T) {
			source := schemaJSON(t, `{"`+keyword+`":{"items":[true]}}`)
			want := schemaJSON(t, `{"`+keyword+`":{"prefixItems":[true]}}`)
			if got := normalizeSchema(source); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestAnthropicTupleToolMetadata(t *testing.T) {
	raw := json.RawMessage(`{"model":"claude-sonnet-5.5","max_tokens":16,"messages":[{"role":"user","content":"OK"}],"tools":[{"name":"create_color_harmony","description":"RGB harmony","cache_control":{"type":"ephemeral"},"input_schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"baseColor":{"type":"array","items":[{"type":"integer","minimum":0,"maximum":255}]}},"required":["baseColor"]}},{"name":"second","input_schema":{"type":"object"}}]}`)
	req, err := ParseAnthropicMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Tools) != 2 || req.Tools[0].Name != "create_color_harmony" || req.Tools[1].Name != "second" || req.Tools[0].Description == nil || *req.Tools[0].Description != "RGB harmony" || req.Tools[0].CacheControl == nil || req.Tools[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("tool metadata changed: %#v", req.Tools)
	}
	want := schemaJSON(t, `{"type":"object","properties":{"baseColor":{"type":"array","prefixItems":[{"type":"integer","minimum":0,"maximum":255}]}},"required":["baseColor"]}`)
	if !reflect.DeepEqual(req.Tools[0].ParametersSchema, want) {
		t.Fatalf("tool schema = %#v", req.Tools[0].ParametersSchema)
	}
}
