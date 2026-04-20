package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"omnillm/internal/routes"
)

type sseLogWriter struct {
	source string
}

func (w sseLogWriter) Write(p []byte) (int, error) {
	raw := strings.TrimSpace(string(p))
	if raw == "" {
		return len(p), nil
	}

	line := formatBroadcastLogLine(w.source, raw)
	if line != "" {
		routes.BroadcastLogLine(line)
	}

	return len(p), nil
}

var preferredLogFieldOrder = []string{
	"request_id",
	"api_shape",
	"model_requested",
	"model_used",
	"model",
	"provider",
	"tool_name",
	"tool_call_id",
	"tool_arguments",
	"tool_result",
	"tool_is_error",
	"loop_message_index",
	"loop_item_index",
	"loop_block_index",
	"messages",
	"tools",
	"stream",
	"stop_reason",
	"input_tokens",
	"output_tokens",
	"latency_ms",
	"url",
	"admin",
	"level",
	"count",
	"verbose",
}

func formatBroadcastLogLine(source string, raw string) string {
	if source == "" {
		source = "backend"
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return fmt.Sprintf("[%s] | %s | INFO | %s", time.Now().Format(time.RFC3339), source, raw)
	}

	timestamp := stringValue(event["time"])
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339)
	}

	level := strings.ToUpper(stringValue(event["level"]))
	if level == "" {
		level = "INFO"
	}

	message := stringValue(event["message"])
	if message == "" {
		message = raw
	}

	segments := []string{
		fmt.Sprintf("[%s]", timestamp),
		source,
		level,
		message,
	}

	segments = append(segments, collectFormattedFields(event)...)
	return strings.Join(segments, " | ")
}

func collectFormattedFields(event map[string]interface{}) []string {
	fields := make([]string, 0, len(event))
	seen := make(map[string]struct{}, len(event))

	// Pre-read values needed for suppression logic.
	requestedModel := stringValue(event["model_requested"])

	for _, key := range preferredLogFieldOrder {
		formatted, ok := formatStructuredField(key, event[key], requestedModel)
		if ok {
			fields = append(fields, formatted)
			seen[key] = struct{}{}
		}
	}

	remaining := make([]string, 0, len(event))
	for key, value := range event {
		if _, ok := seen[key]; ok {
			continue
		}

		formatted, ok := formatStructuredField(key, value, requestedModel)
		if ok {
			remaining = append(remaining, formatted)
		}
	}

	sort.Strings(remaining)
	return append(fields, remaining...)
}

func formatStructuredField(key string, value interface{}, requestedModel string) (string, bool) {
	switch key {
	case "", "level", "message", "time", "method", "path", "status":
		return "", false
	}

	formattedValue := stringValue(value)
	if formattedValue == "" {
		return "", false
	}

	switch key {
	case "request_id":
		return "request=" + formattedValue, true
	case "api_shape":
		return "api=" + formattedValue, true
	case "model_requested":
		return "requested=" + formattedValue, true
	case "model_used":
		// Omit when model_used equals model_requested (no routing occurred).
		if formattedValue == requestedModel {
			return "", false
		}
		return "used=" + formattedValue, true
	case "latency_ms":
		if !strings.HasSuffix(formattedValue, "ms") {
			formattedValue += "ms"
		}
		return "latency=" + formattedValue, true
	case "input_tokens":
		// Suppress zero token counts — they add no information.
		if numericValue(value) == 0 {
			return "", false
		}
		return "input=" + formattedValue, true
	case "output_tokens":
		if numericValue(value) == 0 {
			return "", false
		}
		return "output=" + formattedValue, true
	default:
		return key + "=" + formattedValue, true
	}
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	}
}

func numericValue(value interface{}) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case float64:
		return int64(typed)
	case json.Number:
		v, _ := typed.Int64()
		return v
	default:
		return 0
	}
}
