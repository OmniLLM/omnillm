package database

import "testing"

func TestMigrationV17RemovesOnlyLegacyResponseCache(t *testing.T) {
	db := GetDatabase()

	var originallyRecorded int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 17`).Scan(&originallyRecorded); err != nil {
		t.Fatalf("read original migration state: %v", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS response_cache (
			cache_key TEXT PRIMARY KEY,
			model_id TEXT NOT NULL,
			response_data TEXT NOT NULL,
			hit_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_hit_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_response_cache_created_at ON response_cache (created_at)`,
		`INSERT OR REPLACE INTO response_cache (cache_key, model_id, response_data) VALUES ('legacy-key', 'legacy-model', '{}')`,
		`INSERT OR REPLACE INTO config (key, value) VALUES ('response_cache.enabled', 'true')`,
		`INSERT OR REPLACE INTO provider_instances (instance_id, provider_id, name) VALUES ('migration-v17-provider', 'test', 'Migration Provider')`,
		`DELETE FROM schema_migrations WHERE version = 17`,
	}
	for _, statement := range statements {
		if _, err := db.db.Exec(statement); err != nil {
			t.Fatalf("seed migration v17 state: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.db.Exec(`DELETE FROM provider_instances WHERE instance_id = 'migration-v17-provider'`)
		_, _ = db.db.Exec(`DELETE FROM config WHERE key = 'response_cache.enabled'`)
		if originallyRecorded == 0 {
			_, _ = db.db.Exec(`DELETE FROM schema_migrations WHERE version = 17`)
		}
	})

	if err := db.applyMigrations(); err != nil {
		t.Fatalf("apply migration v17: %v", err)
	}

	for _, object := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "response_cache"},
		{kind: "index", name: "idx_response_cache_created_at"},
	} {
		var count int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, object.kind, object.name).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", object.name, err)
		}
		if count != 0 {
			t.Fatalf("expected %s %s to be removed", object.kind, object.name)
		}
	}

	var enabled string
	if err := db.db.QueryRow(`SELECT value FROM config WHERE key = 'response_cache.enabled'`).Scan(&enabled); err != nil {
		t.Fatalf("read preserved cache setting: %v", err)
	}
	if enabled != "true" {
		t.Fatalf("cache setting = %q, want true", enabled)
	}

	var providerName string
	if err := db.db.QueryRow(`SELECT name FROM provider_instances WHERE instance_id = 'migration-v17-provider'`).Scan(&providerName); err != nil {
		t.Fatalf("read preserved provider: %v", err)
	}
	if providerName != "Migration Provider" {
		t.Fatalf("provider name = %q", providerName)
	}

	var recorded int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 17`).Scan(&recorded); err != nil {
		t.Fatalf("read migration record: %v", err)
	}
	if recorded != 1 {
		t.Fatalf("migration v17 record count = %d, want 1", recorded)
	}
}

func TestFreshDatabaseHasNoSQLiteResponseCache(t *testing.T) {
	var count int
	if err := GetDatabase().db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'response_cache'`).Scan(&count); err != nil {
		t.Fatalf("query response_cache table: %v", err)
	}
	if count != 0 {
		t.Fatal("fresh database should not retain the SQLite response cache table")
	}
}
