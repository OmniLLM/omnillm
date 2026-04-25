package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

type Database struct {
	db *sql.DB
}

var globalDB *Database

func InitializeDatabase(configDir string) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "database.sqlite")
	log.Debug().Str("path", dbPath).Msg("Initializing SQLite database")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite file database works best with a single writer connection
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(4)

	globalDB = &Database{db: db}

	if err := globalDB.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Debug().Msg("SQLite database initialized successfully")
	return nil
}

// GetDatabase returns the global database instance.
// Panics if InitializeDatabase was not called first.
func GetDatabase() *Database {
	if globalDB == nil {
		panic("database not initialized; call InitializeDatabase first")
	}
	return globalDB
}

func (db *Database) createTables() error {
	queries := []string{
		// Configuration table
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Provider instances table
		`CREATE TABLE IF NOT EXISTS provider_instances (
			instance_id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			name TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			activated INTEGER NOT NULL DEFAULT 0 CHECK (activated IN (0, 1)),
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Tokens table
		`CREATE TABLE IF NOT EXISTS tokens (
			instance_id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			token_data TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Provider model states table
		`CREATE TABLE IF NOT EXISTS provider_model_states (
			instance_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (instance_id, model_id),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Provider configurations table
		`CREATE TABLE IF NOT EXISTS provider_configs (
			instance_id TEXT PRIMARY KEY,
			config_data TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Model configurations table
		`CREATE TABLE IF NOT EXISTS model_configs (
			instance_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			version TEXT NOT NULL DEFAULT '1',
			config_data TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (instance_id, model_id),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Provider models cache table
		`CREATE TABLE IF NOT EXISTS provider_models_cache (
			instance_id TEXT PRIMARY KEY,
			models_data TEXT NOT NULL,
			cached_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Chat sessions table
		`CREATE TABLE IF NOT EXISTS chat_sessions (
			session_id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			model_id TEXT NOT NULL,
			api_shape TEXT NOT NULL DEFAULT 'openai',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Chat messages table
		`CREATE TABLE IF NOT EXISTS chat_messages (
			message_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (session_id) REFERENCES chat_sessions (session_id) ON DELETE CASCADE
		)`,

		// Virtual models table
		`CREATE TABLE IF NOT EXISTS virtual_models (
			virtual_model_id TEXT PRIMARY KEY,
			name             TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			api_shape        TEXT NOT NULL DEFAULT 'openai',
			lb_strategy      TEXT NOT NULL DEFAULT 'round-robin'
			                 CHECK (lb_strategy IN ('round-robin','random','priority','weighted')),
			enabled          INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Virtual model upstreams table
		`CREATE TABLE IF NOT EXISTS virtual_model_upstreams (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			virtual_model_id TEXT NOT NULL,
			provider_id      TEXT NOT NULL DEFAULT '',
			model_id         TEXT NOT NULL,
			weight           INTEGER NOT NULL DEFAULT 1,
			priority         INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (virtual_model_id)
				REFERENCES virtual_models(virtual_model_id) ON DELETE CASCADE
		)`,
		// Backfill newer columns for existing databases
		`ALTER TABLE virtual_model_upstreams ADD COLUMN provider_id TEXT NOT NULL DEFAULT ''`,
	}

	for _, query := range queries {
		if _, err := db.db.Exec(query); err != nil {
			if strings.Contains(query, "ALTER TABLE") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	// Create indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_provider_id ON provider_instances (provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_priority ON provider_instances (priority)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_activated ON provider_instances (activated)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens (provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_model_states_instance_id ON provider_model_states (instance_id)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_model_states_enabled ON provider_model_states (enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_model_configs_instance_id ON model_configs (instance_id)`,
		`CREATE INDEX IF NOT EXISTS idx_model_configs_model_id ON model_configs (model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id ON chat_messages (session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_model_upstreams_vmodel_id ON virtual_model_upstreams (virtual_model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_models_cache_cached_at ON provider_models_cache (cached_at)`,
	}

	for _, indexQuery := range indexes {
		if _, err := db.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Debug().Msg("Database tables created successfully")
	return nil
}

func (db *Database) Close() error {
	if db.db != nil {
		log.Debug().Msg("Database connection closed")
		return db.db.Close()
	}
	return nil
}
