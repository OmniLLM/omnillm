package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"omnillm/internal/server"
)

const readinessPathEnvironment = "OMNILLM_INTERNAL_READINESS_PATH"

var (
	backgroundStartupTimeout = 30 * time.Second
	resolveExecutable        = os.Executable
)

type readinessMessage struct {
	Ready   bool   `json:"ready,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Address string `json:"address,omitempty"`
	Error   string `json:"error,omitempty"`
}

var ServeChildCmd = &cobra.Command{
	Use:    "_serve",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		readinessPath := os.Getenv(readinessPathEnvironment)
		if readinessPath == "" {
			return errors.New("missing internal readiness path")
		}
		var options server.StartOptions
		if err := json.NewDecoder(cmd.InOrStdin()).Decode(&options); err != nil {
			return fmt.Errorf("decode child startup configuration: %w", err)
		}
		reported := false
		options.Ready = func() error {
			reported = true
			message := readinessMessage{
				Ready:   true,
				PID:     os.Getpid(),
				Address: fmt.Sprintf("http://%s:%d", options.Host, options.Port),
			}
			return writeReadiness(readinessPath, message)
		}
		err := server.RunServer(options)
		if err != nil && !reported {
			_ = writeReadiness(readinessPath, readinessMessage{Error: err.Error()})
		}
		return err
	},
}

func startBackground(cmd *cobra.Command, options server.StartOptions) error {
	executable, err := resolveExecutable()
	if err != nil {
		return fmt.Errorf("resolve OmniLLM executable: %w", err)
	}
	configuration, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("encode child startup configuration: %w", err)
	}

	readinessFile, err := os.CreateTemp("", "omnillm-readiness-*.json")
	if err != nil {
		return fmt.Errorf("create readiness state: %w", err)
	}
	readinessPath := readinessFile.Name()
	if err := readinessFile.Close(); err != nil {
		_ = os.Remove(readinessPath)
		return fmt.Errorf("close readiness state: %w", err)
	}
	if err := os.Remove(readinessPath); err != nil {
		return fmt.Errorf("prepare readiness state: %w", err)
	}
	defer os.Remove(readinessPath)

	child := exec.Command(executable, "_serve")
	child.Stdin = bytes.NewReader(configuration)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open null device for background server: %w", err)
	}
	child.Stdout = devNull
	child.Stderr = devNull
	child.Env = append(os.Environ(), readinessPathEnvironment+"="+readinessPath)
	detachProcess(child)
	if err := child.Start(); err != nil {
		_ = devNull.Close()
		return fmt.Errorf("start background server: %w", err)
	}
	_ = devNull.Close()

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- child.Wait()
	}()

	cleanup := func() {
		_ = child.Process.Kill()
		<-waitResult
	}
	deadline := time.NewTimer(backgroundStartupTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(25 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case waitErr := <-waitResult:
			if message, ok := readReadiness(readinessPath); ok && message.Error != "" {
				return errors.New(message.Error)
			}
			return fmt.Errorf("background server exited before readiness: %w", waitErr)
		case <-poll.C:
			message, ok := readReadiness(readinessPath)
			if !ok {
				continue
			}
			if !message.Ready {
				cleanup()
				if message.Error == "" {
					message.Error = "child exited before reporting readiness"
				}
				return errors.New(message.Error)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OmniLLM server started in the background (PID %d, %s).\n", message.PID, message.Address)
			return nil
		case <-deadline.C:
			cleanup()
			return fmt.Errorf("timed out after %s waiting for background server readiness", backgroundStartupTimeout)
		}
	}
}

func writeReadiness(path string, message readinessMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omnillm-readiness-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readReadiness(path string) (readinessMessage, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return readinessMessage{}, false
	}
	var message readinessMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return readinessMessage{}, false
	}
	return message, true
}
