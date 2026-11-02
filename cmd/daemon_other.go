//go:build !darwin && !linux

package cmd

import (
	"syscall"

	"github.com/flexdinesh/servef/internal/daemon"
)

func daemonSupported() error { return daemon.ErrUnsupported }

// daemonProcAttr is unused: daemon commands stop at daemonSupported.
func daemonProcAttr() *syscall.SysProcAttr { return nil }
