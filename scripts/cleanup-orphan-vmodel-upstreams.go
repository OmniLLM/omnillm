//go:build ignore

// Run with: go run scripts/cleanup-orphan-vmodel-upstreams.go
// Deletes virtual_model_upstreams rows whose parent virtual_model no longer exists.

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("cannot determine home dir: %v", err)
	}
	dbPath := filepath.Join(home, ".config", "omnillm", "database.sqlite")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Find orphaned rows first (dry-run listing).
	rows, err := db.Query(`
		SELECT u.id, u.virtual_model_id, u.provider_id, u.model_id
		FROM virtual_model_upstreams u
		LEFT JOIN virtual_models v ON v.virtual_model_id = u.virtual_model_id
		WHERE v.virtual_model_id IS NULL
		ORDER BY u.virtual_model_id, u.id
	`)
	if err != nil {
		log.Fatalf("query orphans: %v", err)
	}

	type orphan struct {
		id         int64
		vmID       string
		providerID string
		modelID    string
	}
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.vmID, &o.providerID, &o.modelID); err != nil {
			log.Fatalf("scan: %v", err)
		}
		orphans = append(orphans, o)
	}
	rows.Close()

	if len(orphans) == 0 {
		fmt.Println("No orphaned upstream rows found.")
		return
	}

	fmt.Printf("Found %d orphaned upstream rows:\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  #%d  vmodel=%-35s provider=%-40s model=%s\n", o.id, o.vmID, o.providerID, o.modelID)
	}

	// Delete them all.
	result, err := db.Exec(`
		DELETE FROM virtual_model_upstreams
		WHERE id IN (
			SELECT u.id
			FROM virtual_model_upstreams u
			LEFT JOIN virtual_models v ON v.virtual_model_id = u.virtual_model_id
			WHERE v.virtual_model_id IS NULL
		)
	`)
	if err != nil {
		log.Fatalf("delete orphans: %v", err)
	}
	deleted, _ := result.RowsAffected()
	fmt.Printf("\nDeleted %d orphaned rows.\n", deleted)
}
