package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProviderIdentityTransferPreservesOwnedRecords(t *testing.T) {
	db := GetDatabase().db
	const oldID = "identity-transfer-old"
	const newID = "identity-transfer-new"
	const virtualID = "identity-transfer-vm"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM virtual_models WHERE virtual_model_id = ?`, virtualID)
		_, _ = db.Exec(`DELETE FROM provider_instances WHERE instance_id IN (?, ?)`, oldID, newID)
	})

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO provider_instances (instance_id, provider_id, name, subtitle, priority, activated) VALUES (?, 'alibaba', 'Name', 'Alias', 7, 1)`, []any{oldID}},
		{`INSERT INTO tokens (instance_id, token_data) VALUES (?, '{"token":"secret"}')`, []any{oldID}},
		{`INSERT INTO provider_configs (instance_id, config_data) VALUES (?, '{"region":"global"}')`, []any{oldID}},
		{`INSERT INTO provider_model_states (instance_id, model_id, enabled) VALUES (?, 'model', 0)`, []any{oldID}},
		{`INSERT INTO model_configs (instance_id, model_id, version, config_data) VALUES (?, 'model', 3, '{}')`, []any{oldID}},
		{`INSERT INTO provider_models_cache (instance_id, provider_id, models_data) VALUES (?, 'alibaba', '[]')`, []any{oldID}},
		{`INSERT INTO virtual_models (virtual_model_id, name, lb_strategy) VALUES (?, 'VM', 'priority')`, []any{virtualID}},
		{`INSERT INTO virtual_model_upstreams (virtual_model_id, provider_id, model_id) VALUES (?, ?, 'identity-transfer-old/native')`, []any{virtualID, oldID}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	if err := NewProviderInstanceStore().RenameIdentity(context.Background(), oldID, newID); err != nil {
		t.Fatalf("transfer identity: %v", err)
	}
	for _, table := range []string{"tokens", "provider_configs", "provider_model_states", "model_configs", "provider_models_cache"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE instance_id = ?`, newID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s new ownership: %d, %v", table, count, err)
		}
	}
	var id, model string
	if err := db.QueryRow(`SELECT provider_id, model_id FROM virtual_model_upstreams WHERE virtual_model_id = ?`, virtualID).Scan(&id, &model); err != nil {
		t.Fatalf("read upstream: %v", err)
	}
	if id != newID || model != "identity-transfer-old/native" {
		t.Fatalf("upstream identity/model: %q, %q", id, model)
	}
	old, err := NewProviderInstanceStore().Get(oldID)
	if err != nil || old != nil {
		t.Fatalf("old provider: %#v, %v", old, err)
	}
	record, err := NewProviderInstanceStore().Get(newID)
	if err != nil || record == nil || record.Name != "Name" || record.Subtitle != "Alias" || record.Priority != 7 || !record.Activated {
		t.Fatalf("new provider: %#v, %v", record, err)
	}
}

func TestProviderIdentityTransferCollisionLeavesStateIntact(t *testing.T) {
	db := GetDatabase().db
	const oldID = "identity-collision-source"
	const newID = "identity-collision-target"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM provider_instances WHERE instance_id IN (?, ?)`, oldID, newID)
	})
	for _, id := range []string{oldID, newID} {
		if _, err := db.Exec(
			`INSERT INTO provider_instances (instance_id, provider_id, name) VALUES (?, 'alibaba', ?)`, id, id,
		); err != nil {
			t.Fatalf("seed provider: %v", err)
		}
	}
	if err := NewTokenStore().Save(oldID, map[string]string{"token": "original"}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	if err := NewProviderInstanceStore().RenameIdentity(context.Background(), oldID, newID); err == nil {
		t.Fatal("expected collision to fail")
	}
	for _, id := range []string{oldID, newID} {
		record, err := NewProviderInstanceStore().Get(id)
		if err != nil || record == nil || record.Name != id {
			t.Fatalf("provider %q after collision: %#v, %v", id, record, err)
		}
	}
	token, err := NewTokenStore().Get(oldID)
	if err != nil || token == nil || token.TokenData != `{"token":"original"}` {
		t.Fatalf("source token after collision: %#v, %v", token, err)
	}
}

