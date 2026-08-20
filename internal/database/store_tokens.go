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
		SELECT instance_id, token_data, created_at, updated_at
		FROM tokens WHERE instance_id = ?
	`, instanceID).Scan(&record.InstanceID, &record.TokenData, &createdAtStr, &updatedAtStr)

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

func (ts *TokenStore) Save(instanceID string, tokenData interface{}) error {
	tokenJSON, err := json.Marshal(tokenData)
	if err != nil {
		return err
	}

	_, err = ts.db.db.Exec(`
		INSERT INTO tokens (instance_id, token_data, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			token_data = excluded.token_data,
			updated_at = datetime('now')
	`, instanceID, string(tokenJSON))
	return err
}

// ClearRefreshTokenIfMatches atomically retires a rejected OAuth refresh token
// without overwriting a newer token persisted by another provider process.
func (ts *TokenStore) ClearRefreshTokenIfMatches(instanceID, rejected string) (bool, error) {
	result, err := ts.db.db.Exec(`
		UPDATE tokens
		SET token_data = json_set(token_data, '$.refresh_token', ''),
			updated_at = datetime('now')
		WHERE instance_id = ?
			AND json_extract(token_data, '$.refresh_token') = ?
	`, instanceID, rejected)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (ts *TokenStore) Delete(instanceID string) error {
	_, err := ts.db.db.Exec("DELETE FROM tokens WHERE instance_id = ?", instanceID)
	return err
}

// GetAllByProvider returns all token records for a given provider type.
// Joins provider_instances to avoid relying on the deprecated provider_id column in tokens.
func (ts *TokenStore) GetAllByProvider(providerID string) ([]TokenRecord, error) {
	rows, err := ts.db.db.Query(`
		SELECT t.instance_id, t.token_data, t.created_at, t.updated_at
		FROM tokens t
		JOIN provider_instances pi ON pi.instance_id = t.instance_id
		WHERE pi.provider_id = ?
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TokenRecord
	for rows.Next() {
		var record TokenRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&record.InstanceID, &record.TokenData, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		record.CreatedAt = parseTime(createdAtStr)
		record.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, record)
	}
	return records, rows.Err()
}
