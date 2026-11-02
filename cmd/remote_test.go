package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/internal/daemon"
)

const testSSHConnection = "10.0.0.9 51234 10.0.0.5 22"

// remoteFake records what servef remote asked of the daemon.
type remoteFake struct {
	ensured     []daemonOptions
	openedRoots []string
}

// newRemoteFake answers as a Linux SSH session on devbox with a freshly
// started daemon and a project per opened root, writing to a pipe.
func newRemoteFake() (*remoteFake, *remoteDeps) {
	fake := &remoteFake{}
	return fake, &remoteDeps{
		goos: "linux",
		getenv: func(name string) string {
			if name == "SSH_CONNECTION" {
				return testSSHConnection
			}
			return ""
		},
		hostname: func() (string, error) { return "devbox", nil },
		username: func() string { return "dee" },
		terminal: func(io.Writer) bool { return false },
		daemon: daemonRootsDeps{
			socket: fakeSocket,
			ensure: func(_ context.Context, opts daemonOptions) (daemon.Status, bool, error) {
				fake.ensured = append(fake.ensured, opts)
				return daemon.Status{Protocol: daemon.ProtocolVersion, Depth: opts.depth, ScanDepth: opts.scanDepth}, true, nil
			},
			openRoot: func(_ context.Context, _, path string) (daemon.OpenResult, error) {
				fake.openedRoots = append(fake.openedRoots, path)
				return daemon.OpenResult{Root: path, Project: path}, nil
			},
		},
	}
}

func remoteOpts(paths ...string) remoteOptions {
	return remoteOptions{paths: paths, depth: 5, scanDepth: 10}
}

func runRemoteWith(opts remoteOptions, deps *remoteDeps) (string, string, string, error) {
	var stdout, stderr bytes.Buffer
	link, err := openRemote(context.Background(), opts, &stdout, &stderr, *deps)
	return link, stdout.String(), stderr.String(), err
}

func TestRemotePrintsTheSSHLink(t *testing.T) {
	root := appRoot(t)
	fake, deps := newRemoteFake()

	link, stdout, stderr, err := runRemoteWith(remoteOpts(root), deps)
	if err != nil {
		t.Fatal(err)
	}
	want := "servef://ssh/devbox/?project=" + url.QueryEscape(root) + "&ssh-hostname=devbox&ssh-user=dee&ssh-addr=10.0.0.5"
	if link != want {
		t.Errorf("link = %s, want %s", link, want)
	}
	if wantOut := "serving " + root + " on devbox\n" + want + "\nopen the link on your computer; it needs the servef desktop app\n"; stdout != wantOut || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q; want %q", stdout, stderr, wantOut)
	}
	if want := []daemonOptions{{socket: fakeSocket, depth: 5, scanDepth: 10, idleExit: daemon.DefaultIdleExit}}; !reflect.DeepEqual(fake.ensured, want) {
		t.Errorf("ensured %+v, want %+v", fake.ensured, want)
	}
	if !reflect.DeepEqual(fake.openedRoots, []string{root}) {
		t.Errorf("opened %v", fake.openedRoots)
	}
}

