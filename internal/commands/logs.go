package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var LogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream or view server logs",
}

func init() {
	logsTailCmd.Flags().String("level", "", "Filter: only show messages at this level or above (error|warn|info|debug|trace)")
	_ = logsTailCmd.RegisterFlagCompletionFunc("level", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"fatal", "error", "warn", "info", "debug", "trace"}, cobra.ShellCompDirectiveNoFileComp
	})
	LogsCmd.AddCommand(logsTailCmd)
}

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream live server logs (SSE)",
	Example: `  # Stream all logs
  omnillm logs tail

  # Stream only errors and above
  omnillm logs tail --level error

  # Stream warnings and above
  omnillm logs tail --level warn`,
	RunE: func(cmd *cobra.Command, args []string) error {
		levelFilter, _ := cmd.Flags().GetString("level")

		c := NewClient(cmd)
		resp, err := c.GetStream("/api/admin/logs/stream")
		if err != nil {
			return fmt.Errorf("connect to log stream: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connected to log stream (Ctrl+C to stop)\n\n")

		out := cmd.OutOrStdout()
		useColor := IsTerminalWriter(out) && os.Getenv("NO_COLOR") == ""
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE format: lines starting with "data: " contain the payload
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := line[6:] // strip "data: "

			// Try to parse as JSON for filtering and pretty display
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &entry); err != nil {
				// Not JSON — print raw
				fmt.Fprintln(out, payload)
				continue
			}

			// Level filtering
			if levelFilter != "" {
				level, _ := entry["level"].(string)
				if !isLevelAtOrAbove(level, levelFilter) {
					continue
				}
			}

			// Formatted output
			ts, _ := entry["time"].(string)
			level, _ := entry["level"].(string)
			message, _ := entry["message"].(string)

			if ts == "" {
				ts = time.Now().Format("15:04:05")
			} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
				ts = t.Format("15:04:05")
			}

			levelStr := padRight(strings.ToUpper(level), 5)
			if useColor {
				c := levelColor(level)
				fmt.Fprintf(out, "%s%s%s  %s%s%s  %s%s%s\n",
					colorDim, ts, colorReset,
					c, levelStr, colorReset,
					c, message, colorReset)
			} else {
				fmt.Fprintf(out, "%s  %s  %s\n", ts, levelStr, message)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("log stream error: %w", err)
		}
		return nil
	},
}

// ANSI colors for log level rendering.
const (
	colorReset   = "\x1b[0m"
	colorDim     = "\x1b[90m"
	colorRed     = "\x1b[31m"
	colorBoldRed = "\x1b[1;31m"
	colorYellow  = "\x1b[33m"
	colorGreen   = "\x1b[32m"
	colorCyan    = "\x1b[36m"
	colorMagenta = "\x1b[35m"
)

// levelColor returns the ANSI color for a log level.
func levelColor(level string) string {
	switch strings.ToLower(level) {
	case "fatal", "panic":
		return colorBoldRed
	case "error":
		return colorRed
	case "warn", "warning":
		return colorYellow
	case "info":
		return colorGreen
	case "debug":
		return colorCyan
	case "trace":
		return colorMagenta
	default:
		return colorReset
	}
}

// levelOrder maps log level names to severity (lower = more severe).
var levelOrder = map[string]int{
	"fatal": 0,
	"error": 1,
	"warn":  2,
	"info":  3,
	"debug": 4,
	"trace": 5,
}

// isLevelAtOrAbove returns true if msgLevel is at least as severe as filterLevel.
func isLevelAtOrAbove(msgLevel, filterLevel string) bool {
	m, mOK := levelOrder[strings.ToLower(msgLevel)]
	f, fOK := levelOrder[strings.ToLower(filterLevel)]
	if !mOK || !fOK {
		return true // unknown levels pass through
	}
	return m <= f
}

// ─── placeholder so bytes/io are not unused if stream is empty ───────────────
var _ = bytes.NewBuffer
var _ = io.Discard