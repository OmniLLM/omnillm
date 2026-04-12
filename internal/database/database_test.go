package database

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "database-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestParseTimeSupportsKnownFormats(t *testing.T) {
	tests := []string{
		"2026-04-12T10:11:12Z",
		"2026-04-12T10:11:12.123456789Z",
		"2026-04-12 10:11:12",
	}

	for _, input := range tests {
		if got := parseTime(input); got.IsZero() {
			t.Fatalf("expected %q to parse", input)
		}
	}
	if got := parseTime("not-a-time"); !got.IsZero() {
		t.Fatal("expected invalid time to return zero value")
	}
}

func TestConfigStoreCRUD(t *testing.T) {
	store := NewConfigStore()
	if err := store.Set("theme", "dark"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	value, err := store.Get("theme")
	if err != nil || value != "dark" {
		t.Fatalf("expected dark, got %q err=%v", value, err)
	}
	all, err := store.GetAll()
	if err != nil || all["theme"] != "dark" {
		t.Fatalf("expected theme in GetAll, got %#v err=%v", all, err)
	}
	if err := store.Delete("theme"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	value, err = store.Get("theme")
	if err != nil || value != "" {
		t.Fatalf("expected deleted key to return empty string, got %q err=%v", value, err)
	}
}

func TestProviderInstanceStoreCRUD(t *testing.T) {
	store := NewProviderInstanceStore()
	record := &ProviderInstanceRecord{InstanceID: "provider-1", ProviderID: "mock", Name: "Mock", Priority: 7, Activated: true}
	if err := store.Save(record); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := store.Get("provider-1")
	if err != nil || got == nil {
		t.Fatalf("get failed: %#v err=%v", got, err)
	}
	if got.Priority != 7 || !got.Activated {
		t.Fatalf("unexpected record: %#v", got)
	}
	if err := store.SetActivation("provider-1", false); err != nil {
		t.Fatalf("set activation failed: %v", err)
	}
	got, _ = store.Get("provider-1")
	if got.Activated {
		t.Fatal("expected provider to be deactivated")
	}
	all, err := store.GetAll()
	if err != nil || len(all) == 0 {
		t.Fatalf("expected provider in GetAll, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	got, err = store.Get("provider-1")
	if err != nil || got != nil {
		t.Fatalf("expected deleted provider to be nil, got %#v err=%v", got, err)
	}
}

func TestTokenStoreCRUD(t *testing.T) {
	store := NewTokenStore()
	if err := store.Save("provider-1", "mock", map[string]any{"token": "abc", "expires": 123}); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := store.Get("provider-1")
	if err != nil || got == nil {
		t.Fatalf("get failed: %#v err=%v", got, err)
	}
	if got.ProviderID != "mock" {
		t.Fatalf("unexpected provider id: %#v", got)
	}
	all, err := store.GetAllByProvider("mock")
	if err != nil || len(all) == 0 {
		t.Fatalf("expected token in GetAllByProvider, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestChatStoreCRUD(t *testing.T) {
	store := NewChatStore()
	if err := store.CreateSession("session-1", "Title", "model-1", "openai"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := store.AddMessage("msg-1", "session-1", "user", "hello"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}
	session, err := store.GetSession("session-1")
	if err != nil || session == nil {
		t.Fatalf("get session failed: %#v err=%v", session, err)
	}
	if err := store.UpdateSessionTitle("session-1", "Updated"); err != nil {
		t.Fatalf("update title failed: %v", err)
	}
	if err := store.TouchSession("session-1"); err != nil {
		t.Fatalf("touch session failed: %v", err)
	}
	messages, err := store.GetMessages("session-1")
	if err != nil || len(messages) != 1 {
		t.Fatalf("expected one message, got %#v err=%v", messages, err)
	}
	sessions, err := store.ListSessions()
	if err != nil || len(sessions) == 0 {
		t.Fatalf("expected sessions, got %#v err=%v", sessions, err)
	}
	if err := store.DeleteSession("session-1"); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}
}

func TestModelStateStoreCRUD(t *testing.T) {
	instanceStore := NewProviderInstanceStore()
	if err := instanceStore.Save(&ProviderInstanceRecord{InstanceID: "provider-ms", ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider failed: %v", err)
	}
	store := NewModelStateStore()
	if err := store.SetEnabled("provider-ms", "model-1", true); err != nil {
		t.Fatalf("set enabled failed: %v", err)
	}
	got, err := store.Get("provider-ms", "model-1")
	if err != nil || got == nil || !got.Enabled {
		t.Fatalf("unexpected model state: %#v err=%v", got, err)
	}
	all, err := store.GetAllByInstance("provider-ms")
	if err != nil || len(all) != 1 {
		t.Fatalf("expected one model state, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-ms", "model-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestVirtualModelStoresCRUD(t *testing.T) {
	vmStore := NewVirtualModelStore()
	upstreamStore := NewVirtualModelUpstreamStore()

	record := &VirtualModelRecord{
		VirtualModelID: "vm-1",
		Name:           "VM 1",
		Description:    "test",
		APIShape:       "openai",
		LbStrategy:     LbStrategyWeighted,
		Enabled:        true,
	}
	if err := vmStore.Create(record); err != nil {
		t.Fatalf("create virtual model failed: %v", err)
	}
	got, err := vmStore.Get("vm-1")
	if err != nil || got == nil || got.Name != "VM 1" {
		t.Fatalf("unexpected virtual model: %#v err=%v", got, err)
	}
	record.Name = "VM 1 updated"
	record.Enabled = false
	if err := vmStore.Update(record); err != nil {
		t.Fatalf("update virtual model failed: %v", err)
	}
	all, err := vmStore.GetAll()
	if err != nil || len(all) == 0 {
		t.Fatalf("expected virtual models, got %#v err=%v", all, err)
	}
	upstreams := []VirtualModelUpstreamRecord{
		{ProviderID: "provider-a", ModelID: "model-a", Weight: 2, Priority: 1},
		{ProviderID: "provider-b", ModelID: "model-b", Weight: 1, Priority: 2},
	}
	if err := upstreamStore.SetForVModel("vm-1", upstreams); err != nil {
		t.Fatalf("set upstreams failed: %v", err)
	}
	gotUpstreams, err := upstreamStore.GetForVModel("vm-1")
	if err != nil || len(gotUpstreams) != 2 {
		t.Fatalf("expected two upstreams, got %#v err=%v", gotUpstreams, err)
	}
	if gotUpstreams[0].ProviderID != "provider-a" || gotUpstreams[1].ProviderID != "provider-b" {
		t.Fatalf("unexpected upstream order: %#v", gotUpstreams)
	}
	if err := vmStore.Delete("vm-1"); err != nil {
		t.Fatalf("delete virtual model failed: %v", err)
	}
}

func TestCreateTablesIsIdempotentForBackfill(t *testing.T) {
	db := GetDatabase()
	if err := db.createTables(); err != nil {
		t.Fatalf("first createTables call failed: %v", err)
	}
	if err := db.createTables(); err != nil {
		t.Fatalf("second createTables call failed: %v", err)
	}
}

func TestParseTimeZeroValueForEmptyString(t *testing.T) {
	if got := parseTime(""); !got.Equal(time.Time{}) {
		t.Fatal("expected empty string to return zero time")
	}
}
