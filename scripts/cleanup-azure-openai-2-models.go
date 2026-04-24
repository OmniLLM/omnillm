//go:build ignore

// Run with: go run scripts/cleanup-azure-openai-2-models.go
// Removes stale model_state entries for azure-openai-2 that are not in its deployment config.

package main

import (
	"database/sql"
	"encoding/json"
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

	const instanceID = "azure-openai-2"

	// Get deployment config for this instance
	var configData string
	err = db.QueryRow(`SELECT config_data FROM provider_configs WHERE instance_id = ?`, instanceID).Scan(&configData)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(configData), &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

	// Build set of valid model IDs from deployments
	validModels := map[string]struct{}{}
	if raw, ok := cfg["deployments"]; ok {
		switch v := raw.(type) {
		case []interface{}:
			for _, item := range v {
				switch it := item.(type) {
				case string:
					if it != "" {
						validModels[it] = struct{}{}
					}
				case map[string]interface{}:
					if m, ok := it["model"].(string); ok && m != "" {
						validModels[m] = struct{}{}
					}
					if d, ok := it["deployment"].(string); ok && d != "" {
						validModels[d] = struct{}{}
					}
				}
			}
		}
	}

	fmt.Printf("Valid models from config: %v\n", validModels)

	// List all model states for this instance
	rows, err := db.Query(`SELECT model_id FROM provider_model_states WHERE instance_id = ?`, instanceID)
	if err != nil {
		log.Fatalf("query states: %v", err)
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var modelID string
		if err := rows.Scan(&modelID); err != nil {
			log.Fatalf("scan: %v", err)
		}
		if _, ok := validModels[modelID]; !ok {
			toDelete = append(toDelete, modelID)
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("No stale models found.")
		return
	}

	fmt.Printf("Deleting stale models: %v\n", toDelete)
	for _, modelID := range toDelete {
		if _, err := db.Exec(`DELETE FROM provider_model_states WHERE instance_id = ? AND model_id = ?`, instanceID, modelID); err != nil {
			log.Printf("  WARN: failed to delete %s: %v", modelID, err)
		} else {
			fmt.Printf("  Deleted: %s\n", modelID)
		}
		// Also clear model config
		db.Exec(`DELETE FROM provider_model_configs WHERE instance_id = ? AND model_id = ?`, instanceID, modelID)
		// Also clear models cache so UI refreshes
		db.Exec(`DELETE FROM provider_models_cache WHERE instance_id = ?`, instanceID)
	}

	fmt.Println("Done.")
}
