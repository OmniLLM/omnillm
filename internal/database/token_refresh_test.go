package database

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestCredentialCompareAndSwapFailureAndConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{{"write-failure", true}, {"newer-snapshot", false}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := &TokenStore{db: &Database{db: db}}
			expectation := mock.ExpectExec("UPDATE tokens").WithArgs(`{"access_token":"new"}`, "provider", "old-snapshot")
			if tc.fail {
				expectation.WillReturnError(errors.New("disk failure"))
			} else {
				expectation.WillReturnResult(sqlmock.NewResult(0, 0))
			}
			saved, err := store.SaveIfUnchanged(context.Background(), "provider", "old-snapshot", map[string]interface{}{"access_token": "new"})
			if saved || (err != nil) != tc.fail {
				t.Fatalf("saved=%v err=%v", saved, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
