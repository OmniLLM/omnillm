package responsecache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisStore(t *testing.T, prefix string) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr:         server.Addr(),
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolTimeout:  2 * time.Second,
		MaxRetries:   -1,
	})
	store, err := NewRedisStore(client, RedisStoreConfig{
		Prefix:           prefix,
		CommandTimeout:   2 * time.Second,
		CircuitCooldown:  20 * time.Millisecond,
		RecoveryInterval: time.Hour,
		ScanCount:        3,
		BatchSize:        2,
	})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, server
}

func digest(number int) string {
	return fmt.Sprintf("%064x", number)
}

func TestRedisStoreMissSaveHitAndOverwrite(t *testing.T) {
	store, server := newTestRedisStore(t, "tenant_a")
	ctx := context.Background()
	key := digest(1)

	record, err := store.Get(ctx, key)
	if err != nil || record != nil {
		t.Fatalf("Get miss = (%v, %v), want (nil, nil)", record, err)
	}
	if err := store.Save(ctx, key, "model-a", `{"id":"first"}`, time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	record, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get hit: %v", err)
	}
	if record == nil || record.ResponseData != `{"id":"first"}` || record.ModelID != "model-a" || record.HitCount != 1 || record.LastHitAt == nil {
		t.Fatalf("unexpected first hit: %#v", record)
	}

	if err := store.Save(ctx, key, "model-b", `{"id":"second"}`, 2*time.Minute); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	raw := server.HGet(store.entry+key, "hit_count")
	if raw != "0" {
		t.Fatalf("hit count after overwrite = %q, want 0", raw)
	}
	if lastHit := server.HGet(store.entry+key, "last_hit_at_ms"); lastHit != "" {
		t.Fatalf("last hit after overwrite = %q, want empty", lastHit)
	}
	record, err = store.Get(ctx, key)
	if err != nil || record == nil || record.ResponseData != `{"id":"second"}` || record.HitCount != 1 {
		t.Fatalf("Get overwritten = (%#v, %v)", record, err)
	}
}

func TestRedisStoreConcurrentHitAccounting(t *testing.T) {
	store, _ := newTestRedisStore(t, "concurrent")
	ctx := context.Background()
	key := digest(2)
	if err := store.Save(ctx, key, "model", `{"ok":true}`, time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}

	const readers = 64
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := store.Get(ctx, key)
			if err != nil {
				errs <- err
				return
			}
			if record == nil {
				errs <- errors.New("unexpected miss")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Get: %v", err)
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats != (Stats{Entries: 1, TotalHits: readers}) {
		t.Fatalf("Stats = %#v, want one entry and %d hits", stats, readers)
	}
}

func TestRedisStoreNativeTTLAndNonResurrection(t *testing.T) {
	store, server := newTestRedisStore(t, "ttl")
	ctx := context.Background()
	oldKey, newKey := digest(3), digest(4)
	if err := store.Save(ctx, oldKey, "model", `{"old":true}`, time.Second); err != nil {
		t.Fatalf("Save old: %v", err)
	}
	server.FastForward(500 * time.Millisecond)
	if err := store.Save(ctx, newKey, "model", `{"new":true}`, 10*time.Second); err != nil {
		t.Fatalf("Save new: %v", err)
	}
	server.FastForward(600 * time.Millisecond)

	oldRecord, err := store.Get(ctx, oldKey)
	if err != nil || oldRecord != nil {
		t.Fatalf("expired Get = (%#v, %v), want miss", oldRecord, err)
	}
	newRecord, err := store.Get(ctx, newKey)
	if err != nil || newRecord == nil {
		t.Fatalf("live Get = (%#v, %v), want hit", newRecord, err)
	}
	server.FastForward(20 * time.Second)
	oldRecord, err = store.Get(ctx, oldKey)
	if err != nil || oldRecord != nil {
		t.Fatalf("expired entry resurrected: (%#v, %v)", oldRecord, err)
	}
}

func TestRedisStoreMalformedEntriesAreMissesAndRemoved(t *testing.T) {
	store, server := newTestRedisStore(t, "malformed")
	ctx := context.Background()
	key := digest(5)
	redisKey := store.entry + key
	server.HSet(redisKey, "schema_version", redisSchemaVersion, "response_data", "not-json", "model_id", "model", "created_at_ms", "1")

	record, err := store.Get(ctx, key)
	if err != nil || record != nil {
		t.Fatalf("malformed Get = (%#v, %v), want miss", record, err)
	}
	if server.Exists(redisKey) {
		t.Fatal("malformed entry was not removed")
	}
}

func TestRedisStoreStatsClearNamespaceAndBatches(t *testing.T) {
	store, server := newTestRedisStore(t, "owned")
	ctx := context.Background()
	for i := 1; i <= 7; i++ {
		key := digest(100 + i)
		if err := store.Save(ctx, key, "model", fmt.Sprintf(`{"n":%d}`, i), time.Minute); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		for hit := 0; hit < i; hit++ {
			if _, err := store.Get(ctx, key); err != nil {
				t.Fatalf("Get %d: %v", i, err)
			}
		}
	}
	server.Set("unrelated", "keep")
	server.Set("other:response-cache:v1:entry:"+digest(1), "keep")

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats != (Stats{Entries: 7, TotalHits: 28}) {
		t.Fatalf("Stats = %#v", stats)
	}
	removed, err := store.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if removed != 7 {
		t.Fatalf("Clear removed %d, want 7", removed)
	}
	stats, err = store.Stats(ctx)
	if err != nil || stats != (Stats{}) {
		t.Fatalf("Stats after clear = (%#v, %v)", stats, err)
	}
	if value, err := server.Get("unrelated"); err != nil || value != "keep" {
		t.Fatalf("unrelated key changed: (%q, %v)", value, err)
	}
	if value, err := server.Get("other:response-cache:v1:entry:" + digest(1)); err != nil || value != "keep" {
		t.Fatalf("other namespace changed: (%q, %v)", value, err)
	}
}

func TestRedisStoreContextDeadlineDoesNotOpenCircuit(t *testing.T) {
	store, _ := newTestRedisStore(t, "deadline")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.Get(ctx, digest(6))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get error = %v, want context canceled", err)
	}
	if !store.Available() {
		t.Fatal("caller cancellation marked Redis unavailable")
	}
}

