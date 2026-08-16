package database

import (
	"testing"
	"time"
)

func intp(value int) *int { return &value }

func TestPromptCacheStatusAndAggregates(t *testing.T) {
	db := GetDatabase()
	filter := MeteringFilter{ModelID: "cache-status-test"}
	rows := []MeteringRecord{
		{RequestID: "hit", ModelID: filter.ModelID, ProviderID: "p", CacheReadInputTokens: intp(12), CacheWriteInputTokens: intp(3), CreatedAt: time.Now()},
		{RequestID: "miss", ModelID: filter.ModelID, ProviderID: "p", CacheReadInputTokens: intp(0), CreatedAt: time.Now()},
		{RequestID: "unknown", ModelID: filter.ModelID, ProviderID: "p", CreatedAt: time.Now()},
	}
	for _, row := range rows {
		if err := db.InsertMeteringRecord(row); err != nil {
			t.Fatalf("insert %s: %v", row.RequestID, err)
		}
	}

	records, _, err := db.ListMeteringRecords(filter, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]PromptCacheStatus{}
	for _, record := range records {
		statuses[record.RequestID] = record.PromptCacheStatus
	}
	if statuses["hit"] != PromptCacheHit || statuses["miss"] != PromptCacheMiss || statuses["unknown"] != PromptCacheUnknown {
		t.Fatalf("statuses = %#v", statuses)
	}

	stats, err := db.GetMeteringStats(filter)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CacheHits != 1 || stats.CacheMisses != 1 || stats.CacheUnknown != 1 || stats.CacheReadTokens != 12 || stats.CacheWriteTokens != 3 {
		t.Fatalf("stats = %#v", stats)
	}

	for status, want := range map[PromptCacheStatus]string{PromptCacheHit: "hit", PromptCacheMiss: "miss", PromptCacheUnknown: "unknown"} {
		filtered, count, err := db.ListMeteringRecords(MeteringFilter{ModelID: filter.ModelID, PromptCacheStatus: status}, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 || len(filtered) != 1 || filtered[0].RequestID != want {
			t.Fatalf("filter %s = %#v count=%d", status, filtered, count)
		}
	}
}
