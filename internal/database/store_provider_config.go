package database

import (
	"database/sql"
	"encoding/json"
)

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
