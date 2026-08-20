//go:build !windows

package commands

import (
	"os/exec"
	"syscall"
)

func detachProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
