package responsecache

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var (
	// ErrStoreUnavailable indicates that response-cache storage is not currently
	// usable. Request-path callers deliberately treat it as a cache miss.
	ErrStoreUnavailable = errors.New("response cache store unavailable")
	// ErrStoreClosed indicates that the configured store has been shut down.
	ErrStoreClosed = errors.New("response cache store closed")
)

// Record is a response-cache entry and its storage metadata.
type Record struct {
	Key          string
	ModelID      string
	ResponseData string
	HitCount     int64
	CreatedAt    time.Time
	LastHitAt    *time.Time
}

// Stats contains live, namespace-scoped cache totals and lookup activity for
// the current Redis-backed statistics window.
type Stats struct {
	Entries       int64
	TotalHits     int64
	PayloadBytes  int64
	LookupHits    int64
	LookupMisses  int64
	LookupHitRate *float64
	StatsSince    *time.Time
}

// Store is the context-aware storage contract for exact-match responses.
type Store interface {
	Get(ctx context.Context, key string) (*Record, error)
	Save(ctx context.Context, key, modelID, responseData string, ttl time.Duration) error
	Stats(ctx context.Context) (Stats, error)
	Clear(ctx context.Context) (int64, error)
	Available() bool
	Close() error
}

type unavailableStore struct{}

func (unavailableStore) Get(context.Context, string) (*Record, error) { return nil, nil }
func (unavailableStore) Save(context.Context, string, string, string, time.Duration) error {
	return nil
}
func (unavailableStore) Stats(context.Context) (Stats, error) {
	return Stats{}, ErrStoreUnavailable
}
func (unavailableStore) Clear(context.Context) (int64, error) {
	return 0, ErrStoreUnavailable
}
func (unavailableStore) Available() bool { return false }
func (unavailableStore) Close() error    { return nil }

type storeHolder struct {
	store Store
}

var configuredStore atomic.Pointer[storeHolder]

func init() {
	configuredStore.Store(&storeHolder{store: unavailableStore{}})
}

// CurrentStore returns the currently configured response-cache store.
func CurrentStore() Store {
	return configuredStore.Load().store
}

// ConfigureStore atomically installs store and returns a concurrency-safe
// restore function intended for scoped configuration and tests. Passing nil
// installs the fail-open unavailable store.
func ConfigureStore(store Store) (restore func()) {
	if store == nil {
		store = unavailableStore{}
	}
	previous := configuredStore.Load()
	installed := &storeHolder{store: store}
	configuredStore.Store(installed)
	var restored atomic.Bool
	return func() {
		if restored.CompareAndSwap(false, true) {
			configuredStore.CompareAndSwap(installed, previous)
		}
	}
}
