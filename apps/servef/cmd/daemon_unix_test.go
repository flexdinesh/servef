//go:build darwin || linux

package cmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCloseInheritedOnExecMarksInheritableDescriptors(t *testing.T) {
	fd, err := syscall.Open(filepath.Join(t.TempDir(), "inherited"), syscall.O_CREAT|syscall.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if closeOnExec(t, fd) {
		t.Fatal("descriptor starts close-on-exec; the test needs an inheritable one")
	}
	closeInheritedOnExec()
	if !closeOnExec(t, fd) {
		t.Fatal("inherited descriptor stays inheritable")
	}
	if closeOnExec(t, int(os.Stderr.Fd())) {
		t.Fatal("stderr became close-on-exec")
	}
}

func closeOnExec(t *testing.T, fd int) bool {
	t.Helper()
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	if errno != 0 {
		t.Fatal(errno)
	}
	return flags&syscall.FD_CLOEXEC != 0
}
