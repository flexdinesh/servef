//go:build darwin || linux

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/srcz/internal/daemon"
)

func executeDaemon(t *testing.T, args ...string) (string, daemonOptions, error) {
	t.Helper()
	var got daemonOptions
	var called string
	record := func(name string) daemonAction {
		return func(_ context.Context, opts daemonOptions, _, _ io.Writer) error {
			called, got = name, opts
			return nil
		}
	}
	command := newDaemonCommandWith(daemonActions{run: record("run"), ensure: record("ensure"), stop: record("stop"), status: record("status")})
	command.SetArgs(args)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	err := command.Execute()
	return called, got, err
}

func TestDaemonCommandParsesFlags(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	defaultSocket, err := daemon.DefaultSocket()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args []string
		want string
		opts daemonOptions
	}{
		{[]string{"run"}, "run", daemonOptions{socket: defaultSocket, depth: 5, scanDepth: 10, idleExit: 30 * time.Minute}},
		{[]string{"ensure", "--json", "--socket", "/tmp/sf/d.sock", "--depth", "3", "--scan-depth", "4", "--idle-exit", "0"}, "ensure", daemonOptions{socket: "/tmp/sf/d.sock", depth: 3, scanDepth: 4, json: true}},
		{[]string{"--socket", "/tmp/sf/d.sock", "stop"}, "stop", daemonOptions{socket: "/tmp/sf/d.sock"}},
		{[]string{"status", "--json"}, "status", daemonOptions{socket: defaultSocket, json: true}},
		{[]string{"run", "--roots-file", "/tmp/sf/roots.json"}, "run", daemonOptions{socket: defaultSocket, depth: 5, scanDepth: 10, idleExit: 30 * time.Minute, rootsFile: "/tmp/sf/roots.json"}},
	}
	for _, test := range tests {
		called, opts, err := executeDaemon(t, test.args...)
		if err != nil || called != test.want || opts != test.opts {
			t.Errorf("daemon %v = %s %+v, %v; want %s %+v", test.args, called, opts, err, test.want, test.opts)
		}
	}
	if !strings.HasPrefix(defaultSocket, runtimeDir) {
		t.Errorf("default socket %s is not under XDG_RUNTIME_DIR", defaultSocket)
	}
}

func TestDaemonCommandValidatesFlags(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--depth", "-1"},
		{"ensure", "--scan-depth", "-1"},
		{"run", "--idle-exit", "-1s"},
		{"stop", "--depth", "3"},
		{"status", "--idle-exit", "1m"},
		{"stop", "--json"},
		{"status", "--roots-file", "/tmp/sf/roots.json"},
		{"run", "extra"},
	} {
		if called, _, err := executeDaemon(t, args...); err == nil || called != "" {
			t.Errorf("daemon %v ran %q, error %v; want rejected", args, called, err)
		}
	}
}

func TestRootCommandRoutesDaemonSubcommand(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("browser runner called for daemon")
		return nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"daemon", "status", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "srcz daemon status [flags]") {
		t.Fatalf("help = %q", output.String())
	}
}

type cliResult struct {
	stdout, stderr string
	err            error
}

// srcz runs the test binary as srcz (see TestMain).
func srcz(t *testing.T, env []string, args ...string) cliResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], args...)
	command.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// processGone reports whether pid has exited; a zombie awaiting its reaper
// has exited.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	return len(fields) > 0 && fields[0] == "Z"
}

func TestDaemonCommandsDetachEnsureStopAndStatus(t *testing.T) {
	dir, err := os.MkdirTemp("", "sf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "daemon.sock")
	env := []string{"SRCZ_CMD_TEST_EXEC=1", "XDG_CONFIG_HOME=" + t.TempDir(), "XDG_CACHE_HOME=" + t.TempDir()}
	t.Cleanup(func() { srcz(t, env, "daemon", "stop", "--socket", socket) })

	type ensured struct {
		daemon.Status
		Started *bool `json:"started"`
	}
	decode := func(result cliResult) ensured {
		t.Helper()
		if result.err != nil {
			t.Fatalf("srcz: %v\nstdout %s\nstderr %s", result.err, result.stdout, result.stderr)
		}
		var value ensured
		if err := json.Unmarshal([]byte(result.stdout), &value); err != nil {
			t.Fatalf("output %q: %v", result.stdout, err)
		}
		return value
	}

	first := decode(srcz(t, env, "daemon", "ensure", "--json", "--socket", socket, "--idle-exit", "2m"))
	if first.Started == nil || !*first.Started || first.Protocol != daemon.ProtocolVersion || first.PID == os.Getpid() || first.IdleExit != "2m0s" {
		t.Fatalf("first ensure = %+v", first)
	}
	second := decode(srcz(t, env, "daemon", "ensure", "--json", "--socket", socket))
	if second.Started == nil || *second.Started || second.PID != first.PID {
		t.Fatalf("second ensure = %+v, want started false and pid %d", second, first.PID)
	}
	status := decode(srcz(t, env, "daemon", "status", "--json", "--socket", socket))
	if status.Started != nil || status.PID != first.PID || status.Socket != socket {
		t.Fatalf("status = %+v", status)
	}
	if human := srcz(t, env, "daemon", "status", "--socket", socket); human.err != nil || !strings.Contains(human.stdout, "(protocol 1) at "+socket+", pid ") {
		t.Fatalf("status = %+v", human)
	}

	if stopped := srcz(t, env, "daemon", "stop", "--socket", socket); stopped.err != nil || stopped.stdout != "stopped\n" {
		t.Fatalf("stop = %+v", stopped)
	}
	if again := srcz(t, env, "daemon", "stop", "--socket", socket); again.err != nil || again.stdout != "not running\n" {
		t.Fatalf("second stop = %+v", again)
	}
	if missing := srcz(t, env, "daemon", "status", "--socket", socket); missing.err == nil || !strings.Contains(missing.stderr, "srcz daemon is not running at "+socket) {
		t.Fatalf("status after stop = %+v", missing)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !processGone(first.PID) {
		if time.Now().After(deadline) {
			t.Fatalf("daemon process %d still running after stop", first.PID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDaemonEnsureForwardsRootsFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "sf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "daemon.sock")
	env := []string{"SRCZ_CMD_TEST_EXEC=1", "XDG_CONFIG_HOME=" + t.TempDir(), "XDG_CACHE_HOME=" + t.TempDir()}
	t.Cleanup(func() { srcz(t, env, "daemon", "stop", "--socket", socket) })
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A relative path is forwarded absolute: the started daemon runs in /.
	rootsFile := filepath.Join(t.TempDir(), "roots.json")
	if err := daemon.SaveRoots(rootsFile, []string{root}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(rootsFile))

	result := srcz(t, env, "daemon", "ensure", "--json", "--socket", socket, "--roots-file", filepath.Base(rootsFile))
	var status daemon.Status
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &status) != nil {
		t.Fatalf("ensure = %+v", result)
	}
	if len(status.Roots) != 1 || status.Roots[0] != root {
		t.Fatalf("roots = %v, want %s from --roots-file", status.Roots, root)
	}
}
