package database

import "database/sql"

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
