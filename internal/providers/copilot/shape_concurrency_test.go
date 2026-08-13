package copilot

import (
	"strings"
	"sync"
	"testing"

	"omnillm/internal/cif"
)

func TestShapeCacheNormalizesPublishedModelIDs(t *testing.T) {
	provider := &GitHubCopilotProvider{}
	cache := modelShapeCache{}
	modelID := "  GPT-5-CUSTOM  "
	cache[strings.ToLower(strings.TrimSpace(modelID))] = shapeResponses
	provider.publishShapeCache(cache)
	if !provider.IsResponsesOnlyModel("gpt-5-custom") {
		t.Fatal("normalized model ID did not resolve from shape cache")
	}
}

func TestShapeCacheConcurrentPublishAndLookup(t *testing.T) {
	provider := &GitHubCopilotProvider{}
	adapter := &CopilotAdapter{provider: provider}
	request := &cif.CanonicalRequest{}

	var wg sync.WaitGroup
	for i := range 1000 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			shape := shapeChat
			if i%2 == 0 {
				shape = shapeResponses
			}
			provider.publishShapeCache(modelShapeCache{"model": shape})
		}()
		go func() {
			defer wg.Done()
			shape := adapter.selectShape("model", request)
			if shape != shapeChat && shape != shapeResponses {
				t.Errorf("unexpected shape %q", shape)
			}
			_ = provider.IsResponsesOnlyModel("model")
		}()
	}
	wg.Wait()
}