func TestRedisStoreCircuitBypassAndRecovery(t *testing.T) {
	store, server := newTestRedisStore(t, "recovery")
	ctx := context.Background()
	server.Close()
	_, firstErr := store.Get(ctx, digest(7))
	if firstErr == nil || errors.Is(firstErr, ErrStoreUnavailable) {
		t.Fatalf("first outage error = %v, want backend command error", firstErr)
	}
	started := time.Now()
	_, secondErr := store.Get(ctx, digest(7))
	if !errors.Is(secondErr, ErrStoreUnavailable) {
		t.Fatalf("open-circuit error = %v", secondErr)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Millisecond {
		t.Fatalf("open circuit took %v, want immediate bypass", elapsed)
	}
	if err := server.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	record, err := store.Get(ctx, digest(7))
	if err != nil || record != nil {
		t.Fatalf("half-open recovery Get = (%#v, %v)", record, err)
	}
	if !store.Available() {
		t.Fatal("store did not recover")
	}
}

func TestRedisStoreRecoveryLoopSkipsHealthyPings(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	store, err := NewRedisStore(client, RedisStoreConfig{
		Prefix:           "healthy_probe",
		CommandTimeout:   100 * time.Millisecond,
		CircuitCooldown:  10 * time.Millisecond,
		RecoveryInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer store.Close()

	baseline := server.CommandCount()
	time.Sleep(35 * time.Millisecond)
	if got := server.CommandCount() - baseline; got != 0 {
		t.Fatalf("healthy recovery loop issued %d Redis commands, want 0", got)
	}
}

func TestRedisStoreCloseIsIdempotent(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	store, err := NewRedisStore(client, RedisStoreConfig{Prefix: "close", RecoveryInterval: time.Hour})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	const closers = 16
	var wg sync.WaitGroup
	errs := make(chan error, closers)
	for range closers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	}
	if store.Available() {
		t.Fatal("closed store reports available")
	}
	if _, err := store.Get(context.Background(), digest(8)); !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("Get after Close = %v, want ErrStoreClosed", err)
	}
}

func TestRedisStoreRejectsInvalidKeysPrefixesAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	if _, err := NewRedisStore(client, RedisStoreConfig{Prefix: "bad:prefix"}); err == nil {
		t.Fatal("NewRedisStore accepted invalid prefix")
	}
	_ = client.Close()

	store, _ := newTestRedisStore(t, "validation")
	if _, err := store.Get(context.Background(), "not-a-digest"); err == nil {
		t.Fatal("Get accepted invalid digest")
	}
	if err := store.Save(context.Background(), digest(9), "model", `{}`, 0); err == nil {
		t.Fatal("Save accepted zero TTL")
	}
}
