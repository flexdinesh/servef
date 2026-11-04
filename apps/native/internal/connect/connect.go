// Package connect makes the connections native browses: the local daemon
// over its Unix socket, ensured with the bundled servef binary as the
// Electron app does (decision 0067), and servef http servers over TCP
// (decisions 0071, 0076). Every data call goes through pkg/client.
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

	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Protocol is the daemon protocol native speaks (decision 0061).
const Protocol = 1

// EnsureTimeout bounds `servef daemon ensure`.
const EnsureTimeout = 15 * time.Second

// Connection is one place native browses.
type Connection struct {
	// ID is "local" or "http-<host>-<port>"; Label names it in the UI.
	ID     string
	Label  string
	Daemon bool
	Client client.Client
	// Target is the connection as a link's target, without a project.
	Target link.Target
}

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
// in a packaged app; otherwise SERVEF_NATIVE_BINARY, else servef on PATH.
func Locate(packaged bool, resources string, getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	if packaged {
		return filepath.Join(resources, "bin", "servef"), nil
	}
	if override := getenv("SERVEF_NATIVE_BINARY"); override != "" {
		return override, nil
	}
	if found, err := lookPath("servef"); err == nil && filepath.IsAbs(found) {
		return found, nil
	}
	return "", errors.New("servef is not on PATH and SERVEF_NATIVE_BINARY is not set")
}

// DevFlags maps SERVEF_NATIVE_SOCKET and SERVEF_NATIVE_ROOTS_FILE to the
// daemon's --socket and --roots-file, for an isolated development daemon.
func DevFlags(getenv func(string) string) []string {
	var flags []string
	if socket := getenv("SERVEF_NATIVE_SOCKET"); socket != "" {
		flags = append(flags, "--socket", socket)
	}
	if roots := getenv("SERVEF_NATIVE_ROOTS_FILE"); roots != "" {
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
			"The running daemon speaks protocol %d; servef-native needs protocol %d. Run servef daemon stop, or use a matching servef.", status.Protocol, Protocol)}
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

// Local connects to the daemon at socket.
func Local(socket string) Connection {
	return Connection{ID: "local", Label: "This computer", Daemon: true, Client: client.NewUnix(socket), Target: link.Target{Kind: link.Local}}
}

// Server connects to the servef http server a Server target names.
func Server(target link.Target) Connection {
	origin := target.Origin()
	return Connection{
		ID:     fmt.Sprintf("http-%s-%d", strings.NewReplacer(":", "-", "[", "", "]", "").Replace(target.Host), target.Port),
		Label:  strings.TrimPrefix(origin, "http://"),
		Client: client.NewTCP(origin),
		Target: link.Target{Kind: link.Server, Host: target.Host, Port: target.Port},
	}
}

// ListedServer is a running CLI server, from `servef control list --json`
// (decision 0071).
type ListedServer struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Origin string `json:"origin"`
}

// ListServers lists the CLI servers running on this computer; only
// loopback ones are kept, as the Electron app does.
func ListServers(ctx context.Context, run Run, binary string) ([]ListedServer, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stdout, _, err := run(ctx, binary, "control", "list", "--json")
	if err != nil {
		return nil, err
	}
	var listing struct {
		Servers []ListedServer `json:"servers"`
	}
	if err := json.Unmarshal(stdout, &listing); err != nil {
		return nil, fmt.Errorf("servef control list: %w", err)
	}
	var servers []ListedServer
	seen := map[int]bool{}
	for _, server := range listing.Servers {
		if link.Loopback(server.Host) && server.Port > 0 && server.Port < 65536 && !seen[server.Port] {
			seen[server.Port] = true
			servers = append(servers, server)
		}
	}
	return servers, nil
}

// System connects with the servef binary on this computer.
type System struct {
	Binary string
	Flags  []string
	Run    Run
	// BinaryErr explains a missing binary; Local and Servers fail with it.
	BinaryErr error
}

// Local ensures the daemon and connects to it.
func (s System) Local(ctx context.Context) (Connection, error) {
	if s.BinaryErr != nil {
		return Connection{}, s.BinaryErr
	}
	status, err := Ensure(ctx, s.Run, s.Binary, s.Flags)
	if err != nil {
		return Connection{}, err
	}
	return Local(status.Socket), nil
}

// Server connects to an http server.
func (s System) Server(target link.Target) Connection { return Server(target) }

// Servers lists the CLI servers running on this computer.
func (s System) Servers(ctx context.Context) ([]ListedServer, error) {
	if s.BinaryErr != nil {
		return nil, s.BinaryErr
	}
	return ListServers(ctx, s.Run, s.Binary)
}
