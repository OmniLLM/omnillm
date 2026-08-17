package responsecache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"omnillm/internal/cif"
)

type recordingStore struct {
	mu        sync.Mutex
	record    *Record
	getErr    error
	saveErr   error
	getCtx    context.Context
	saveCtx   context.Context
	saves     int
	savedTTL  time.Duration
	savedData string
	available bool
}

func (s *recordingStore) Get(ctx context.Context, _ string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCtx = ctx
	return s.record, s.getErr
}

func (s *recordingStore) Save(ctx context.Context, _, _, responseData string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCtx = ctx
	s.saves++
	s.savedTTL = ttl
	s.savedData = responseData
	return s.saveErr
}

func (s *recordingStore) Stats(context.Context) (Stats, error) { return Stats{}, nil }
func (s *recordingStore) Clear(context.Context) (int64, error) { return 0, nil }
func (s *recordingStore) Available() bool                      { return s.available }
func (s *recordingStore) Close() error                         { return nil }

func TestDefaultStoreFailsOpen(t *testing.T) {
	restore := ConfigureStore(nil)
	defer restore()
	req := baseReq()
	if got := GetContext(context.Background(), Config{Enabled: true}, req, Key(req)); got != nil {
		t.Fatalf("GetContext unavailable store = %#v, want miss", got)
	}
	PutContext(context.Background(), Config{Enabled: true}, req, Key(req), cacheTestResponse())
	if CurrentStore().Available() {
		t.Fatal("default unavailable store reports available")
	}
}

func TestGetPutContextPropagationAndFailOpen(t *testing.T) {
	response := cacheTestResponse()
	encoded, err := encodeResponse(response)
	if err != nil {
		t.Fatalf("encodeResponse: %v", err)
	}
	store := &recordingStore{record: &Record{ResponseData: encoded}, available: true}
	restore := ConfigureStore(store)
	defer restore()
	req := baseReq()
	key := Key(req)
	type markerKey struct{}
	ctx := context.WithValue(context.Background(), markerKey{}, "request")

	got := GetContext(ctx, Config{Enabled: true, TTL: time.Minute}, req, key)
	if got == nil || got.ID != response.ID {
		t.Fatalf("GetContext = %#v", got)
	}
	PutContext(ctx, Config{Enabled: true, TTL: 0}, req, key, response)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.getCtx.Value(markerKey{}) != "request" || store.saveCtx.Value(markerKey{}) != "request" {
		t.Fatal("request context did not propagate to store")
	}
	if store.savedTTL != DefaultTTL {
		t.Fatalf("saved TTL = %v, want %v", store.savedTTL, DefaultTTL)
	}

	store.getErr = errors.New("Redis down")
	store.record = nil
	store.mu.Unlock()
	if got := GetContext(ctx, Config{Enabled: true}, req, key); got != nil {
		t.Fatalf("GetContext backend error = %#v, want miss", got)
	}
	store.mu.Lock()
}

func TestPutContextExcludesUnsupportedResponses(t *testing.T) {
	store := &recordingStore{available: true}
	restore := ConfigureStore(store)
	defer restore()
	req := baseReq()
	key := Key(req)

	cases := []*cif.CanonicalResponse{
		nil,
		{StopReason: cif.StopReasonError, Content: []cif.CIFContentPart{cif.CIFTextPart{Text: "error"}}},
		{StopReason: cif.StopReasonEndTurn},
		{StopReason: cif.StopReasonEndTurn, Content: []cif.CIFContentPart{cif.CIFImagePart{}}},
	}
	for _, response := range cases {
		PutContext(context.Background(), Config{Enabled: true}, req, key, response)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.saves != 0 {
		t.Fatalf("unsupported responses caused %d saves", store.saves)
	}
}

func TestConfigureStoreConcurrentAccessAndRestore(t *testing.T) {
	original := CurrentStore()
	first := &recordingStore{}
	restore := ConfigureStore(first)
	if CurrentStore() != first {
		t.Fatal("ConfigureStore did not install store")
	}
	const workers = 32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = CurrentStore().Available()
			}
		}()
	}
	wg.Wait()
	restore()
	restore()
	if CurrentStore() != original {
		t.Fatal("restore did not reinstate original store")
	}
}

func cacheTestResponse() *cif.CanonicalResponse {
	return &cif.CanonicalResponse{
		ID:         "response-id",
		Model:      "model",
		StopReason: cif.StopReasonEndTurn,
		Content:    []cif.CIFContentPart{cif.CIFTextPart{Text: "cached"}},
	}
}
