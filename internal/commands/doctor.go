package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// DoctorCmd checks the local environment and server health for the operator.
var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and server health",
	Long: `Check the local OmniLLM configuration and running server health.

Verifies:
  - Config directory and database presence
  - Server reachability
  - API key configuration
  - Provider and virtual model counts
  - In-progress authentication flows

Prints a recommended next action at the end.`,
	Example: `  omnillm doctor
  omnillm doctor --server http://127.0.0.1:5000`,
	RunE: runDoctor,
}

type checkRow struct {
	ok    bool
	label string
	value string
}

func renderChecksTable(out io.Writer, title string, rows []checkRow) error {
	if err := PrintSection(out, title); err != nil {
		return err
	}
	table := NewTable("CHECK", "STATUS", "VALUE")
	for _, r := range rows {
		status := "ok"
		if !r.ok {
			status = "needs attention"
		}
		table.AddRow(r.label, status, r.value)
	}
	return table.Render(out)
}

func renderKeyValueTable(out io.Writer, title string, pairs [][2]string) error {
	if err := PrintSection(out, title); err != nil {
		return err
	}
	table := NewTable("FIELD", "VALUE")
	for _, p := range pairs {
		table.AddRow(p[0], p[1])
	}
	return table.Render(out)
}

func responseCacheStatusFromAdmin(status map[string]interface{}) (string, bool) {
	services, _ := status["services"].(map[string]interface{})
	cache, _ := services["responseCache"].(map[string]interface{})
	backend, _ := cache["backend"].(string)
	available, _ := cache["available"].(bool)
	return backend, available
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	nextStep := ""

	if err := PrintSection(out, "OmniLLM Doctor"); err != nil {
		return err
	}
	fmt.Fprintln(out)

	// ── Local config ──────────────────────────────────────────────────────────
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "omnillm")
	dbPath := filepath.Join(configDir, "database.sqlite")
	apiKeyFile := filepath.Join(configDir, "api-key")

	configOK := true
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		configOK = false
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to initialise the configuration directory."
		}
	}

	dbValue := dbPath + " (not found — will be created on first start)"
	dbOK := false
	if info, err := os.Stat(dbPath); err == nil {
		dbOK = true
		dbValue = fmt.Sprintf("%s (%d bytes)", dbPath, info.Size())
	}

	apiKeyOK := true
	if _, err := os.Stat(apiKeyFile); os.IsNotExist(err) {
		apiKeyOK = false
	}
	apiKeyValue := apiKeyFile
	if !apiKeyOK {
		apiKeyValue = "not found (will be generated on start)"
	}

	if err := renderChecksTable(out, "Local configuration", []checkRow{
		{ok: configOK, label: "Config directory", value: configDir},
		{ok: dbOK, label: "Database", value: dbValue},
		{ok: apiKeyOK, label: "API key file", value: apiKeyValue},
	}); err != nil {
		return err
	}

	fmt.Fprintln(out)

	// ── Server reachability ───────────────────────────────────────────────────
	c := NewClient(cmd)
	apiKeyConfigured := c.APIKey != ""
	apiKeyStatus := "configured"
	if !apiKeyConfigured {
		apiKeyStatus = "not set (use --api-key or OMNILLM_API_KEY)"
	}

	serverChecks := []checkRow{
		{ok: true, label: "Server address", value: c.BaseURL},
		{ok: apiKeyConfigured, label: "API key", value: apiKeyStatus},
	}

	serverOK := false
	var statusResp map[string]interface{}
	start := time.Now()
	statusData, err := c.Get("/api/admin/status")
	latency := time.Since(start)
	if err != nil {
		serverChecks = append(serverChecks, checkRow{ok: false, label: "Server reachable", value: fmt.Sprintf("NO — %v", err)})
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to start the server."
		}
	} else {
		serverOK = true
		serverChecks = append(serverChecks, checkRow{ok: true, label: "Server reachable", value: fmt.Sprintf("yes (%dms)", latency.Milliseconds())})
		_ = json.Unmarshal(statusData, &statusResp)
	}

	if err := renderChecksTable(out, "Server", serverChecks); err != nil {
		return err
	}

	if serverOK && statusResp != nil {
		fmt.Fprintln(out)
		status, _ := statusResp["status"].(string)
		uptime, _ := statusResp["uptime"].(string)
		modelCount, _ := statusResp["modelCount"].(float64)
		cacheBackend, cacheAvailable := responseCacheStatusFromAdmin(statusResp)

		statusOK := status == "ok" || status == "running" || status == "healthy"
		if err := renderChecksTable(out, "Server status", []checkRow{
			{ok: statusOK, label: "Status", value: status},
			{ok: true, label: "Uptime", value: uptime},
			{ok: true, label: "Models", value: fmt.Sprintf("%.0f", modelCount)},
		}); err != nil {
			return err
		}

		if cacheBackend != "" {
			availability := "available"
			if !cacheAvailable {
				availability = "degraded (model serving remains available)"
			}
			fmt.Fprintln(out)
			if err := renderChecksTable(out, "Response cache", []checkRow{{
				ok:    cacheAvailable,
				label: cacheBackend,
				value: availability,
			}}); err != nil {
				return err
			}
		}

		fmt.Fprintln(out)
		if err := PrintSection(out, "Providers"); err != nil {
			return err
		}

		providerData, provErr := c.Get("/api/admin/providers")
		if provErr == nil {
			providers, _ := parseProviders(providerData)
			activeCount := 0
			for _, p := range providers {
				if v, ok := p["isActive"].(bool); ok && v {
					activeCount++
				}
			}
			providerOK := len(providers) > 0
			if !providerOK && nextStep == "" {
				nextStep = "Run 'omnillm auth' to add and authenticate a provider."
			} else if activeCount == 0 && len(providers) > 0 && nextStep == "" {
				nextStep = "Run 'omnillm provider activate <id>' to activate a provider."
			}

			providerTable := NewTable("CHECK", "STATUS", "VALUE")
			providerStatus := "ok"
			if !providerOK {
				providerStatus = "needs attention"
			}
			providerTable.AddRow("Providers configured", providerStatus, fmt.Sprintf("%d total, %d active", len(providers), activeCount))

			vmData, vmErr := c.Get("/api/admin/virtualmodels")
			if vmErr == nil {
				var vmResp map[string]interface{}
				if jsonErr := json.Unmarshal(vmData, &vmResp); jsonErr == nil {
					items, _ := vmResp["data"].([]interface{})
					providerTable.AddRow("Virtual models", "ok", fmt.Sprintf("%d configured", len(items)))
				}
			}

			if err := providerTable.Render(out); err != nil {
				return err
			}
		}

		authData, authErr := c.Get("/api/admin/auth-status")
		if authErr == nil {
			var authResp map[string]interface{}
			if jsonErr := json.Unmarshal(authData, &authResp); jsonErr == nil {
				authStatus, _ := authResp["status"].(string)
				if authStatus != "" && authStatus != "idle" {
					fmt.Fprintln(out)
					providerID, _ := authResp["providerId"].(string)
					if err := renderChecksTable(out, "Active auth flow", []checkRow{
						{ok: false, label: "Auth in progress", value: fmt.Sprintf("%s (%s)", providerID, authStatus)},
					}); err != nil {
						return err
					}
					kvPairs := [][2]string{}
					if userCode, ok := authResp["userCode"].(string); ok && userCode != "" {
						kvPairs = append(kvPairs, [2]string{"User code", userCode})
					}
					if url, ok := authResp["instructionURL"].(string); ok && url != "" {
						kvPairs = append(kvPairs, [2]string{"Visit", url})
					}
					if len(kvPairs) > 0 {
						if err := renderKeyValueTable(out, "Auth flow details", kvPairs); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	fmt.Fprintln(out)
	if nextStep != "" {
		if _, err := fmt.Fprintf(out, "Next step: %s\n", nextStep); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(out, "Everything looks good. ✓"); err != nil {
			return err
		}
	}

	return nil
}
