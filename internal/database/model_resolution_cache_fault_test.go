package database

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func withMockDatabase(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	StopAsyncWorkers()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	previous := globalDB
	globalDB = &Database{db: db}
	t.Cleanup(func() {
		globalDB = previous
		db.Close()
		StartAsyncWorkers()
	})
	return mock
}

func TestVirtualModelStoresReturnLateRowErrors(t *testing.T) {
	iterationErr := errors.New("late row failure")
	t.Run("virtual models", func(t *testing.T) {
		mock := withMockDatabase(t)
		rows := sqlmock.NewRows([]string{"virtual_model_id", "name", "description", "api_shape", "lb_strategy", "enabled", "created_at", "updated_at"}).
			AddRow("vm", "VM", "", "openai", "round_robin", 1, "2026-01-01 00:00:00", "2026-01-01 00:00:00").
			RowError(0, iterationErr)
		mock.ExpectQuery("SELECT virtual_model_id").WillReturnRows(rows)
		if _, err := NewVirtualModelStore().GetAll(); !errors.Is(err, iterationErr) {
			t.Fatalf("expected late row error, got %v", err)
		}
	})

	t.Run("upstreams", func(t *testing.T) {
		mock := withMockDatabase(t)
		rows := sqlmock.NewRows([]string{"id", "virtual_model_id", "provider_id", "model_id", "weight", "priority", "created_at", "updated_at"}).
			AddRow(1, "vm", "provider", "model", 1, 0, "2026-01-01 00:00:00", "2026-01-01 00:00:00").
			RowError(0, iterationErr)
		mock.ExpectQuery("SELECT id, virtual_model_id").WillReturnRows(rows)
		if _, err := NewVirtualModelUpstreamStore().GetForVModel("vm"); !errors.Is(err, iterationErr) {
			t.Fatalf("expected late row error, got %v", err)
		}
	})
}

func TestCacheLoadersDoNotPublishFailedSnapshots(t *testing.T) {
	t.Run("virtual models", func(t *testing.T) {
		mock := withMockDatabase(t)
		mock.ExpectQuery("SELECT virtual_model_id").WillReturnError(errors.New("query failed"))
		cache := &ModelResolutionCache{vmByName: map[string]*VirtualModelRecord{"old": {VirtualModelID: "old"}}}
		cache.ensureVMLoaded()
		if cache.vmLoaded {
			t.Fatal("failed virtual-model load was marked loaded")
		}
		if cache.vmByName["old"] == nil {
			t.Fatal("failed load replaced prior virtual-model snapshot")
		}
	})

	t.Run("provider instances", func(t *testing.T) {
		mock := withMockDatabase(t)
		mock.ExpectQuery("SELECT instance_id").WillReturnError(errors.New("query failed"))
		cache := &ModelResolutionCache{instByID: map[string]ProviderInstanceRecord{"old": {InstanceID: "old"}}}
		cache.ensureInstLoaded()
		if cache.instLoaded {
			t.Fatal("failed instance load was marked loaded")
		}
		if _, ok := cache.instByID["old"]; !ok {
			t.Fatal("failed load replaced prior instance snapshot")
		}
	})

	t.Run("model state", func(t *testing.T) {
		mock := withMockDatabase(t)
		mock.ExpectQuery("SELECT instance_id, model_id").WillReturnError(errors.New("query failed"))
		cache := &ModelStateCache{data: map[string]map[string]bool{"old": {"model": false}}}
		cache.ensure()
		if cache.loaded {
			t.Fatal("failed model-state load was marked loaded")
		}
		if cache.data["old"] == nil {
			t.Fatal("failed load replaced prior model-state snapshot")
		}
	})
}
