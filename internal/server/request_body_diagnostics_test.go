package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"omnillm/internal/registry"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type uploadLogCapture struct {
	mu    sync.Mutex
	lines []string
}

func (b *uploadLogCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, string(p))
	return len(p), nil
}
func (b *uploadLogCapture) snapshot() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, "")
}

func TestTruncatedUploadHTTPDiagnostics(t *testing.T) {
	registry.GetProviderRegistry().WaitForPendingSaves()
	old := log.Logger
	var logs uploadLogCapture
	log.Logger = zerolog.New(&logs)
	defer func() { log.Logger = old }()
	srv := newTestServer(t)
	defer srv.Close()
	for _, framing := range []struct{ name, headers, body string }{
		{"length", "Content-Length: 100\r\n", "BODY_SECRET"},
		{"chunked", "Transfer-Encoding: chunked\r\n", "20\r\nBODY_SECRET"},
	} {
		t.Run(framing.name, func(t *testing.T) {
			conn, err := net.DialTimeout("tcp", strings.TrimPrefix(srv.URL, "http://"), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			_, err = fmt.Fprintf(conn, "POST /v1/responses?token=QUERY_SECRET HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer test-api-key\r\n%sConnection: close\r\n\r\n%s", framing.headers, framing.body)
			if err != nil {
				t.Fatal(err)
			}
			if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 400 || !strings.Contains(string(body), "invalid_request_error") {
				t.Fatalf("response %d %s", resp.StatusCode, body)
			}
		})
	}
	// Reading each Connection: close response to EOF completes its handler/logging.
	raw := logs.snapshot()
	var failures, accesses int
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event["reason"] == "unexpected_eof" {
			failures++
			if event["level"] != "warn" || event["bytes_read"] != float64(11) {
				t.Errorf("bad failure evidence: %s", line)
			}
		}
		if event["message"] == "HTTP" {
			accesses++
			formatted := formatBroadcastLogLine("backend", line)
			for _, part := range []string{"method=POST", "path=/v1/responses", "status=400", "request=", "latency="} {
				if !strings.Contains(formatted, part) {
					t.Errorf("missing %s in %s", part, formatted)
				}
			}
		}
	}
	if failures != 2 || accesses != 2 {
		t.Errorf("failures=%d accesses=%d logs=%s", failures, accesses, raw)
	}
	for _, secret := range []string{"BODY_SECRET", "QUERY_SECRET", "test-api-key"} {
		if strings.Contains(raw, secret) {
			t.Errorf("leaked %s", secret)
		}
	}
}
