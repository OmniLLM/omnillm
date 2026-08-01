package commands

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var LogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream or view server logs",
	Long: `Stream live server logs.

Running "omnillm logs" with no subcommand is equivalent to "omnillm logs tail".`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogsTail(cmd)
	},
}

func init() {
	addLogsTailFlags(LogsCmd)
	addLogsTailFlags(logsTailCmd)
	LogsCmd.AddCommand(logsTailCmd)
}

// addLogsTailFlags registers the tail flag set on cmd so that both
// "omnillm logs" and "omnillm logs tail" accept the same options.
func addLogsTailFlags(cmd *cobra.Command) {
	cmd.Flags().String("level", "", "Only show messages at this level or above (fatal|error|warn|info|debug|trace)")
	cmd.Flags().String("source", "", "Only show messages from this source (e.g. backend, frontend)")
	cmd.Flags().String("grep", "", "Only show lines matching this regular expression")
	cmd.Flags().Bool("json", false, "Emit one JSON object per log line instead of human-readable text")
	cmd.Flags().Bool("no-fields", false, "Hide structured fields (request=, latency=, ...) and show only the message")
	_ = cmd.RegisterFlagCompletionFunc("level", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"fatal", "error", "warn", "info", "debug", "trace"}, cobra.ShellCompDirectiveNoFileComp
	})
}

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream live server logs (SSE)",
	Args:  cobra.NoArgs,
	Example: `  # Stream all logs
  omnillm logs tail

  # Stream only errors and above
  omnillm logs tail --level error

  # Only backend warnings, without structured fields
  omnillm logs tail --level warn --source backend --no-fields

  # Follow a single request and pipe into jq
  omnillm logs tail --grep 'request=abc123' --json | jq .`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogsTail(cmd)
	},
}

func runLogsTail(cmd *cobra.Command) error {
	levelFilter, _ := cmd.Flags().GetString("level")
	sourceFilter, _ := cmd.Flags().GetString("source")
	grepPattern, _ := cmd.Flags().GetString("grep")
	asJSON, _ := cmd.Flags().GetBool("json")
	hideFields, _ := cmd.Flags().GetBool("no-fields")

	if levelFilter != "" {
		if _, ok := levelOrder[strings.ToLower(levelFilter)]; !ok {
			return fmt.Errorf("invalid --level %q: want one of fatal, error, warn, info, debug, trace", levelFilter)
		}
	}

	var grep *regexp.Regexp
	if grepPattern != "" {
		var err error
		grep, err = regexp.Compile(grepPattern)
		if err != nil {
			return fmt.Errorf("invalid --grep pattern: %w", err)
		}
	}

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

	out := cmd.OutOrStdout()
	if !asJSON {
		fmt.Fprintf(cmd.ErrOrStderr(), "Connected to log stream (Ctrl+C to stop)\n\n")
	}

	useColor := !asJSON && IsTerminalWriter(out) && os.Getenv("NO_COLOR") == ""
	var colorAllocator *correlationColorAllocator
	if useColor && !hideFields {
		colorAllocator, err = newCorrelationColorAllocator()
		if err != nil {
			return fmt.Errorf("initialize log colors: %w", err)
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	// Log lines carrying request/response payloads can exceed the 64KiB default
	// scanner buffer; without this the stream aborts with ErrTooLong.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE framing: ":" lines are comments (heartbeats); only "data: " carries a payload.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		entry := parseLogPayload(strings.TrimPrefix(line, "data: "))

		if levelFilter != "" && !isLevelAtOrAbove(entry.Level, levelFilter) {
			continue
		}
		if sourceFilter != "" && !strings.EqualFold(entry.Source, sourceFilter) {
			continue
		}
		if grep != nil && !grep.MatchString(entry.Raw) {
			continue
		}

		if asJSON {
			encoded, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintln(out, string(encoded))
			continue
		}

		fmt.Fprintln(out, entry.Render(useColor, hideFields, colorAllocator))
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("log stream error: %w", err)
	}
	return nil
}

// LogEntry is a single parsed log line from the admin log stream.
type LogEntry struct {
	Time    string   `json:"time"`
	Level   string   `json:"level"`
	Source  string   `json:"source,omitempty"`
	Message string   `json:"message"`
	Fields  []string `json:"fields,omitempty"`
	Raw     string   `json:"-"`
}

// parseLogPayload parses one SSE payload. The server emits pipe-delimited lines
// of the form "[timestamp] | source | LEVEL | message | key=value | ...", but it
// also forwards raw JSON and plain text, so all three shapes are handled.
func parseLogPayload(payload string) LogEntry {
	payload = strings.TrimSpace(payload)
	entry := LogEntry{Raw: payload, Level: "info", Message: payload}

	if strings.HasPrefix(payload, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(payload), &obj); err == nil {
			if v, ok := obj["time"].(string); ok {
				entry.Time = v
			}
			if v, ok := obj["level"].(string); ok && v != "" {
				entry.Level = v
			}
			if v, ok := obj["source"].(string); ok {
				entry.Source = v
			}
			if v, ok := obj["message"].(string); ok && v != "" {
				entry.Message = v
			}
			return entry
		}
	}

	segments := strings.Split(payload, " | ")
	for i := range segments {
		segments[i] = strings.TrimSpace(segments[i])
	}
	// Minimum well-formed shape: [timestamp] | source | LEVEL | message
	if len(segments) < 4 || !strings.HasPrefix(segments[0], "[") || !strings.HasSuffix(segments[0], "]") {
		return entry
	}

	entry.Time = strings.TrimSuffix(strings.TrimPrefix(segments[0], "["), "]")
	entry.Source = segments[1]
	entry.Level = strings.ToLower(segments[2])
	entry.Message = segments[3]
	for _, field := range segments[4:] {
		if field != "" {
			entry.Fields = append(entry.Fields, field)
		}
	}
	return entry
}

