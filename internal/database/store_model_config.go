package database

import (
	"database/sql"
	"fmt"
	"strconv"
)

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
	versionInt, err := strconv.Atoi(version)
	if err != nil {
		return fmt.Errorf("invalid model version %q: %w", version, err)
	}

	_, err = mcs.db.db.Exec(`
		INSERT INTO model_configs (instance_id, model_id, version, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(instance_id, model_id) DO UPDATE SET
			version = excluded.version,
			updated_at = datetime('now')
	`, instanceID, modelID, versionInt)
	return err
}

func (mcs *ModelConfigStore) Delete(instanceID, modelID string) error {
	_, err := mcs.db.db.Exec(
		"DELETE FROM model_configs WHERE instance_id = ? AND model_id = ?",
		instanceID, modelID,
	)
	return err
}
