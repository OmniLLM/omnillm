// Package database provides SQLite-based persistence for OmniLLM
package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

// parseTime converts a string timestamp to time.Time
// SQLite returns timestamps as strings from modernc.org/sqlite
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339 format first (ISO 8601 with timezone)
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try RFC3339Nano format
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t
	}
	// Try SQLite datetime format
	t, err = time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t
	}
	// Return zero time if parsing fails
	return time.Time{}
}

type Database struct {
	db *sql.DB
}

type ConfigRecord struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProviderInstanceRecord struct {
	InstanceID string    `json:"instance_id"`
	ProviderID string    `json:"provider_id"`
	Name       string    `json:"name"`
	Priority   int       `json:"priority"`
	Activated  bool      `json:"activated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TokenRecord struct {
	InstanceID string    `json:"instance_id"`
	ProviderID string    `json:"provider_id"`
	TokenData  string    `json:"token_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelStateRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ConfigData string    `json:"config_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ModelConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Version    string    `json:"version"`
	ConfigData string    `json:"config_data"` // JSON string - for model-specific configuration
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelsCacheRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelsData string    `json:"models_data"` // JSON array of cached models
	CachedAt   time.Time `json:"cached_at"`
}

type ChatSessionRecord struct {
	SessionID string    `json:"session_id"`
	Title     string    `json:"title"`
	ModelID   string    `json:"model_id"`
	APIShape  string    `json:"api_shape"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessageRecord struct {
	MessageID string    `json:"message_id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type LbStrategy string

const (
	LbStrategyRoundRobin LbStrategy = "round-robin"
	LbStrategyRandom     LbStrategy = "random"
	LbStrategyPriority   LbStrategy = "priority"
	LbStrategyWeighted   LbStrategy = "weighted"
)

type VirtualModelRecord struct {
	VirtualModelID string     `json:"virtual_model_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	APIShape       string     `json:"api_shape"`
	LbStrategy     LbStrategy `json:"lb_strategy"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type VirtualModelUpstreamRecord struct {
	ID             int64     `json:"id"`
	VirtualModelID string    `json:"virtual_model_id"`
	ProviderID     string    `json:"provider_id"`
	ModelID        string    `json:"model_id"`
	Weight         int       `json:"weight"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
}

var globalDB *Database

func InitializeDatabase(configDir string) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "database.sqlite")
	log.Debug().Str("path", dbPath).Msg("Initializing SQLite database")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

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

// Configuration operations
type ConfigStore struct {
	db *Database
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{db: GetDatabase()}
}

func (cs *ConfigStore) Get(key string) (string, error) {
	var value string
	err := cs.db.db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (cs *ConfigStore) Set(key, value string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO config (key, value, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = datetime('now')
	`, key, value)
	return err
}

func (cs *ConfigStore) Delete(key string) error {
	_, err := cs.db.db.Exec("DELETE FROM config WHERE key = ?", key)
	return err
}

func (cs *ConfigStore) GetAll() (map[string]string, error) {
	rows, err := cs.db.db.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

// Provider instance operations
type ProviderInstanceStore struct {
	db *Database
}

func NewProviderInstanceStore() *ProviderInstanceStore {
	return &ProviderInstanceStore{db: GetDatabase()}
}

func (pis *ProviderInstanceStore) Get(instanceID string) (*ProviderInstanceRecord, error) {
	var record ProviderInstanceRecord
	var activated int
	var createdAtStr, updatedAtStr string
	err := pis.db.db.QueryRow(`
		SELECT instance_id, provider_id, name, priority, activated, created_at, updated_at
		FROM provider_instances WHERE instance_id = ?
	`, instanceID).Scan(&record.InstanceID, &record.ProviderID, &record.Name, &record.Priority, &activated, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record.Activated = activated != 0
	record.CreatedAt = parseTime(createdAtStr)
	record.UpdatedAt = parseTime(updatedAtStr)
	return &record, nil
}

func (pis *ProviderInstanceStore) GetAll() ([]ProviderInstanceRecord, error) {
	rows, err := pis.db.db.Query(`
		SELECT instance_id, provider_id, name, priority, activated, created_at, updated_at
		FROM provider_instances ORDER BY priority ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ProviderInstanceRecord
	for rows.Next() {
		var record ProviderInstanceRecord
		var activated int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&record.InstanceID, &record.ProviderID, &record.Name, &record.Priority, &activated, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		record.Activated = activated != 0
		record.CreatedAt = parseTime(createdAtStr)
		record.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, record)
	}
	return records, nil
}

func (pis *ProviderInstanceStore) Save(record *ProviderInstanceRecord) error {
	activated := 0
	if record.Activated {
		activated = 1
	}

	_, err := pis.db.db.Exec(`
		INSERT INTO provider_instances
		(instance_id, provider_id, name, priority, activated, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			provider_id = excluded.provider_id,
			name = excluded.name,
			priority = excluded.priority,
			activated = excluded.activated,
			updated_at = datetime('now')
	`, record.InstanceID, record.ProviderID, record.Name, record.Priority, activated)
	return err
}

func (pis *ProviderInstanceStore) Delete(instanceID string) error {
	_, err := pis.db.db.Exec("DELETE FROM provider_instances WHERE instance_id = ?", instanceID)
	return err
}

func (pis *ProviderInstanceStore) SetActivation(instanceID string, activated bool) error {
	activatedInt := 0
	if activated {
		activatedInt = 1
	}

	_, err := pis.db.db.Exec(`
		UPDATE provider_instances
		SET activated = ?, updated_at = datetime('now')
		WHERE instance_id = ?
	`, activatedInt, instanceID)
	return err
}

// Token operations
type TokenStore struct {
	db *Database
}

func NewTokenStore() *TokenStore {
	return &TokenStore{db: GetDatabase()}
}

func (ts *TokenStore) Get(instanceID string) (*TokenRecord, error) {
	var record TokenRecord
	var createdAtStr, updatedAtStr string
	err := ts.db.db.QueryRow(`
		SELECT instance_id, provider_id, token_data, created_at, updated_at
		FROM tokens WHERE instance_id = ?
	`, instanceID).Scan(&record.InstanceID, &record.ProviderID, &record.TokenData, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record.CreatedAt = parseTime(createdAtStr)
	record.UpdatedAt = parseTime(updatedAtStr)
	return &record, nil
}

func (ts *TokenStore) Save(instanceID, providerID string, tokenData interface{}) error {
	tokenJSON, err := json.Marshal(tokenData)
	if err != nil {
		return err
	}

	_, err = ts.db.db.Exec(`
		INSERT INTO tokens (instance_id, provider_id, token_data, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			provider_id = excluded.provider_id,
			token_data = excluded.token_data,
			updated_at = datetime('now')
	`, instanceID, providerID, string(tokenJSON))
	return err
}

func (ts *TokenStore) Delete(instanceID string) error {
	_, err := ts.db.db.Exec("DELETE FROM tokens WHERE instance_id = ?", instanceID)
	return err
}

func (ts *TokenStore) GetAllByProvider(providerID string) ([]TokenRecord, error) {
	rows, err := ts.db.db.Query(`
		SELECT instance_id, provider_id, token_data, created_at, updated_at
		FROM tokens WHERE provider_id = ?
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TokenRecord
	for rows.Next() {
		var record TokenRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&record.InstanceID, &record.ProviderID, &record.TokenData, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		record.CreatedAt = parseTime(createdAtStr)
		record.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, record)
	}
	return records, nil
}

// Chat operations
type ChatStore struct {
	db *Database
}

func NewChatStore() *ChatStore {
	return &ChatStore{db: GetDatabase()}
}

func (cs *ChatStore) CreateSession(sessionID, title, modelID, apiShape string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO chat_sessions (session_id, title, model_id, api_shape)
		VALUES (?, ?, ?, ?)
	`, sessionID, title, modelID, apiShape)
	return err
}

func (cs *ChatStore) UpdateSessionTitle(sessionID, title string) error {
	_, err := cs.db.db.Exec(`
		UPDATE chat_sessions SET title = ?, updated_at = datetime('now')
		WHERE session_id = ?
	`, title, sessionID)
	return err
}

func (cs *ChatStore) TouchSession(sessionID string) error {
	_, err := cs.db.db.Exec(`
		UPDATE chat_sessions SET updated_at = datetime('now') WHERE session_id = ?
	`, sessionID)
	return err
}

func (cs *ChatStore) ListSessions() ([]ChatSessionRecord, error) {
	rows, err := cs.db.db.Query(`
		SELECT session_id, title, model_id, api_shape, created_at, updated_at
		FROM chat_sessions ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []ChatSessionRecord
	for rows.Next() {
		var session ChatSessionRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&session.SessionID, &session.Title, &session.ModelID, &session.APIShape, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		session.CreatedAt = parseTime(createdAtStr)
		session.UpdatedAt = parseTime(updatedAtStr)
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (cs *ChatStore) GetSession(sessionID string) (*ChatSessionRecord, error) {
	var session ChatSessionRecord
	var createdAtStr, updatedAtStr string
	err := cs.db.db.QueryRow(`
		SELECT session_id, title, model_id, api_shape, created_at, updated_at
		FROM chat_sessions WHERE session_id = ?
	`, sessionID).Scan(&session.SessionID, &session.Title, &session.ModelID, &session.APIShape, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session.CreatedAt = parseTime(createdAtStr)
	session.UpdatedAt = parseTime(updatedAtStr)
	return &session, nil
}

func (cs *ChatStore) DeleteSession(sessionID string) error {
	_, err := cs.db.db.Exec("DELETE FROM chat_sessions WHERE session_id = ?", sessionID)
	return err
}

func (cs *ChatStore) DeleteAllSessions() error {
	_, err := cs.db.db.Exec("DELETE FROM chat_sessions")
	return err
}

func (cs *ChatStore) AddMessage(messageID, sessionID, role, content string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO chat_messages (message_id, session_id, role, content)
		VALUES (?, ?, ?, ?)
	`, messageID, sessionID, role, content)
	return err
}

func (cs *ChatStore) GetMessages(sessionID string) ([]ChatMessageRecord, error) {
	rows, err := cs.db.db.Query(`
		SELECT message_id, session_id, role, content, created_at
		FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessageRecord
	for rows.Next() {
		var message ChatMessageRecord
		var createdAtStr string
		if err := rows.Scan(&message.MessageID, &message.SessionID, &message.Role, &message.Content, &createdAtStr); err != nil {
			return nil, err
		}
		message.CreatedAt = parseTime(createdAtStr)
		messages = append(messages, message)
	}
	return messages, nil
}

// Provider model state operations
type ModelStateStore struct {
	db *Database
}

func NewModelStateStore() *ModelStateStore {
	return &ModelStateStore{db: GetDatabase()}
}

func (ms *ModelStateStore) Get(instanceID, modelID string) (*ProviderModelStateRecord, error) {
	var record ProviderModelStateRecord
	var enabled int
	var createdAtStr, updatedAtStr string
	err := ms.db.db.QueryRow(`
		SELECT instance_id, model_id, enabled, created_at, updated_at
		FROM provider_model_states WHERE instance_id = ? AND model_id = ?
	`, instanceID, modelID).Scan(&record.InstanceID, &record.ModelID, &enabled, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record.Enabled = enabled != 0
	record.CreatedAt = parseTime(createdAtStr)
	record.UpdatedAt = parseTime(updatedAtStr)
	return &record, nil
}

func (ms *ModelStateStore) GetAllByInstance(instanceID string) ([]ProviderModelStateRecord, error) {
	rows, err := ms.db.db.Query(`
		SELECT instance_id, model_id, enabled, created_at, updated_at
		FROM provider_model_states WHERE instance_id = ?
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ProviderModelStateRecord
	for rows.Next() {
		var record ProviderModelStateRecord
		var enabled int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&record.InstanceID, &record.ModelID, &enabled, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		record.Enabled = enabled != 0
		record.CreatedAt = parseTime(createdAtStr)
		record.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, record)
	}
	return records, nil
}

func (ms *ModelStateStore) SetEnabled(instanceID, modelID string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := ms.db.db.Exec(`
		INSERT INTO provider_model_states (instance_id, model_id, enabled, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(instance_id, model_id) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = datetime('now')
	`, instanceID, modelID, enabledInt)
	return err
}

func (ms *ModelStateStore) Delete(instanceID, modelID string) error {
	_, err := ms.db.db.Exec(
		"DELETE FROM provider_model_states WHERE instance_id = ? AND model_id = ?",
		instanceID, modelID,
	)
	return err
}

// Provider config operations
type ProviderConfigStore struct {
	db *Database
}

func NewProviderConfigStore() *ProviderConfigStore {
	return &ProviderConfigStore{db: GetDatabase()}
}

func (pcs *ProviderConfigStore) Get(instanceID string) (*ProviderConfigRecord, error) {
	var record ProviderConfigRecord
	var createdAtStr, updatedAtStr string
	err := pcs.db.db.QueryRow(`
		SELECT instance_id, config_data, created_at, updated_at
		FROM provider_configs WHERE instance_id = ?
	`, instanceID).Scan(&record.InstanceID, &record.ConfigData, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.CreatedAt = parseTime(createdAtStr)
	record.UpdatedAt = parseTime(updatedAtStr)
	return &record, nil
}

func (pcs *ProviderConfigStore) Save(instanceID string, configData map[string]interface{}) error {
	configJSON, err := json.Marshal(configData)
	if err != nil {
		return err
	}

	_, err = pcs.db.db.Exec(`
		INSERT INTO provider_configs (instance_id, config_data, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			config_data = excluded.config_data,
			updated_at = datetime('now')
	`, instanceID, string(configJSON))
	return err
}

func (pcs *ProviderConfigStore) Delete(instanceID string) error {
	_, err := pcs.db.db.Exec("DELETE FROM provider_configs WHERE instance_id = ?", instanceID)
	return err
}

// Model config operations
type ModelConfigStore struct {
	db *Database
}

func NewModelConfigStore() *ModelConfigStore {
	return &ModelConfigStore{db: GetDatabase()}
}

func (mcs *ModelConfigStore) Get(instanceID, modelID string) (*ModelConfigRecord, error) {
	var record ModelConfigRecord
	var createdAtStr, updatedAtStr string
	err := mcs.db.db.QueryRow(`
		SELECT instance_id, model_id, version, config_data, created_at, updated_at
		FROM model_configs WHERE instance_id = ? AND model_id = ?
	`, instanceID, modelID).Scan(&record.InstanceID, &record.ModelID, &record.Version, &record.ConfigData, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.CreatedAt = parseTime(createdAtStr)
	record.UpdatedAt = parseTime(updatedAtStr)
	return &record, nil
}

func (mcs *ModelConfigStore) GetAllByInstance(instanceID string) ([]ModelConfigRecord, error) {
	rows, err := mcs.db.db.Query(`
		SELECT instance_id, model_id, version, config_data, created_at, updated_at
		FROM model_configs WHERE instance_id = ?
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ModelConfigRecord
	for rows.Next() {
		var record ModelConfigRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&record.InstanceID, &record.ModelID, &record.Version, &record.ConfigData, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		record.CreatedAt = parseTime(createdAtStr)
		record.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, record)
	}
	return records, nil
}

func (mcs *ModelConfigStore) SetVersion(instanceID, modelID, version string) error {
	_, err := mcs.db.db.Exec(`
		INSERT INTO model_configs (instance_id, model_id, version, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(instance_id, model_id) DO UPDATE SET
			version = excluded.version,
			updated_at = datetime('now')
	`, instanceID, modelID, version)
	return err
}

func (mcs *ModelConfigStore) Delete(instanceID, modelID string) error {
	_, err := mcs.db.db.Exec(
		"DELETE FROM model_configs WHERE instance_id = ? AND model_id = ?",
		instanceID, modelID,
	)
	return err
}

// ─── Provider models cache operations ─────────────────────────────────────────

// DefaultCacheTTL is the default time-to-live for cached model lists (24 hours).
const DefaultCacheTTL = 24 * time.Hour

type ProviderModelsCacheStore struct {
	db *Database
}

func NewProviderModelsCacheStore() *ProviderModelsCacheStore {
	return &ProviderModelsCacheStore{db: GetDatabase()}
}

// Get returns the cached model list if it exists and is still valid (not expired).
// Returns nil, nil if no cache entry exists or it has expired.
func (cs *ProviderModelsCacheStore) Get(instanceID string, ttl time.Duration) (*ProviderModelsCacheRecord, error) {
	var record ProviderModelsCacheRecord
	var cachedAtStr string
	err := cs.db.db.QueryRow(`
		SELECT instance_id, models_data, cached_at
		FROM provider_models_cache WHERE instance_id = ?
	`, instanceID).Scan(&record.InstanceID, &record.ModelsData, &cachedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record.CachedAt = parseTime(cachedAtStr)

	// Check if cache has expired
	if time.Since(record.CachedAt) > ttl {
		return nil, nil
	}

	return &record, nil
}

// Save stores the model list in the cache.
func (cs *ProviderModelsCacheStore) Save(instanceID string, modelsData string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO provider_models_cache (instance_id, models_data, cached_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			models_data = excluded.models_data,
			cached_at = datetime('now')
	`, instanceID, modelsData)
	return err
}

// Delete removes the cache entry for a provider instance.
func (cs *ProviderModelsCacheStore) Delete(instanceID string) error {
	_, err := cs.db.db.Exec("DELETE FROM provider_models_cache WHERE instance_id = ?", instanceID)
	return err
}

// ─── Virtual model operations ─────────────────────────────────────────────────

type VirtualModelStore struct {
	db *Database
}

func NewVirtualModelStore() *VirtualModelStore {
	return &VirtualModelStore{db: GetDatabase()}
}

func (s *VirtualModelStore) GetAll() ([]VirtualModelRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []VirtualModelRecord
	for rows.Next() {
		var r VirtualModelRecord
		var enabledInt int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt == 1
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, r)
	}
	return records, nil
}

func (s *VirtualModelStore) Get(virtualModelID string) (*VirtualModelRecord, error) {
	var r VirtualModelRecord
	var enabledInt int
	var createdAtStr, updatedAtStr string
	err := s.db.db.QueryRow(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models WHERE virtual_model_id = ?
	`, virtualModelID).Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabledInt == 1
	r.CreatedAt = parseTime(createdAtStr)
	r.UpdatedAt = parseTime(updatedAtStr)
	return &r, nil
}

func (s *VirtualModelStore) Create(r *VirtualModelRecord) error {
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	_, err := s.db.db.Exec(`
		INSERT INTO virtual_models (virtual_model_id, name, description, api_shape, lb_strategy, enabled)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.VirtualModelID, r.Name, r.Description, r.APIShape, string(r.LbStrategy), enabledInt)
	return err
}

func (s *VirtualModelStore) Update(r *VirtualModelRecord) error {
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	_, err := s.db.db.Exec(`
		UPDATE virtual_models
		SET name = ?, description = ?, api_shape = ?, lb_strategy = ?, enabled = ?, updated_at = datetime('now')
		WHERE virtual_model_id = ?
	`, r.Name, r.Description, r.APIShape, string(r.LbStrategy), enabledInt, r.VirtualModelID)
	return err
}

func (s *VirtualModelStore) Delete(virtualModelID string) error {
	_, err := s.db.db.Exec("DELETE FROM virtual_models WHERE virtual_model_id = ?", virtualModelID)
	return err
}

// ─── Virtual model upstream operations ───────────────────────────────────────

type VirtualModelUpstreamStore struct {
	db *Database
}

func NewVirtualModelUpstreamStore() *VirtualModelUpstreamStore {
	return &VirtualModelUpstreamStore{db: GetDatabase()}
}

func (s *VirtualModelUpstreamStore) GetForVModel(virtualModelID string) ([]VirtualModelUpstreamRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT id, virtual_model_id, provider_id, model_id, weight, priority, created_at
		FROM virtual_model_upstreams
		WHERE virtual_model_id = ?
		ORDER BY priority ASC, id ASC
	`, virtualModelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []VirtualModelUpstreamRecord
	for rows.Next() {
		var r VirtualModelUpstreamRecord
		var createdAtStr string
		if err := rows.Scan(&r.ID, &r.VirtualModelID, &r.ProviderID, &r.ModelID, &r.Weight, &r.Priority, &createdAtStr); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAtStr)
		records = append(records, r)
	}
	return records, nil
}

// SetForVModel atomically replaces all upstreams for a virtual model.
func (s *VirtualModelUpstreamStore) SetForVModel(virtualModelID string, upstreams []VirtualModelUpstreamRecord) error {
	tx, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM virtual_model_upstreams WHERE virtual_model_id = ?", virtualModelID); err != nil {
		return err
	}
	for _, u := range upstreams {
		if _, err := tx.Exec(`
			INSERT INTO virtual_model_upstreams (virtual_model_id, provider_id, model_id, weight, priority)
			VALUES (?, ?, ?, ?, ?)
		`, virtualModelID, u.ProviderID, u.ModelID, u.Weight, u.Priority); err != nil {
			return err
		}
	}
	return tx.Commit()
}
