package lifecycle

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRegisterReadAndOwnershipSafeClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	registration, err := Register(path)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	state, err := Read(path)
	if err != nil || state.PID != os.Getpid() {
		t.Fatalf("read registered state = %#v, %v", state, err)
	}

	replacement := []byte(`{"pid":1,"executable":"replacement","start_id":"new"}`)
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatalf("replace state: %v", err)
	}
	if err := registration.Close(); err != nil {
		t.Fatalf("close registration: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("replacement state was removed: %v", err)
	}
}

func TestRegisterReplacesStaleState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{"pid":99999999,"executable":"/missing","start_id":"1"}`), 0o600); err != nil {
		t.Fatalf("write stale state: %v", err)
	}
	registration, err := Register(path)
	if err != nil {
		t.Fatalf("register over stale state: %v", err)
	}
	t.Cleanup(func() { _ = registration.Close() })
}

func TestReadRejectsMalformedAndMismatchedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || errors.Is(err, ErrNotRunning) {
		t.Fatalf("malformed state error = %v", err)
	}

	state, err := processState(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	state.StartID += "-wrong"
	data := []byte(`{"pid":` + strconv.Itoa(state.PID) + `,"executable":` + strconv.Quote(state.Executable) + `,"start_id":` + strconv.Quote(state.StartID) + `}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("mismatched state error = %v", err)
	}
}

func TestStopTerminatesManagedProcess(t *testing.T) {
	command := exec.Command("sleep", "30")
	if err := command.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })

	state, err := processState(command.Process.Pid)
	if err != nil {
		t.Fatalf("identify child: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	waited := make(chan struct{})
	go func() {
		_ = command.Wait()
		_ = os.Remove(path)
		close(waited)
	}()

	if err := Stop(path, 2*time.Second); err != nil {
		t.Fatalf("stop child: %v", err)
	}
	<-waited
}

func TestStopRemovesStaleState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{"pid":99999999,"executable":"/missing","start_id":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Stop(path, time.Second); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("stop stale state error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale state remains: %v", err)
	}
}

func TestStopTimesOutWhenProcessDoesNotTerminate(t *testing.T) {
	command := exec.Command("sh", "-c", "trap '' TERM; echo ready; while :; do sleep 1; done")
	ready, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	buffer := make([]byte, 6)
	if _, err := io.ReadFull(ready, buffer); err != nil {
		t.Fatalf("wait for child readiness: %v", err)
	}

	state, err := processState(command.Process.Pid)
	if err != nil {
		t.Fatalf("identify child: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	err = Stop(path, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("stop timeout error = %v", err)
	}
}
