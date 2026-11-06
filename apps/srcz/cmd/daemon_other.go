//go:build !darwin && !linux

package cmd

import (
	"syscall"

	"github.com/flexdinesh/srcz/internal/daemon"
)

func daemonSupported() error { return daemon.ErrUnsupported }

// daemonProcAttr is unused: daemon commands stop at daemonSupported.
func daemonProcAttr() *syscall.SysProcAttr { return nil }

// closeInheritedOnExec is unused: daemon commands stop at daemonSupported.
func closeInheritedOnExec() {}
