package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// CacheCmd groups exact-response cache administration commands.
var CacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect the exact-response cache",
}

func init() {
	CacheCmd.AddCommand(cacheStatsCmd)
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show exact-response cache storage and lookup statistics",
	Args:  cobra.NoArgs,
	RunE:  runCacheStats,
}

type cacheStatsResponse struct {
	Enabled       bool       `json:"enabled"`
	TTLSeconds    int64      `json:"ttl_seconds"`
	Entries       int64      `json:"entries"`
	TotalHits     int64      `json:"total_hits"`
	PayloadBytes  int64      `json:"payload_bytes"`
	LookupHits    int64      `json:"lookup_hits"`
	LookupMisses  int64      `json:"lookup_misses"`
	LookupHitRate *float64   `json:"lookup_hit_rate"`
	StatsSince    *time.Time `json:"stats_since"`
	Backend       string     `json:"backend"`
	Available     bool       `json:"available"`
}

func runCacheStats(cmd *cobra.Command, _ []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/settings/response-cache")
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	var resp cacheStatsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}

	availability := responseCacheAvailability(resp.Available)
	hitRate := "unavailable"
	statsSince := "unavailable"
	if resp.Available {
		if resp.LookupHitRate != nil {
			hitRate = fmt.Sprintf("%.2f%%", *resp.LookupHitRate*100)
		}
		if resp.StatsSince != nil {
			statsSince = resp.StatsSince.UTC().Format(time.RFC3339)
		}
	}

	out := cmd.OutOrStdout()
	if err := PrintSection(out, "Exact-response cache statistics"); err != nil {
		return err
	}
	table := NewTable("FIELD", "VALUE")
	state := "disabled"
	if resp.Enabled {
		state = "enabled"
	}
	table.AddRow("State", state)
	table.AddRow("TTL", fmt.Sprintf("%d seconds", resp.TTLSeconds))
	table.AddRow("Backend", fmt.Sprintf("%s (%s)", resp.Backend, availability))
	table.AddRow("Entries", strconv.FormatInt(resp.Entries, 10))
	table.AddRow("Canonical payload", formatBytes(resp.PayloadBytes))
	table.AddRow("Lookup hits", strconv.FormatInt(resp.LookupHits, 10))
	table.AddRow("Lookup misses", strconv.FormatInt(resp.LookupMisses, 10))
	table.AddRow("Lookup hit rate", hitRate)
	table.AddRow("Statistics since", statsSince)
	table.AddRow("Live-entry total hits", strconv.FormatInt(resp.TotalHits, 10))
	return table.Render(out)
}

func formatBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	size := float64(value)
	for _, unit := range units {
		size /= 1024
		if size < 1024 || unit == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s (%d B)", size, unit, value)
		}
	}
	return fmt.Sprintf("%d B", value)
}
