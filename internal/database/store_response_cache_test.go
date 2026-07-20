package database

import (
	"testing"
	"time"
)

func TestResponseCacheStore_SaveGet(t *testing.T) {
	s := NewResponseCacheStore()
	key := "test-key-savedget"
	if _, err := s.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}

	// Miss on empty.
	if rec, err := s.Get(key, time.Hour); err != nil || rec != nil {
		t.Fatalf("expected miss, got rec=%v err=%v", rec, err)
	}

	// Save then hit.
	if err := s.Save(key, "gpt-x", `{"id":"r1"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	rec, err := s.Get(key, time.Hour)
	if err != nil || rec == nil {
		t.Fatalf("expected hit, got rec=%v err=%v", rec, err)
	}
	if rec.ResponseData != `{"id":"r1"}` || rec.ModelID != "gpt-x" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.HitCount != 1 {
		t.Fatalf("expected hit_count 1 after first Get, got %d", rec.HitCount)
	}

	// Second hit increments.
	rec2, _ := s.Get(key, time.Hour)
	if rec2.HitCount != 2 {
		t.Fatalf("expected hit_count 2, got %d", rec2.HitCount)
	}
}

func TestResponseCacheStore_TTLExpiry(t *testing.T) {
	s := NewResponseCacheStore()
	key := "test-key-ttl"
	_, _ = s.Clear()
	if err := s.Save(key, "gpt-x", `{"id":"r"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	// A zero/near-zero TTL treats everything as expired.
	if rec, err := s.Get(key, time.Nanosecond); err != nil {
		t.Fatalf("get: %v", err)
	} else if rec != nil {
		t.Fatalf("expected expiry miss, got %+v", rec)
	}
	// ttl<=0 means no expiry check → hit.
	if rec, _ := s.Get(key, 0); rec == nil {
		t.Fatal("ttl=0 should disable expiry and return the row")
	}
}

func TestResponseCacheStore_Overwrite(t *testing.T) {
	s := NewResponseCacheStore()
	key := "test-key-overwrite"
	_, _ = s.Clear()
	_ = s.Save(key, "gpt-x", `{"v":1}`)
	_, _ = s.Get(key, time.Hour) // bump hit_count
	_ = s.Save(key, "gpt-x", `{"v":2}`)
	rec, _ := s.Get(key, time.Hour)
	if rec.ResponseData != `{"v":2}` {
		t.Fatalf("expected overwrite to newest value, got %s", rec.ResponseData)
	}
	// Overwrite resets hit_count to 0, then the Get above made it 1.
	if rec.HitCount != 1 {
		t.Fatalf("expected hit_count reset then 1, got %d", rec.HitCount)
	}
}

func TestResponseCacheStore_PurgeAndStats(t *testing.T) {
	s := NewResponseCacheStore()
	_, _ = s.Clear()
	_ = s.Save("k1", "m", `{}`)
	_ = s.Save("k2", "m", `{}`)
	entries, _, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if entries != 2 {
		t.Fatalf("expected 2 entries, got %d", entries)
	}
	// PurgeExpired with a huge TTL removes nothing.
	if n, _ := s.PurgeExpired(24 * time.Hour); n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}
	// Negative TTL pushes the cutoff into the future → removes all.
	if n, _ := s.PurgeExpired(-time.Hour); n != 2 {
		t.Fatalf("expected 2 purged, got %d", n)
	}
}
