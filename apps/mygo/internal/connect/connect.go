// Package connect ensures the local daemon with the bundled servef binary,
// as the Electron app does (decision 0067).
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Protocol is the daemon protocol the app speaks (decision 0061).
const Protocol = 1

// EnsureTimeout bounds `servef daemon ensure`.
const EnsureTimeout = 15 * time.Second

// Run runs a program and returns its standard output and error.
type Run func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// Exec runs programs with os/exec, argv only, stdin from /dev/null.
func Exec(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Locate returns the servef binary to run: the bundled resources/bin/servef
// in a packaged app; otherwise SERVEF_MYGO_BINARY, else servef on PATH.
func Locate(packaged bool, resources string, getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	if packaged {
		return filepath.Join(resources, "bin", "servef"), nil
	}
	if override := getenv("SERVEF_MYGO_BINARY"); override != "" {
		return override, nil
	}
	if found, err := lookPath("servef"); err == nil && filepath.IsAbs(found) {
		return found, nil
	}
	return "", errors.New("servef is not on PATH and SERVEF_MYGO_BINARY is not set")
}

// DevFlags maps SERVEF_MYGO_SOCKET and SERVEF_MYGO_ROOTS_FILE to the
// daemon's --socket and --roots-file, for an isolated development daemon.
func DevFlags(getenv func(string) string) []string {
	var flags []string
	if socket := getenv("SERVEF_MYGO_SOCKET"); socket != "" {
		flags = append(flags, "--socket", socket)
	}
	if roots := getenv("SERVEF_MYGO_ROOTS_FILE"); roots != "" {
		flags = append(flags, "--roots-file", roots)
	}
	return flags
}

// EnsureError is a daemon that could not be ensured.
type EnsureError struct {
	// Mismatch: the running daemon speaks another protocol; Status is set.
	Mismatch bool
	Status   *contract.DaemonStatus
	Detail   string
}

func (e *EnsureError) Error() string { return e.Detail }

// Ensure runs `servef daemon ensure --json` and returns the daemon's
// status, whose Socket the client dials. ensure prints the status even when
// it exits non-zero on a protocol mismatch.
func Ensure(ctx context.Context, run Run, binary string, flags []string) (contract.DaemonStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, EnsureTimeout)
	defer cancel()
	stdout, stderr, err := run(ctx, binary, append([]string{"daemon", "ensure", "--json"}, flags...)...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return contract.DaemonStatus{}, &EnsureError{Detail: fmt.Sprintf("servef daemon ensure timed out after %s", EnsureTimeout)}
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return contract.DaemonStatus{}, &EnsureError{Detail: fmt.Sprintf("cannot run %s: %v", binary, err)}
	}
	status, parsed := lastStatus(stdout)
	if parsed && status.Protocol != Protocol {
		return status, &EnsureError{Mismatch: true, Status: &status, Detail: fmt.Sprintf(
			"The running daemon speaks protocol %d; servef-mygo needs protocol %d. Run servef daemon stop, or use a matching servef.", status.Protocol, Protocol)}
	}
	if parsed && err == nil && status.Socket != "" {
		return status, nil
	}
	detail := strings.TrimSpace(string(stderr))
	if len(detail) > 2000 {
		detail = detail[len(detail)-2000:]
	}
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = "servef daemon ensure printed no status"
	}
	return contract.DaemonStatus{}, &EnsureError{Detail: detail}
}

func lastStatus(stdout []byte) (contract.DaemonStatus, bool) {
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	var status contract.DaemonStatus
	if len(lines) == 0 || json.Unmarshal([]byte(strings.TrimSpace(lines[len(lines)-1])), &status) != nil || status.Protocol == 0 {
		return contract.DaemonStatus{}, false
	}
	return status, true
}
