package daemon

import (
	"errors"
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strconv"
)

// ErrUnsupported is returned on hosts other than macOS and Linux.
var ErrUnsupported = errors.New("servef daemon supports macOS and Linux only")

// socketName is the socket's file name within its private directory.
const socketName = "daemon.sock"

// socketPath is the per-user socket path (decision 0061). Linux uses
// $XDG_RUNTIME_DIR/servef when it is set and absolute; otherwise Linux uses
// <tempDir>/servef-<uid> and macOS <tempDir>/servef ($TMPDIR is per-user
// there). A path over the platform's sun_path limit falls back to
// /tmp/servef-<uid>.
func socketPath(getenv func(string) string, tempDir string, uid int, darwin bool) (string, error) {
	user := "servef-" + strconv.Itoa(uid)
	var preferred string
	switch runtimeDir := getenv("XDG_RUNTIME_DIR"); {
	case darwin:
		preferred = filepath.Join(tempDir, "servef", socketName)
	case runtimeDir != "" && filepath.IsAbs(runtimeDir):
		preferred = filepath.Join(runtimeDir, "servef", socketName)
	default:
		preferred = filepath.Join(tempDir, user, socketName)
	}
	return firstFitting(maxSocketLength(darwin), preferred, filepath.Join("/tmp", user, socketName))
}

// firstFitting returns the first path of at most limit bytes.
func firstFitting(limit int, paths ...string) (string, error) {
	for _, path := range paths {
		if len(path) <= limit {
			return path, nil
		}
	}
	return "", fmt.Errorf("no servef daemon socket path fits in %d bytes; pass --socket", limit)
}

// maxSocketLength is the longest socket path sun_path holds with its
// terminating NUL.
func maxSocketLength(darwin bool) int {
	if darwin {
		return 103
	}
	return 107
}

// checkSocketLength rejects paths longer than the platform sun_path limit.
func checkSocketLength(path string) error {
	if limit := maxSocketLength(goruntime.GOOS == "darwin"); len(path) > limit {
		return fmt.Errorf("socket path %s is %d bytes; the limit is %d", path, len(path), limit)
	}
	return nil
}