func TestProviderIdentityTransferRollsBackOnChildFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := &ProviderInstanceStore{db: &Database{db: db}}
	wantErr := errors.New("child update failed")
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO provider_instances").WithArgs("new", "old").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT id, model_id").WithArgs(upstreamIdentityHintKeyPrefix, "old").WillReturnRows(sqlmock.NewRows([]string{"id", "model_id", "value"}))
	mock.ExpectExec("UPDATE tokens").WithArgs("new", "old").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE provider_configs").WithArgs("new", "old").WillReturnError(wantErr)
	mock.ExpectRollback()

	if err := store.RenameIdentity(context.Background(), "old", "new"); !errors.Is(err, wantErr) {
		t.Fatalf("transfer error = %v, want child update failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}

func TestVirtualUpstreamIdentityHintsAreRemovedWithUpstreams(t *testing.T) {
	const oldID = "hint-cleanup-old"
	const newID = "hint-cleanup-new"
	const lastID = "hint-cleanup-last"
	const vmID = "hint-cleanup-vm"
	store := NewProviderInstanceStore()
	models := NewVirtualModelStore()
	upstreams := NewVirtualModelUpstreamStore()
	config := NewConfigStore()
	t.Cleanup(func() {
		_ = models.Delete(vmID)
		_ = store.Delete(oldID)
		_ = store.Delete(newID)
		_ = store.Delete(lastID)
	})
	if err := store.Save(&ProviderInstanceRecord{InstanceID: oldID, ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if err := models.Create(&VirtualModelRecord{VirtualModelID: vmID, Name: "VM", APIShape: "openai", LbStrategy: LbStrategyPriority}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}
	if err := upstreams.SetForVModel(vmID, []VirtualModelUpstreamRecord{{ProviderID: oldID, ModelID: oldID + "/model", Weight: 1}}); err != nil {
		t.Fatalf("save upstream: %v", err)
	}
	if err := store.RenameIdentity(context.Background(), oldID, newID); err != nil {
		t.Fatalf("rename provider: %v", err)
	}
	rows, err := upstreams.GetForVModel(vmID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("get upstreams: %#v, %v", rows, err)
	}
	key := upstreamIdentityHintKey(rows[0].ID)
	if value, err := config.Get(key); err != nil || value == "" {
		t.Fatalf("expected identity hint: %q, %v", value, err)
	}
	if err := upstreams.SetForVModel(vmID, []VirtualModelUpstreamRecord{{ProviderID: newID, ModelID: newID + "/model", Weight: 1}}); err != nil {
		t.Fatalf("replace upstream: %v", err)
	}
	if value, err := config.Get(key); err != nil || value != "" {
		t.Fatalf("old identity hint not removed: %q, %v", value, err)
	}
	if err := store.RenameIdentity(context.Background(), newID, lastID); err != nil {
		t.Fatalf("rename provider again: %v", err)
	}
	rows, err = upstreams.GetForVModel(vmID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("get replacement upstreams: %#v, %v", rows, err)
	}
	key = upstreamIdentityHintKey(rows[0].ID)
	if value, err := config.Get(key); err != nil || value == "" {
		t.Fatalf("expected replacement identity hint: %q, %v", value, err)
	}
	if err := models.Delete(vmID); err != nil {
		t.Fatalf("delete virtual model: %v", err)
	}
	if value, err := config.Get(key); err != nil || value != "" {
		t.Fatalf("deleted model left identity hint: %q, %v", value, err)
	}
}

func TestProviderIdentityTransferSurvivesDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.sqlite")
	open := func() *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", sqliteDSN(path))
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		db.SetMaxOpenConns(1)
		return db
	}
	first := open()
	db := &Database{db: first}
	if err := db.createTables(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	statements := []string{
		`INSERT INTO provider_instances (instance_id, provider_id, name, created_at) VALUES ('old-reopen', 'mock', 'Old', '2020-01-02 03:04:05')`,
		`INSERT INTO provider_instances (instance_id, provider_id, name) VALUES ('unrelated', 'mock', 'Unrelated')`,
		`INSERT INTO tokens (instance_id, token_data) VALUES ('old-reopen', '{"token":"secret"}')`,
		`INSERT INTO request_logs (request_id, provider_id) VALUES ('historic-identity', 'old-reopen')`,
		`INSERT INTO virtual_models (virtual_model_id, name, lb_strategy) VALUES ('reopen-vm', 'VM', 'priority')`,
		`INSERT INTO virtual_model_upstreams (virtual_model_id, provider_id, model_id) VALUES ('reopen-vm', 'old-reopen', 'old-reopen/model')`,
	}
	for _, query := range statements {
		if _, err := first.Exec(query); err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}
	store := &ProviderInstanceStore{db: db}
	if err := store.RenameIdentity(context.Background(), "old-reopen", "new-reopen"); err != nil {
		t.Fatalf("rename identity: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	second := open()
	defer second.Close()
	if err := (&Database{db: second}).createTables(); err != nil {
		t.Fatalf("reopen and migrate database: %v", err)
	}
	var created, token, history, provider, model, hint string
	if err := second.QueryRow(`SELECT created_at FROM provider_instances WHERE instance_id = 'new-reopen'`).Scan(&created); err != nil || !parseTime(created).Equal(time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("preserved creation time = %q, %v", created, err)
	}
	if err := second.QueryRow(`SELECT token_data FROM tokens WHERE instance_id = 'new-reopen'`).Scan(&token); err != nil || token != `{"token":"secret"}` {
		t.Fatalf("reopened token = %q, %v", token, err)
	}
	if err := second.QueryRow(`SELECT provider_id FROM request_logs WHERE request_id = 'historic-identity'`).Scan(&history); err != nil || history != "old-reopen" {
		t.Fatalf("historical provider = %q, %v", history, err)
	}
	if err := second.QueryRow(`SELECT provider_id, model_id FROM virtual_model_upstreams WHERE virtual_model_id = 'reopen-vm'`).Scan(&provider, &model); err != nil || provider != "new-reopen" || model != "old-reopen/model" {
		t.Fatalf("reopened upstream = %q, %q, %v", provider, model, err)
	}
	if err := second.QueryRow(`SELECT value FROM config WHERE key = ? || (SELECT id FROM virtual_model_upstreams WHERE virtual_model_id = 'reopen-vm')`, upstreamIdentityHintKeyPrefix).Scan(&hint); err != nil {
		t.Fatalf("reopened hint: %v", err)
	}
	if _, err := decodeUpstreamIdentityHint(hint, "new-reopen", "old-reopen/model"); err != nil {
		t.Fatalf("reopened hint invalid: %v", err)
	}
	var unrelated string
	if err := second.QueryRow(`SELECT name FROM provider_instances WHERE instance_id = 'unrelated'`).Scan(&unrelated); err != nil || unrelated != "Unrelated" {
		t.Fatalf("unrelated provider = %q, %v", unrelated, err)
	}
}
