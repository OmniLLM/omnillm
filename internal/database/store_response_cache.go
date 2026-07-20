package database

import (
	"database/sql"
	"time"
)

// ─── Exact-match response cache operations ────────────────────────────────────
//
// This cache stores full upstream LLM responses keyed by a hash of the
// canonical request (model + messages + tools + sampling params). It only ever
// stores DETERMINISTIC requests (temperature == 0) and never streaming
// requests — both invariants are enforced by the caller, not here. The store is
// deliberately dumb: hash in, JSON blob out.

// ResponseCacheRecord is a single cached upstream response.
type ResponseCacheRecord struct {
	CacheKey     string
	ModelID      string
	ResponseData string
	HitCount     int
	CreatedAt    time.Time
	LastHitAt    *time.Time
}

// ResponseCacheStore is the persistence layer for the exact-match response cache.
type ResponseCacheStore struct {
	db *Database
}

// NewResponseCacheStore returns a store bound to the process-global database.
func NewResponseCacheStore() *ResponseCacheStore {
	return &ResponseCacheStore{db: GetDatabase()}
}

// Get returns the cached response for cacheKey if it exists and is younger than
// ttl. A hit bumps hit_count and last_hit_at. Returns (nil, nil) on miss or
// expiry (an expired row is left in place for the sweeper to reap).
func (s *ResponseCacheStore) Get(cacheKey string, ttl time.Duration) (*ResponseCacheRecord, error) {
	var rec ResponseCacheRecord
	var createdAtStr string
	err := s.db.db.QueryRow(`
		SELECT cache_key, model_id, response_data, hit_count, created_at
		FROM response_cache WHERE cache_key = ?
	`, cacheKey).Scan(&rec.CacheKey, &rec.ModelID, &rec.ResponseData, &rec.HitCount, &createdAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rec.CreatedAt = parseTime(createdAtStr)
	if ttl > 0 && time.Since(rec.CreatedAt) > ttl {
		return nil, nil
	}

	// Best-effort hit accounting; a failure here must not fail the request.
	_, _ = s.db.db.Exec(
		`UPDATE response_cache SET hit_count = hit_count + 1, last_hit_at = datetime('now') WHERE cache_key = ?`,
		cacheKey,
	)
	rec.HitCount++
	return &rec, nil
}

// Save upserts a cached response. Overwriting an existing key resets its age and
// hit accounting, which is correct: a fresh upstream call supersedes the old one.
func (s *ResponseCacheStore) Save(cacheKey, modelID, responseData string) error {
	_, err := s.db.db.Exec(`
		INSERT INTO response_cache (cache_key, model_id, response_data, hit_count, created_at)
		VALUES (?, ?, ?, 0, datetime('now'))
		ON CONFLICT(cache_key) DO UPDATE SET
			model_id      = excluded.model_id,
			response_data = excluded.response_data,
			hit_count     = 0,
			created_at    = datetime('now'),
			last_hit_at   = NULL
	`, cacheKey, modelID, responseData)
	return err
}

// PurgeExpired deletes rows older than ttl and returns the number removed.
func (s *ResponseCacheStore) PurgeExpired(ttl time.Duration) (int64, error) {
	cutoff := time.Now().Add(-ttl).UTC().Format("2006-01-02 15:04:05")
	res, err := s.db.db.Exec(`DELETE FROM response_cache WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Clear removes every cached response and returns the number removed.
func (s *ResponseCacheStore) Clear() (int64, error) {
	res, err := s.db.db.Exec(`DELETE FROM response_cache`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats returns the current entry count and total accumulated hits.
func (s *ResponseCacheStore) Stats() (entries int, totalHits int, err error) {
	err = s.db.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(hit_count), 0) FROM response_cache`,
	).Scan(&entries, &totalHits)
	return entries, totalHits, err
}