// Render formats the entry for human consumption.
func (e LogEntry) Render(useColor, hideFields bool, colorAllocator *correlationColorAllocator) string {
	ts := e.Time
	if ts == "" {
		ts = time.Now().Format("15:04:05")
	} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
		ts = t.Format("15:04:05")
	}

	levelStr := padRight(strings.ToUpper(e.Level), 5)

	var b strings.Builder
	if useColor {
		lc := levelColor(e.Level)
		fmt.Fprintf(&b, "%s%s%s  %s%s%s", colorDim, ts, colorReset, lc, levelStr, colorReset)
		if e.Source != "" {
			fmt.Fprintf(&b, "  %s%s%s", colorDim, e.Source, colorReset)
		}
		fmt.Fprintf(&b, "  %s%s%s", lc, e.Message, colorReset)
	} else {
		fmt.Fprintf(&b, "%s  %s", ts, levelStr)
		if e.Source != "" {
			fmt.Fprintf(&b, "  %s", e.Source)
		}
		fmt.Fprintf(&b, "  %s", e.Message)
	}

	if !hideFields && len(e.Fields) > 0 {
		b.WriteString("  ")
		writeLogFields(&b, e.Fields, useColor, colorAllocator)
	}
	return b.String()
}

// writeLogFields highlights correlation fields while leaving other metadata dim.
func writeLogFields(b *strings.Builder, fields []string, useColor bool, allocator *correlationColorAllocator) {
	for i, field := range fields {
		if i > 0 {
			b.WriteByte(' ')
		}
		if !useColor {
			b.WriteString(field)
			continue
		}

		color := colorDim
		if value, ok := correlationFieldValue(field); ok && allocator != nil {
			color = allocator.color(value)
		}
		b.WriteString(color)
		b.WriteString(field)
		b.WriteString(colorReset)
	}
}

func correlationFieldValue(field string) (string, bool) {
	key, value, ok := strings.Cut(field, "=")
	if !ok || value == "" {
		return "", false
	}

	switch strings.ToLower(strings.TrimSpace(key)) {
	case "request", "request_id", "session", "session_id":
		return value, true
	default:
		return "", false
	}
}

const correlationColorFloor = 80

type correlationColorAllocator struct {
	colors []uint32
	byID   map[string]string
	next   int
}

func newCorrelationColorAllocator() (*correlationColorAllocator, error) {
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return nil, err
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes[:]))
	return newCorrelationColorAllocatorWithColors(correlationColorPalette(), mathrand.New(mathrand.NewSource(seed))), nil
}

func newCorrelationColorAllocatorWithColors(colors []uint32, rng *mathrand.Rand) *correlationColorAllocator {
	shuffled := append([]uint32(nil), colors...)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return &correlationColorAllocator{
		colors: shuffled,
		byID:   make(map[string]string),
	}
}

func correlationColorPalette() []uint32 {
	const channelCount = 256 - correlationColorFloor
	colors := make([]uint32, 0, channelCount*channelCount*channelCount-(channelCount-1)*(channelCount-1)*(channelCount-1))
	for red := correlationColorFloor; red <= 255; red++ {
		for green := correlationColorFloor; green <= 255; green++ {
			for blue := correlationColorFloor; blue <= 255; blue++ {
				if red != 255 && green != 255 && blue != 255 {
					continue
				}
				colors = append(colors, uint32(red<<16|green<<8|blue))
			}
		}
	}
	return colors
}

func (a *correlationColorAllocator) color(value string) string {
	if color, ok := a.byID[value]; ok {
		return color
	}
	if a.next >= len(a.colors) {
		a.byID[value] = colorDim
		return colorDim
	}

	rgb := a.colors[a.next]
	a.next++
	color := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", rgb>>16, rgb>>8&0xff, rgb&0xff)
	a.byID[value] = color
	return color
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
	"fatal":   0,
	"panic":   0,
	"error":   1,
	"warn":    2,
	"warning": 2,
	"info":    3,
	"debug":   4,
	"trace":   5,
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
