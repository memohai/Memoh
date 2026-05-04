//go:build windows

package local

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func detachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

// terminate uses taskkill to fan a graceful close request through the
// process tree; CTRL_BREAK_EVENT does not work for processes that
// detached from the console.
func terminate(pid int) error {
	cmd := exec.Command("taskkill", "/PID", itoa(pid), "/T")
	return cmd.Run()
}

func kill(pid int) error {
	cmd := exec.Command("taskkill", "/PID", itoa(pid), "/T", "/F")
	return cmd.Run()
}

func itoa(pid int) string {
	const digits = "0123456789"
	if pid == 0 {
		return "0"
	}
	negative := pid < 0
	if negative {
		pid = -pid
	}
	var buf [20]byte
	i := len(buf)
	for pid > 0 {
		i--
		buf[i] = digits[pid%10]
		pid /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
