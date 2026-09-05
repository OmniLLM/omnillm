//go:build linux

package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	return normalizeExecutablePath(strings.TrimSuffix(executable, " (deleted)"))
}
