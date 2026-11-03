//go:build unix

package desktopapp

import "syscall"

// detached starts a process in its own session, away from the terminal.
func detached() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