func TestRemoteAliasNamesTheHost(t *testing.T) {
	root := appRoot(t)
	_, deps := newRemoteFake()
	opts := remoteOpts(root)
	opts.alias = "work-box"

	link, stdout, _, err := runRemoteWith(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "servef://ssh/work-box/?") || !strings.Contains(link, "&ssh-hostname=devbox&") {
		t.Errorf("link = %s", link)
	}
	if !strings.HasPrefix(stdout, "serving "+root+" on work-box\n") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRemoteOpensEveryRootAndLinksTheFirst(t *testing.T) {
	parent := appRoot(t)
	first, second := filepath.Join(parent, "b-first"), filepath.Join(parent, "a-second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake, deps := newRemoteFake()

	link, stdout, _, err := runRemoteWith(remoteOpts(first, second), deps)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.openedRoots, []string{first, second}) {
		t.Errorf("opened %v", fake.openedRoots)
	}
	if !strings.Contains(link, "?project="+url.QueryEscape(first)+"&") {
		t.Errorf("link = %s, want the first project", link)
	}
	if !strings.HasPrefix(stdout, "serving "+first+" on devbox\nserving "+second+" on devbox\n") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRemoteHyperlinksOnlyInTerminals(t *testing.T) {
	root := appRoot(t)
	for _, test := range []struct {
		name     string
		terminal bool
		term     string
		osc8     bool
	}{
		{"terminal", true, "xterm-256color", true},
		{"dumb terminal", true, "dumb", false},
		{"pipe", false, "xterm-256color", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, deps := newRemoteFake()
			deps.terminal = func(io.Writer) bool { return test.terminal }
			getenv := deps.getenv
			deps.getenv = func(name string) string {
				if name == "TERM" {
					return test.term
				}
				return getenv(name)
			}
			link, stdout, _, err := runRemoteWith(remoteOpts(root), deps)
			if err != nil {
				t.Fatal(err)
			}
			line := strings.Split(stdout, "\n")[1]
			if want := hyperlink(link, test.osc8); line != want {
				t.Errorf("link line = %q, want %q", line, want)
			}
		})
	}
}

func TestRemoteOutsideSSHWarns(t *testing.T) {
	root := appRoot(t)
	_, deps := newRemoteFake()
	deps.getenv = func(string) string { return "" }

	link, stdout, stderr, err := runRemoteWith(remoteOpts(root), deps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(link, "ssh-addr") || !strings.Contains(stdout, link) {
		t.Errorf("link = %s, stdout = %q", link, stdout)
	}
	if stderr != "not in an SSH session; the link's host hints may not identify this host to your computer\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRemoteFailures(t *testing.T) {
	root := appRoot(t)
	mismatch := &daemon.ProtocolMismatchError{Daemon: 99, Client: daemon.ProtocolVersion, Socket: fakeSocket, DaemonVersion: "9.9.9"}
	for _, test := range []struct {
		name        string
		opts        func(*remoteOptions)
		deps        func(*remoteDeps)
		want        string
		unavailable bool
		// ensured counts the fake's ensures; failing ensures replace it.
		ensured int
	}{
		{"invalid alias", func(o *remoteOptions) { o.alias = "-oProxyCommand=x" }, nil,
			"--alias must start with a letter or digit and contain only letters, digits, '.', '_', and '-' (at most 253 characters)", false, 0},
		{"unsupported OS", nil, func(d *remoteDeps) { d.goos = "windows" }, daemon.ErrUnsupported.Error(), true, 0},
		{"missing path", func(o *remoteOptions) { o.paths = []string{filepath.Join(root, "missing")} }, nil, "", false, 0},
		{"no host name", nil, func(d *remoteDeps) {
			d.hostname = func() (string, error) { return "", errors.New("no name") }
		}, "could not read this host's name for the link; name it with servef remote --alias", true, 0},
		{"ensure failure", nil, func(d *remoteDeps) {
			d.daemon.ensure = func(context.Context, daemonOptions) (daemon.Status, bool, error) {
				return daemon.Status{}, true, errors.New("servef daemon did not start")
			}
		}, "servef daemon is unavailable (servef daemon did not start; see " + fakeSocket + ".log)", true, 0},
		{"protocol mismatch", nil, func(d *remoteDeps) {
			d.daemon.ensure = func(context.Context, daemonOptions) (daemon.Status, bool, error) {
				return daemon.Status{Protocol: 99}, false, mismatch
			}
		}, mismatch.Error(), true, 0},
		{"open failure", nil, func(d *remoteDeps) {
			d.daemon.openRoot = func(context.Context, string, string) (daemon.OpenResult, error) {
				return daemon.OpenResult{}, &daemon.ControlError{Status: 400, Code: "invalidRoot", Message: "nope"}
			}
		}, "servef daemon could not open " + root + " (servef daemon: nope)", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake, deps := newRemoteFake()
			if test.deps != nil {
				test.deps(deps)
			}
			opts := remoteOpts(root)
			if test.opts != nil {
				test.opts(&opts)
			}
			link, stdout, _, err := runRemoteWith(opts, deps)
			if err == nil {
				t.Fatalf("succeeded with %s", link)
			}
			if test.want != "" && err.Error() != test.want {
				t.Errorf("error = %q, want %q", err, test.want)
			}
			var unavailable *remoteUnavailableError
			if errors.As(err, &unavailable) != test.unavailable {
				t.Errorf("error %v: unavailable = %t, want %t", err, !test.unavailable, test.unavailable)
			}
			if link != "" || stdout != "" {
				t.Errorf("link = %q, stdout = %q; want nothing printed", link, stdout)
			}
			if len(fake.ensured) != test.ensured {
				t.Errorf("ensured %d times, want %d", len(fake.ensured), test.ensured)
			}
		})
	}
	// Unsupported platforms report the daemon's sentinel.
	_, deps := newRemoteFake()
	deps.goos = "windows"
	if _, _, _, err := runRemoteWith(remoteOpts(root), deps); !errors.Is(err, daemon.ErrUnsupported) {
		t.Errorf("windows error = %v, want daemon.ErrUnsupported", err)
	}
}

func TestRemoteCommandParsesArgumentsAndFlags(t *testing.T) {
	parse := func(args ...string) (remoteOptions, error) {
		t.Helper()
		var got remoteOptions
		command := newRemoteCommand(func(_ context.Context, opts remoteOptions, _, _ io.Writer) error {
			got = opts
			return nil
		})
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		return got, command.Execute()
	}
	got, err := parse("repo", "--path", "/srv", "--depth", "3", "--scan-depth", "2", "--alias", "devbox")
	if err != nil {
		t.Fatal(err)
	}
	if want := (remoteOptions{paths: []string{"repo", "/srv"}, depth: 3, scanDepth: 2, alias: "devbox"}); !reflect.DeepEqual(got, want) {
		t.Errorf("options = %+v, want %+v", got, want)
	}
	if got, err = parse(); err != nil || !reflect.DeepEqual(got, remoteOptions{paths: []string{"."}, depth: 5, scanDepth: 10}) {
		t.Errorf("default options = %+v, %v", got, err)
	}
	for _, args := range [][]string{{"a", "b"}, {"--depth", "-1"}, {"--scan-depth", "-1"}, {"--host", "0.0.0.0"}} {
		if _, err := parse(args...); err == nil {
			t.Errorf("%v succeeded", args)
		}
	}
}
