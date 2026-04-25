package database

import (
	"database/sql"
	"encoding/json"
)

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
