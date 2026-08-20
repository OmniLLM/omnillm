package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"omnillm/internal/server"
)

func TestBackgroundCommandSurface(t *testing.T) {
	if !ServeChildCmd.Hidden {
		t.Fatal("internal child command must remain hidden")
	}
	for _, command := range []*cobra.Command{StartCmd, RestartCmd} {
		foreground, err := command.Flags().GetBool("foreground")
		if err != nil {
			t.Fatalf("%s foreground flag: %v", command.Name(), err)
		}
		if foreground {
			t.Fatalf("%s must default to background mode", command.Name())
		}
	}
}

func TestReadinessFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready.json")
	want := readinessMessage{Ready: true, PID: 42, Address: "http://127.0.0.1:5000"}
	if err := writeReadiness(path, want); err != nil {
		t.Fatalf("write readiness: %v", err)
	}
	got, ok := readReadiness(path)
	if !ok || got != want {
		t.Fatalf("readiness = %#v, %v; want %#v", got, ok, want)
	}
}

func TestStartBackgroundReadinessSuccess(t *testing.T) {
	requireShellBackgroundTest(t)
	script := writeBackgroundHelper(t, `printf '{"ready":true,"pid":%d,"address":"http://127.0.0.1:5000"}' "$$" > "$OMNILLM_INTERNAL_READINESS_PATH"
exec sleep 30`)
	restoreBackgroundGlobals(t, script, time.Second)

	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := startBackground(cmd, server.StartOptions{}); err != nil {
		t.Fatalf("start background: %v", err)
	}
	fields := strings.Fields(output.String())
	if len(fields) < 8 {
		t.Fatalf("unexpected output: %q", output.String())
	}
	pidText := strings.TrimSuffix(fields[7], ",")
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatalf("parse PID from %q: %v", output.String(), err)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatalf("stop helper PID %d: %v", pid, err)
	}
}

func TestStartBackgroundReportsEarlyFailure(t *testing.T) {
	requireShellBackgroundTest(t)
	script := writeBackgroundHelper(t, `printf '{"error":"bind failed"}' > "$OMNILLM_INTERNAL_READINESS_PATH"
exit 1`)
	restoreBackgroundGlobals(t, script, time.Second)

	err := startBackground(&cobra.Command{}, server.StartOptions{})
	if err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("early failure = %v", err)
	}
}

func TestStartBackgroundTimesOutAndKillsChild(t *testing.T) {
	requireShellBackgroundTest(t)
	script := writeBackgroundHelper(t, `exec sleep 30`)
	restoreBackgroundGlobals(t, script, 50*time.Millisecond)

	err := startBackground(&cobra.Command{}, server.StartOptions{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}
}

func requireShellBackgroundTest(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is Unix-specific")
	}
}

func writeBackgroundHelper(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "helper.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func restoreBackgroundGlobals(t *testing.T, executable string, timeout time.Duration) {
	t.Helper()
	oldExecutable := resolveExecutable
	oldTimeout := backgroundStartupTimeout
	resolveExecutable = func() (string, error) { return executable, nil }
	backgroundStartupTimeout = timeout
	t.Cleanup(func() {
		resolveExecutable = oldExecutable
		backgroundStartupTimeout = oldTimeout
	})
}
