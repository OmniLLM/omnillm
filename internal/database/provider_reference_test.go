package database

import (
	"errors"
	"reflect"
	"testing"
)

func providerReferenceCache(records ...ProviderInstanceRecord) *ModelResolutionCache {
	cache := &ModelResolutionCache{
		instLoaded:    true,
		instByID:      make(map[string]ProviderInstanceRecord),
		instByLcAlias: make(map[string][]string),
		instByLcName:  make(map[string][]string),
	}
	for _, record := range records {
		cache.instByID[record.InstanceID] = record
		if key := normalizeProviderReference(record.Subtitle); key != "" {
			cache.instByLcAlias[key] = append(cache.instByLcAlias[key], record.InstanceID)
		}
		if key := normalizeProviderReference(record.Name); key != "" {
			cache.instByLcName[key] = append(cache.instByLcName[key], record.InstanceID)
		}
	}
	return cache
}

func TestResolveProviderReference(t *testing.T) {
	cache := providerReferenceCache(
		ProviderInstanceRecord{InstanceID: "exact", Subtitle: "shared", Name: "Friendly"},
		ProviderInstanceRecord{InstanceID: "alias-target", Subtitle: "EXACT", Name: "Other"},
		ProviderInstanceRecord{InstanceID: "name-target", Subtitle: "short", Name: "Named Provider"},
	)

	for _, tc := range []struct {
		name, reference, want string
	}{
		{name: "exact ID wins", reference: "exact", want: "exact"},
		{name: "alias ignores case and whitespace", reference: "  ShOrT ", want: "name-target"},
		{name: "name ignores case", reference: "named provider", want: "name-target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cache.ResolveProviderReference(tc.reference)
			if err != nil || got != tc.want {
				t.Fatalf("ResolveProviderReference(%q) = %q, %v; want %q", tc.reference, got, err, tc.want)
			}
		})
	}

	if _, err := cache.ResolveProviderReference("missing"); !errors.Is(err, ErrProviderReferenceNotFound) {
		t.Fatalf("unknown reference error = %v", err)
	}
}

func TestResolveProviderReferenceAmbiguityAndPrecedence(t *testing.T) {
	cache := providerReferenceCache(
		ProviderInstanceRecord{InstanceID: "z-provider", Subtitle: "duplicate", Name: "duplicate-name"},
		ProviderInstanceRecord{InstanceID: "a-provider", Subtitle: "DUPLICATE", Name: "DUPLICATE-NAME"},
		ProviderInstanceRecord{InstanceID: "name-shadowed", Name: "duplicate"},
	)
	// Match lists loaded from SQL are sorted before publication.
	cache.instByLcAlias["duplicate"] = []string{"a-provider", "z-provider"}
	cache.instByLcName["duplicate-name"] = []string{"a-provider", "z-provider"}

	for _, reference := range []string{"duplicate", "duplicate-name"} {
		_, err := cache.ResolveProviderReference(reference)
		var ambiguous *AmbiguousProviderReferenceError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("ResolveProviderReference(%q) error = %v", reference, err)
		}
		if !reflect.DeepEqual(ambiguous.Matches, []string{"a-provider", "z-provider"}) {
			t.Fatalf("matches = %v", ambiguous.Matches)
		}
	}
}
