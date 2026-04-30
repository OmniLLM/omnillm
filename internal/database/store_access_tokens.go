package database

import (
	"database/sql"
	"time"
)

// AccessTokenStore provides CRUD operations for the access_tokens table.
type AccessTokenStore struct {
	db *Database
}

func NewAccessTokenStore() *AccessTokenStore {
	return &AccessTokenStore{db: GetDatabase()}
}

// List returns all access tokens.
func (s *AccessTokenStore) List() ([]AccessTokenRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT id, name, token_hash, token_plaintext, prefix, created_at, expires_at, last_used_at, enabled
		FROM access_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AccessTokenRecord
	for rows.Next() {
		r, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Create inserts a new access token record.
func (s *AccessTokenStore) Create(id, name, tokenHash, tokenPlaintext, prefix string, expiresAt *time.Time) error {
	_, err := s.db.db.Exec(`
		INSERT INTO access_tokens (id, name, token_hash, token_plaintext, prefix, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, name, tokenHash, tokenPlaintext, prefix, expiresAt)
	return err
}

// Get retrieves a single access token by ID.
func (s *AccessTokenStore) Get(id string) (*AccessTokenRecord, error) {
	row := s.db.db.QueryRow(`
		SELECT id, name, token_hash, token_plaintext, prefix, created_at, expires_at, last_used_at, enabled
		FROM access_tokens WHERE id = ?
	`, id)

	var rec AccessTokenRecord
	var createdAtStr, prefixStr string
	var expiresAtStr, lastUsedAtStr sql.NullString
	var enabled int

	err := row.Scan(&rec.ID, &rec.Name, &rec.TokenHash, &rec.TokenPlaintext, &prefixStr,
		&createdAtStr, &expiresAtStr, &lastUsedAtStr, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rec.Prefix = prefixStr
	rec.Enabled = enabled == 1
	rec.CreatedAt = parseTime(createdAtStr)
	if expiresAtStr.Valid {
		t := parseTime(expiresAtStr.String)
		rec.ExpiresAt = &t
	}
	if lastUsedAtStr.Valid {
		t := parseTime(lastUsedAtStr.String)
		rec.LastUsedAt = &t
	}
	return &rec, nil
}

// Delete removes an access token by ID.
func (s *AccessTokenStore) Delete(id string) error {
	_, err := s.db.db.Exec("DELETE FROM access_tokens WHERE id = ?", id)
	return err
}

// ValidateByHash looks up an access token by its SHA-256 hash.
// Returns nil if not found, disabled, or expired.
func (s *AccessTokenStore) ValidateByHash(tokenHash string) (*AccessTokenRecord, error) {
	row := s.db.db.QueryRow(`
		SELECT id, name, token_hash, token_plaintext, prefix, created_at, expires_at, last_used_at, enabled
		FROM access_tokens WHERE token_hash = ? AND enabled = 1
	`, tokenHash)

	var rec AccessTokenRecord
	var createdAtStr, prefixStr string
	var expiresAtStr, lastUsedAtStr sql.NullString
	var enabled int

	err := row.Scan(&rec.ID, &rec.Name, &rec.TokenHash, &rec.TokenPlaintext, &prefixStr,
		&createdAtStr, &expiresAtStr, &lastUsedAtStr, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rec.Prefix = prefixStr
	rec.Enabled = enabled == 1
	rec.CreatedAt = parseTime(createdAtStr)
	if expiresAtStr.Valid {
		t := parseTime(expiresAtStr.String)
		if t.Before(time.Now()) {
			return nil, nil
		}
		rec.ExpiresAt = &t
	}
	if lastUsedAtStr.Valid {
		t := parseTime(lastUsedAtStr.String)
		rec.LastUsedAt = &t
	}

	go func() {
		_, _ = s.db.db.Exec(`UPDATE access_tokens SET last_used_at = datetime('now') WHERE id = ?`, rec.ID)
	}()

	return &rec, nil
}

// scanAccessToken scans an access token from a row set.
func scanAccessToken(rows *sql.Rows) (AccessTokenRecord, error) {
	var rec AccessTokenRecord
	var createdAtStr, prefixStr string
	var expiresAtStr, lastUsedAtStr sql.NullString
	var enabled int

	err := rows.Scan(&rec.ID, &rec.Name, &rec.TokenHash, &rec.TokenPlaintext, &prefixStr,
		&createdAtStr, &expiresAtStr, &lastUsedAtStr, &enabled)
	if err != nil {
		return rec, err
	}

	rec.Prefix = prefixStr
	rec.Enabled = enabled == 1
	rec.CreatedAt = parseTime(createdAtStr)
	if expiresAtStr.Valid {
		t := parseTime(expiresAtStr.String)
		rec.ExpiresAt = &t
	}
	if lastUsedAtStr.Valid {
		t := parseTime(lastUsedAtStr.String)
		rec.LastUsedAt = &t
	}
	return rec, nil
}
