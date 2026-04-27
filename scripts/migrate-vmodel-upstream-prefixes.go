//go:build ignore

// Run with: go run scripts/migrate-vmodel-upstream-prefixes.go
//
// Backfills the provider-prefix on existing virtual_model_upstreams rows.
//
// When a virtual model upstream has a provider_id, the model_id is stored
// as "<subtitle>/<modelID>" (or "<instanceID>/<modelID>" when no subtitle is
// set) so each row is self-describing and collisions between providers that
// expose the same model name are avoided.
//
// This migration is idempotent: rows whose model_id already contains a "/"
// are left unchanged.

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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

	// Load all provider instances so we can look up subtitle by instance_id.
	type instance struct {
		instanceID string
		subtitle   string
	}
	instanceMap := map[string]instance{}
	rows, err := db.Query(`SELECT instance_id, subtitle FROM provider_instances`)
	if err != nil {
		log.Fatalf("query provider_instances: %v", err)
	}
	for rows.Next() {
		var inst instance
		if err := rows.Scan(&inst.instanceID, &inst.subtitle); err != nil {
			log.Fatalf("scan provider_instances: %v", err)
		}
		instanceMap[inst.instanceID] = inst
	}
	rows.Close()
	fmt.Printf("Loaded %d provider instances\n", len(instanceMap))

	// Load all virtual_model_upstreams rows.
	type upstream struct {
		id         int64
		vmID       string
		providerID string
		modelID    string
	}
	var upstreams []upstream
	rows, err = db.Query(`SELECT id, virtual_model_id, provider_id, model_id FROM virtual_model_upstreams`)
	if err != nil {
		log.Fatalf("query virtual_model_upstreams: %v", err)
	}
	for rows.Next() {
		var u upstream
		if err := rows.Scan(&u.id, &u.vmID, &u.providerID, &u.modelID); err != nil {
			log.Fatalf("scan virtual_model_upstreams: %v", err)
		}
		upstreams = append(upstreams, u)
	}
	rows.Close()
	fmt.Printf("Loaded %d upstream rows\n", len(upstreams))

	updated := 0
	skipped := 0
	for _, u := range upstreams {
		// Skip rows with no provider binding or already-prefixed model IDs.
		if u.providerID == "" || strings.Contains(u.modelID, "/") {
			skipped++
			continue
		}

		// Determine the prefix: subtitle if set, otherwise instance_id.
		prefix := u.providerID
		if inst, ok := instanceMap[u.providerID]; ok {
			if inst.subtitle != "" {
				prefix = inst.subtitle
			} else {
				prefix = inst.instanceID
			}
		}

		newModelID := prefix + "/" + u.modelID
		fmt.Printf("  [%s] upstream #%d: %q → %q\n", u.vmID, u.id, u.modelID, newModelID)

		if _, err := db.Exec(
			`UPDATE virtual_model_upstreams SET model_id = ? WHERE id = ?`,
			newModelID, u.id,
		); err != nil {
			log.Printf("  WARN: failed to update row %d: %v", u.id, err)
		} else {
			updated++
		}
	}

	fmt.Printf("\nDone. Updated: %d, Skipped (no provider or already prefixed): %d\n", updated, skipped)
}
