package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const upstreamIdentityHintKeyPrefix = "internal.provider-identity.upstream."

type upstreamIdentityHint struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Prefix     string `json:"prefix"`
}

func upstreamIdentityHintKey(id int64) string {
	return upstreamIdentityHintKeyPrefix + strconv.FormatInt(id, 10)
}

func decodeUpstreamIdentityHint(raw, providerID, modelID string) (*upstreamIdentityHint, error) {
	var hint upstreamIdentityHint
	if err := json.Unmarshal([]byte(raw), &hint); err != nil {
		return nil, fmt.Errorf("invalid virtual upstream identity hint: %w", err)
	}
	if hint.ProviderID != providerID || hint.ModelID != modelID || hint.Prefix == "" ||
		strings.Contains(hint.Prefix, "/") || !strings.HasPrefix(modelID, hint.Prefix+"/") ||
		len(modelID) == len(hint.Prefix)+1 {
		return nil, fmt.Errorf("virtual upstream identity hint does not match the upstream")
	}
	return &hint, nil
}

// FormerQualifier returns a former provider qualifier for this exact upstream.
func (u VirtualModelUpstreamRecord) FormerQualifier() (string, bool, error) {
	if u.IdentityHint == nil {
		return "", false, nil
	}
	hint, err := decodeUpstreamIdentityHint(*u.IdentityHint, u.ProviderID, u.ModelID)
	if err != nil {
		return "", false, err
	}
	return hint.Prefix, true, nil
}

func transferUpstreamIdentityHints(ctx context.Context, tx *sql.Tx, oldID, newID string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, model_id, (SELECT value FROM config WHERE key = ? || virtual_model_upstreams.id)
		FROM virtual_model_upstreams WHERE provider_id = ?`, upstreamIdentityHintKeyPrefix, oldID)
	if err != nil {
		return fmt.Errorf("read virtual upstream identity hints: %w", err)
	}
	defer rows.Close()
	type upstream struct {
		id      int64
		modelID string
		hint    sql.NullString
	}
	upstreams := []upstream{}
	for rows.Next() {
		var item upstream
		if err := rows.Scan(&item.id, &item.modelID, &item.hint); err != nil {
			return fmt.Errorf("read virtual upstream identity: %w", err)
		}
		upstreams = append(upstreams, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate virtual upstream identities: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close virtual upstream identities: %w", err)
	}
	for _, item := range upstreams {
		var hint *upstreamIdentityHint
		if item.hint.Valid {
			hint, err = decodeUpstreamIdentityHint(item.hint.String, oldID, item.modelID)
			if err != nil {
				return err
			}
		} else if !strings.Contains(oldID, "/") && strings.HasPrefix(item.modelID, oldID+"/") && len(item.modelID) > len(oldID)+1 {
			hint = &upstreamIdentityHint{ModelID: item.modelID, Prefix: oldID}
		}
		if hint == nil {
			continue
		}
		hint.ProviderID = newID
		encoded, err := json.Marshal(hint)
		if err != nil {
			return fmt.Errorf("encode virtual upstream identity hint: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO config (key, value, updated_at) VALUES (?, ?, datetime('now'))
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`,
			upstreamIdentityHintKey(item.id), string(encoded)); err != nil {
			return fmt.Errorf("save virtual upstream identity hint: %w", err)
		}
	}
	return nil
}

func deleteUpstreamIdentityHints(tx *sql.Tx, virtualModelID string) error {
	_, err := tx.Exec(`
		DELETE FROM config WHERE key IN (
			SELECT ? || id FROM virtual_model_upstreams WHERE virtual_model_id = ?
		)`, upstreamIdentityHintKeyPrefix, virtualModelID)
	if err != nil {
		return fmt.Errorf("delete virtual upstream identity hints: %w", err)
	}
	return nil
}
