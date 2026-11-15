package routes

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

type blockingEmbeddingProvider struct {
	types.Provider
	started chan struct{}
	release chan struct{}
}

func (p *blockingEmbeddingProvider) GetID() string         { return "lease-test" }
func (p *blockingEmbeddingProvider) GetInstanceID() string { return "lease-embedding" }
func (p *blockingEmbeddingProvider) GetName() string       { return "Lease embedding" }
func (p *blockingEmbeddingProvider) CreateEmbeddings(_ map[string]interface{}) (map[string]interface{}, error) {
	close(p.started)
	<-p.release
	return map[string]interface{}{"data": []interface{}{}}, nil
}

func TestEmbeddingHoldsProviderLease(t *testing.T) {
	resetAdminTestState(t)
	reg := registry.GetProviderRegistry()
	provider := &blockingEmbeddingProvider{started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(provider.release) }) }
	defer releaseProvider()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if _, err := reg.AddActive(provider.GetInstanceID()); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	reg.WaitForPendingSaves()
	t.Cleanup(func() {
		resetAdminTestState(t)
		reg.WaitForPendingSaves()
		_ = database.NewProviderInstanceStore().Delete(provider.GetInstanceID())
	})

	router := gin.New()
	SetupEmbeddingRoutes(router.Group("/"))
	responses := make(chan int, 1)
	go func() {
		response := performJSONRequest(t, router, http.MethodPost, "/embeddings", `{"model":"embedding"}`)
		responses <- response.Code
	}()
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("embedding did not reach provider")
	}
	if _, err := reg.TryLockIdentity(provider.GetInstanceID()); !errors.Is(err, registry.ErrProviderBusy) {
		t.Fatalf("rename lock during embedding = %v, want busy", err)
	}
	releaseProvider()
	select {
	case status := <-responses:
		if status != http.StatusOK {
			t.Fatalf("embedding returned %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("embedding did not finish")
	}
	unlock, err := reg.TryLockIdentity(provider.GetInstanceID())
	if err != nil {
		t.Fatalf("rename lock after embedding: %v", err)
	}
	unlock()
}
