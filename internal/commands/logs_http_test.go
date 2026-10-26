package commands

import (
	"strings"
	"testing"
)

func TestLogTailPreservesHTTPOutcome(t *testing.T) {
	for _, status := range []string{"200", "400", "413"} {
		entry := parseLogPayload("[2026-10-08T12:07:21+08:00] | backend | INFO | HTTP | request=upload-test | method=POST | path=/v1/responses | status=" + status + " | latency=1263ms")
		for _, color := range []bool{false, true} {
			rendered := entry.Render(color, false, nil)
			for _, field := range entry.Fields {
				if !strings.Contains(rendered, field) {
					t.Errorf("missing %s in %s", field, rendered)
				}
			}
		}
		if strings.Contains(entry.Render(false, true, nil), "status=") {
			t.Fatal("no-fields ignored")
		}
	}
}
