//go:build darwin || linux

package daemon

import (
	"os"
	goruntime "runtime"
)

// DefaultSocket is the per-user socket path (decision 0061).
func DefaultSocket() (string, error) {
	return socketPath(os.Getenv, os.TempDir(), os.Getuid(), goruntime.GOOS == "darwin")
}
