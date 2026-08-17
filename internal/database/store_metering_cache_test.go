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
		{RequestID: "response-cache", ModelID: filter.ModelID, ProviderID: "response-cache", InputTokens: 20, OutputTokens: 4, TotalTokens: 24, CreatedAt: time.Now()},
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
	if statuses["hit"] != PromptCacheHit || statuses["miss"] != PromptCacheMiss || statuses["unknown"] != PromptCacheUnknown || statuses["response-cache"] != PromptCacheUnknown {
		t.Fatalf("statuses = %#v", statuses)
	}

	stats, err := db.GetMeteringStats(filter)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CacheHits != 1 || stats.CacheMisses != 1 || stats.CacheUnknown != 2 || stats.CacheReadTokens != 12 || stats.CacheWriteTokens != 3 {
		t.Fatalf("stats = %#v", stats)
	}

	for status, want := range map[PromptCacheStatus][]string{
		PromptCacheHit:     {"hit"},
		PromptCacheMiss:    {"miss"},
		PromptCacheUnknown: {"response-cache", "unknown"},
	} {
		filtered, count, err := db.ListMeteringRecords(MeteringFilter{ModelID: filter.ModelID, PromptCacheStatus: status}, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if count != int64(len(want)) || len(filtered) != len(want) {
			t.Fatalf("filter %s = %#v count=%d", status, filtered, count)
		}
		for index, requestID := range want {
			if filtered[index].RequestID != requestID {
				t.Fatalf("filter %s[%d] = %q, want %q", status, index, filtered[index].RequestID, requestID)
			}
		}
	}
}
