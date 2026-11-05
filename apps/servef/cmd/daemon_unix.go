//go:build darwin || linux

package cmd

import (
	"os"
	"runtime"
	"strconv"
	"syscall"
)

func daemonSupported() error { return nil }

// daemonProcAttr detaches the daemon into its own session.
func daemonProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// closeInheritedOnExec marks descriptors above stderr close-on-exec, so a
// started daemon holds only its stdio. Parents can leave many
// descriptors inheritable, and os/exec passes those on.
func closeInheritedOnExec() {
	dir := "/proc/self/fd"
	if runtime.GOOS == "darwin" {
		dir = "/dev/fd"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if fd, err := strconv.Atoi(entry.Name()); err == nil && fd > 2 {
			syscall.CloseOnExec(fd)
		}
	}
}
