package responsecache

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRealRedisResponseCacheSmoke(t *testing.T) {
	url := os.Getenv("OMNILLM_TEST_REDIS_URL")
	if url == "" {
		t.Skip("OMNILLM_TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse Redis URL: %v", err)
	}
	options.MaxRetries = -1
	store, err := NewRedisStore(redis.NewClient(options), RedisStoreConfig{
		Prefix:           "omnillm_smoke",
		CommandTimeout:   time.Second,
		CircuitCooldown:  25 * time.Millisecond,
		RecoveryInterval: 25 * time.Millisecond,
		ScanCount:        2,
		BatchSize:        2,
	})
	if err != nil {
		t.Fatalf("create Redis store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}
	_, _ = store.Clear(ctx)

	for index := 1; index <= 4; index++ {
		key := digest(10_000 + index)
		if err := store.Save(ctx, key, "smoke-model", fmt.Sprintf(`{"index":%d}`, index), 300*time.Millisecond); err != nil {
			t.Fatalf("save %d: %v", index, err)
		}
		if _, err := store.Get(ctx, key); err != nil {
			t.Fatalf("get %d: %v", index, err)
		}
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Entries != 4 || stats.TotalHits != 4 {
		t.Fatalf("stats = %+v", stats)
	}

	time.Sleep(400 * time.Millisecond)
	stats, err = store.Stats(ctx)
	if err != nil {
		t.Fatalf("stats after expiry: %v", err)
	}
	if stats.Entries != 0 {
		t.Fatalf("expired entries = %d", stats.Entries)
	}

	foreignClient := redis.NewClient(options)
	defer foreignClient.Close()
	if err := foreignClient.Set(ctx, "omnillm-smoke-foreign", "keep", time.Minute).Err(); err != nil {
		t.Fatalf("set foreign key: %v", err)
	}
	if _, err := store.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if value, err := foreignClient.Get(ctx, "omnillm-smoke-foreign").Result(); err != nil || value != "keep" {
		t.Fatalf("foreign key after clear = %q, %v", value, err)
	}
	_ = foreignClient.Del(ctx, "omnillm-smoke-foreign").Err()
}
