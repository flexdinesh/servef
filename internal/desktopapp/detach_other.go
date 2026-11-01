//go:build !unix

package desktopapp

import "syscall"

func detached() *syscall.SysProcAttr { return nil }
