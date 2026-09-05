//go:build darwin

package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinProcInfoCallPIDInfo = 2
	darwinProcPIDPathInfo     = 11
	darwinProcPIDPathMaxSize  = 4 * 1024
)

func processState(pid int) (State, error) {
	executable, err := processExecutable(pid)
	if err != nil {
		return State{}, err
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return State{}, fmt.Errorf("read process metadata: %w", err)
	}
	if info.Proc.P_pid != int32(pid) {
		return State{}, fmt.Errorf("process metadata PID = %d, want %d", info.Proc.P_pid, pid)
	}
	start := info.Proc.P_starttime
	if start.Sec == 0 && start.Usec == 0 {
		return State{}, errors.New("process metadata missing start time")
	}
	startID := fmt.Sprintf("%d:%d", start.Sec, start.Usec)
	return State{PID: pid, Executable: executable, StartID: startID}, nil
}

func processExecutable(pid int) (string, error) {
	buffer := make([]byte, darwinProcPIDPathMaxSize)
	_, _, errno := unix.Syscall6(
		unix.SYS_PROC_INFO,
		darwinProcInfoCallPIDInfo,
		uintptr(pid),
		darwinProcPIDPathInfo,
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if errno != 0 {
		return "", fmt.Errorf("read process executable path: %w", errno)
	}
	executable, err := parseDarwinExecutable(buffer)
	if err != nil {
		return "", err
	}
	return normalizeExecutablePath(executable)
}

func parseDarwinExecutable(data []byte) (string, error) {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	if len(data) == 0 {
		return "", errors.New("process executable path is empty")
	}
	return string(data), nil
}
