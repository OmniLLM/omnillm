package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const stateFileName = "server.pid.json"

var ErrNotRunning = errors.New("no managed OmniLLM server is running")

type State struct {
	PID        int    `json:"pid"`
	Executable string `json:"executable"`
	StartID    string `json:"start_id"`
}

type Registration struct {
	path  string
	state State
}

func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "omnillm", stateFileName), nil
}

func Register(path string) (*Registration, error) {
	state, err := processState(os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("identify server process: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lifecycle directory: %w", err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode lifecycle state: %w", err)
	}
	for attempts := 0; attempts < 2; attempts++ {
		openErr := writeExclusiveAtomic(path, data)
		if openErr == nil {
			return &Registration{path: path, state: state}, nil
		}
		if !errors.Is(openErr, os.ErrExist) {
			return nil, fmt.Errorf("create lifecycle state: %w", openErr)
		}
		existing, readErr := Read(path)
		if readErr == nil {
			return nil, fmt.Errorf("OmniLLM server already running with PID %d", existing.PID)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale lifecycle state: %w", err)
		}
	}
	return nil, errors.New("could not claim lifecycle state")
}

func writeExclusiveAtomic(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".server-lifecycle-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Link(temporaryPath, path)
}

func (registration *Registration) Close() error {
	current, err := readState(registration.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if current != registration.state {
		return nil
	}
	if err := os.Remove(registration.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lifecycle state: %w", err)
	}
	return nil
}

func Read(path string) (State, error) {
	state, err := readState(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, ErrNotRunning
		}
		return State{}, fmt.Errorf("read lifecycle state: %w", err)
	}
	actual, err := processState(state.PID)
	if err != nil || actual != state {
		return State{}, ErrNotRunning
	}
	return state, nil
}

func Stop(path string, timeout time.Duration) error {
	state, err := Read(path)
	if err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove invalid lifecycle state: %w", removeErr)
		}
		return err
	}
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return fmt.Errorf("find managed server: %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal managed server: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := Read(path); errors.Is(err, ErrNotRunning) {
			_ = os.Remove(path)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s waiting for PID %d to stop", timeout, state.PID)
}

func readState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode lifecycle state: %w", err)
	}
	if state.PID <= 0 || state.Executable == "" || state.StartID == "" {
		return State{}, errors.New("lifecycle state is incomplete")
	}
	return state, nil
}

func processState(pid int) (State, error) {
	executable, err := processExecutable(pid)
	if err != nil {
		return State{}, err
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return State{}, err
	}
	closingParen := strings.LastIndexByte(string(stat), ')')
	if closingParen < 0 {
		return State{}, errors.New("invalid process stat")
	}
	fields := strings.Fields(string(stat)[closingParen+1:])
	if len(fields) <= 19 {
		return State{}, errors.New("process stat missing start time")
	}
	return State{PID: pid, Executable: executable, StartID: fields[19]}, nil
}

func processExecutable(pid int) (string, error) {
	executable, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return "", err
	}
	executable = strings.TrimSuffix(executable, " (deleted)")
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Clean(executable), nil
	}
	return "", err
}
