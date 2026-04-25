package database

import "database/sql"

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
