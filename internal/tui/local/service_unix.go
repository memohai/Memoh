//go:build !windows

package local

import (
	"os/exec"
	"syscall"
)

func detachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
