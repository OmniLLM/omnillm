package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func buildLegacyV6DB(t *testing.T) *Database {
	t.Helper()

	tmpDir := t.TempDir()
	rawDB, err := sql.Open("sqlite", filepath.Join(tmpDir, "legacy-v6.sqlite"))
	if err != nil {
		t.Fatalf("open legacy DB: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	statements := []string{
		`CREATE TABLE schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE provider_instances (
			instance_id TEXT    PRIMARY KEY,
			provider_id TEXT    NOT NULL,
			name        TEXT    NOT NULL,
			subtitle    TEXT    NOT NULL DEFAULT '',
			priority    INTEGER NOT NULL DEFAULT 0,
			activated   INTEGER NOT NULL DEFAULT 0 CHECK (activated IN (0, 1)),
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE tokens (
			instance_id TEXT PRIMARY KEY,
			token_data  TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE provider_model_states (instance_id TEXT NOT NULL, model_id TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')), PRIMARY KEY (instance_id, model_id), FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE)`,
		`CREATE TABLE provider_configs (instance_id TEXT PRIMARY KEY, config_data TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')), FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE)`,
		`CREATE TABLE model_configs (instance_id TEXT NOT NULL, model_id TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, config_data TEXT NOT NULL DEFAULT '{}', created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')), PRIMARY KEY (instance_id, model_id), FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE)`,
		`CREATE TABLE provider_models_cache (instance_id TEXT PRIMARY KEY, models_data TEXT NOT NULL, cached_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME, FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE)`,
		`CREATE TABLE chat_sessions (session_id TEXT PRIMARY KEY, title TEXT NOT NULL, model_id TEXT NOT NULL, api_shape TEXT NOT NULL DEFAULT 'openai', created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE chat_messages (message_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, role TEXT NOT NULL CHECK (role IN ('user','assistant','system')), content TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT (datetime('now')), FOREIGN KEY (session_id) REFERENCES chat_sessions (session_id) ON DELETE CASCADE)`,
		`CREATE TABLE virtual_models (virtual_model_id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', api_shape TEXT NOT NULL DEFAULT 'openai', lb_strategy TEXT NOT NULL DEFAULT 'round-robin' CHECK (lb_strategy IN ('round-robin','random','priority','weighted')), enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)), created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE virtual_model_upstreams (id INTEGER PRIMARY KEY AUTOINCREMENT, virtual_model_id TEXT NOT NULL, provider_id TEXT NOT NULL DEFAULT '', model_id TEXT NOT NULL, weight INTEGER NOT NULL DEFAULT 1, priority INTEGER NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT (datetime('now')), FOREIGN KEY (virtual_model_id) REFERENCES virtual_models (virtual_model_id) ON DELETE CASCADE)`,
		`INSERT INTO schema_migrations (version) VALUES (1),(2),(3),(4),(5)`,
		`INSERT INTO virtual_models (virtual_model_id, name) VALUES ('vm-legacy', 'Legacy VM')`,
		`INSERT INTO virtual_model_upstreams (virtual_model_id, provider_id, model_id, weight, priority, created_at) VALUES ('vm-legacy', 'provider-a', 'model-a', 2, 1, '2026-04-25 12:34:56')`,
	}

	for _, s := range statements {
		if _, err := rawDB.Exec(s); err != nil {
			t.Fatalf("seed legacy DB: %v", err)
		}
	}

	return &Database{db: rawDB}
}

func TestMigrationV6_BackfillsVirtualModelUpstreamsUpdatedAt(t *testing.T) {
	db := buildLegacyV6DB(t)

	if err := db.createTables(); err != nil {
		t.Fatalf("createTables: %v", err)
	}

	var updatedAt sql.NullString
	if err := db.db.QueryRow(`SELECT updated_at FROM virtual_model_upstreams WHERE virtual_model_id = ?`, "vm-legacy").Scan(&updatedAt); err != nil {
		t.Fatalf("read migrated updated_at: %v", err)
	}
	if !updatedAt.Valid || !parseTime(updatedAt.String).Equal(time.Date(2026, 4, 25, 12, 34, 56, 0, time.UTC)) {
		t.Fatalf("expected updated_at to be backfilled from created_at, got %#v", updatedAt)
	}
}

func TestMigrationV6_CreatesOrderingIndexes(t *testing.T) {
	db := buildLegacyV6DB(t)

	if err := db.createTables(); err != nil {
		t.Fatalf("createTables: %v", err)
	}

	for _, indexName := range []string{
		"idx_chat_sessions_updated_at",
		"idx_chat_messages_session_created_at",
		"idx_virtual_model_upstreams_ordering",
	} {
		var count int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name = ?`, indexName).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist", indexName)
		}
	}
}

func TestMigrationV6_RecordedInSchemaTable(t *testing.T) {
	db := buildLegacyV6DB(t)

	if err := db.createTables(); err != nil {
		t.Fatalf("createTables: %v", err)
	}

	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 6`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migration v6 to be recorded, got count=%d", count)
	}
}
