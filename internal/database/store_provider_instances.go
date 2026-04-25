package database

import "database/sql"

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
