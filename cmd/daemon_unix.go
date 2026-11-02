//go:build darwin || linux

package cmd

import "syscall"

func daemonSupported() error { return nil }

// daemonProcAttr detaches the daemon into its own session.
func daemonProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
