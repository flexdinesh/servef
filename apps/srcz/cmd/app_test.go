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
	"strconv"
	"strings"
	"testing"

	"github.com/flexdinesh/srcz/internal/daemon"
	"github.com/flexdinesh/srcz/internal/desktopapp"
)

// appFake records what bare srcz asked of the desktop app, the daemon, and
// browser mode.
type appFake struct {
	installedCalls int
	ensured        []daemonOptions
	openedRoots    []string
	openedURLs     []string
	served         []options
}

const fakeSocket = "/run/srcz-test/daemon.sock"

// newAppFake answers as a Linux host with the app installed, a freshly
// started daemon, and a project per opened root.
func newAppFake() (*appFake, *appDeps) {
	fake := &appFake{}
	return fake, &appDeps{
		goos:   "linux",
		getenv: desktopEnv,
		socket: fakeSocket,
		installed: func(context.Context) (bool, error) {
			fake.installedCalls++
			return true, nil
		},
		openApp: func(_ context.Context, target string) error {
			fake.openedURLs = append(fake.openedURLs, target)
			return nil
		},
		ensure: func(_ context.Context, opts daemonOptions) (daemon.Status, bool, error) {
			fake.ensured = append(fake.ensured, opts)
			return daemon.Status{Protocol: daemon.ProtocolVersion, Depth: opts.depth, ScanDepth: opts.scanDepth}, true, nil
		},
		openRoot: func(_ context.Context, socket, path string) (daemon.OpenResult, error) {
			fake.openedRoots = append(fake.openedRoots, path)
			return daemon.OpenResult{Root: path, Project: path}, nil
		},
		browser: func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error {
			fake.served = append(fake.served, opts)
			return nil
		},
		terminal: func(io.Writer) bool { return false },
	}
}

// desktopEnv is a Linux desktop session's environment.
func desktopEnv(name string) string {
	if name == "WAYLAND_DISPLAY" {
		return "wayland-0"
	}
	return ""
}

func appRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func appOptions(paths ...string) options {
	return options{paths: paths, depth: 5, scanDepth: 10, host: defaultHost}
}

func runAppWith(opts options, deps *appDeps) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := openInApp(context.Background(), opts, strings.NewReader(""), &stdout, &stderr, *deps)
	return stdout.String(), stderr.String(), err
}

func TestAppOpensProjectInDesktopApp(t *testing.T) {
	root := appRoot(t)
	fake, deps := newAppFake()
	deps.openRoot = func(_ context.Context, _, path string) (daemon.OpenResult, error) {
		fake.openedRoots = append(fake.openedRoots, path)
		return daemon.OpenResult{Root: path, Project: "/work/my repo"}, nil
	}

	stdout, stderr, err := runAppWith(appOptions(root), deps)
	if err != nil {
		t.Fatal(err)
	}
	want := "srcz://local/?project=%2Fwork%2Fmy+repo"
	if stdout != "opened /work/my repo in srcz\ndesktop: "+want+"\n" || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
	if !reflect.DeepEqual(fake.openedURLs, []string{want}) {
		t.Errorf("opened %v, want %s", fake.openedURLs, want)
	}
	if want := []daemonOptions{{socket: fakeSocket, depth: 5, scanDepth: 10, idleExit: daemon.DefaultIdleExit}}; !reflect.DeepEqual(fake.ensured, want) {
		t.Errorf("ensured %+v, want %+v", fake.ensured, want)
	}
	if !reflect.DeepEqual(fake.openedRoots, []string{root}) || fake.installedCalls != 1 || len(fake.served) != 0 {
		t.Errorf("fake = %+v", fake)
	}
}

func TestAppOpensEveryPathAndTargetsTheFirstProject(t *testing.T) {
	parent := appRoot(t)
	first, second := filepath.Join(parent, "b-first"), filepath.Join(parent, "a-second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake, deps := newAppFake()

	stdout, _, err := runAppWith(appOptions(first, second), deps)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.openedRoots, []string{first, second}) {
		t.Errorf("opened roots %v, want %s then %s", fake.openedRoots, first, second)
	}
	if !strings.HasPrefix(stdout, "opened "+first+" in srcz\n") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestAppNoOpenPrintsTheURL(t *testing.T) {
	root := appRoot(t)
	fake, deps := newAppFake()
	opts := appOptions(root)
	opts.noOpen = true

	stdout, stderr, err := runAppWith(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if want := "srcz://local/?project=" + url.QueryEscape(root) + "\n"; stdout != want || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q; want %q", stdout, stderr, want)
	}
	if len(fake.openedURLs) != 0 || len(fake.served) != 0 {
		t.Errorf("fake = %+v; want nothing opened or served", fake)
	}
}

func TestAppServesInBrowserWithoutTouchingTheApp(t *testing.T) {
	root := appRoot(t)
	for _, test := range []struct {
		name   string
		opts   func(*options)
		deps   func(*appDeps)
		notice string
	}{
		{"host", func(o *options) { o.host, o.hostSet = "127.0.0.1", true }, nil, ""},
		{"port", func(o *options) { o.port, o.portSet = 8080, true }, nil, ""},
		{"port inside SSH", func(o *options) { o.port, o.portSet = 8080, true }, func(d *appDeps) { d.getenv = sshEnv }, ""},
		{"windows", nil, func(d *appDeps) { d.goos = "windows" }, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake, deps := newAppFake()
			opts := appOptions(root)
			if test.opts != nil {
				test.opts(&opts)
			}
			if test.deps != nil {
				test.deps(deps)
			}
			stdout, stderr, err := runAppWith(opts, deps)
			if err != nil {
				t.Fatal(err)
			}
			if stdout != "" || stderr != test.notice {
				t.Errorf("stdout = %q, stderr = %q; want notice %q", stdout, stderr, test.notice)
			}
			if !reflect.DeepEqual(fake.served, []options{opts}) {
				t.Errorf("served %+v, want %+v", fake.served, opts)
			}
			if fake.installedCalls != 0 || len(fake.ensured) != 0 || len(fake.openedURLs) != 0 {
				t.Errorf("fake = %+v; want the app and daemon untouched", fake)
			}
		})
	}
}

func TestAppFallsBackToBrowserWithANotice(t *testing.T) {
	root := appRoot(t)
	linuxHint := desktopapp.InstallHint("linux")
	mismatch := &daemon.ProtocolMismatchError{Daemon: 99, Client: daemon.ProtocolVersion, Socket: fakeSocket, DaemonVersion: "9.9.9"}
	for _, test := range []struct {
		name string
		deps func(*appFake, *appDeps)
		// want is the stderr notice; hint appends the install hint.
		want     string
		hint     bool
		ensured  int
		appCalls int
	}{
		{"not installed", func(_ *appFake, d *appDeps) {
			d.installed = func(context.Context) (bool, error) { return false, nil }
		}, "srcz desktop app is not installed; serving in browser mode\n", true, 0, 0},
		{"detection failure", func(_ *appFake, d *appDeps) {
			d.installed = func(context.Context) (bool, error) { return false, errors.New("xdg-mime broke") }
		}, "could not detect the srcz desktop app (xdg-mime broke); serving in browser mode\n", true, 0, 0},
		{"ensure failure", func(f *appFake, d *appDeps) {
			d.ensure = func(_ context.Context, opts daemonOptions) (daemon.Status, bool, error) {
				f.ensured = append(f.ensured, opts)
				return daemon.Status{}, true, errors.New("srcz daemon did not start")
			}
		}, "srcz daemon is unavailable (srcz daemon did not start; see " + fakeSocket + ".log); serving in browser mode\n", false, 1, 0},
		{"protocol mismatch", func(f *appFake, d *appDeps) {
			d.ensure = func(_ context.Context, opts daemonOptions) (daemon.Status, bool, error) {
				f.ensured = append(f.ensured, opts)
				return daemon.Status{Protocol: 99}, false, mismatch
			}
		}, mismatch.Error() + "; serving in browser mode\n", false, 1, 0},
		{"open root failure", func(_ *appFake, d *appDeps) {
			d.openRoot = func(context.Context, string, string) (daemon.OpenResult, error) {
				return daemon.OpenResult{}, &daemon.ControlError{Status: 400, Code: "invalidRoot", Message: "nope"}
			}
		}, "srcz daemon could not open " + root + " (srcz daemon: nope); serving in browser mode\n", false, 1, 0},
		{"no URL handler", func(f *appFake, d *appDeps) {
			d.openApp = func(_ context.Context, target string) error {
				f.openedURLs = append(f.openedURLs, target)
				return desktopapp.ErrNotInstalled
			}
		}, "srcz desktop app is not installed; serving in browser mode\n", true, 1, 1},
		{"open failure", func(f *appFake, d *appDeps) {
			d.openApp = func(_ context.Context, target string) error {
				f.openedURLs = append(f.openedURLs, target)
				return errors.New("xdg-open failed")
			}
		}, "could not open the srcz desktop app (xdg-open failed); serving in browser mode\n", true, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake, deps := newAppFake()
			test.deps(fake, deps)
			opts := appOptions(root)
			stdout, stderr, err := runAppWith(opts, deps)
			if err != nil {
				t.Fatal(err)
			}
			want := test.want
			if test.hint {
				want += linuxHint
			}
			if stderr != want {
				t.Errorf("stderr = %q, want %q", stderr, want)
			}
			if stdout != "" {
				t.Errorf("stdout = %q", stdout)
			}
			if !reflect.DeepEqual(fake.served, []options{opts}) {
				t.Errorf("served %+v, want the original options %+v", fake.served, opts)
			}
			if len(fake.ensured) != test.ensured || len(fake.openedURLs) != test.appCalls {
				t.Errorf("ensured %d times, opened the app %d times; want %d, %d", len(fake.ensured), len(fake.openedURLs), test.ensured, test.appCalls)
			}
		})
	}
}

func TestAppOnMacOSLetsOpenDecide(t *testing.T) {
	root := appRoot(t)
	fake, deps := newAppFake()
	deps.goos = "darwin"
	deps.openApp = func(context.Context, string) error { return desktopapp.ErrNotInstalled }

	_, stderr, err := runAppWith(appOptions(root), deps)
	if err != nil {
		t.Fatal(err)
	}
	if fake.installedCalls != 0 {
		t.Errorf("checked installation %d times on macOS", fake.installedCalls)
	}
	if want := "srcz desktop app is not installed; serving in browser mode\n" + desktopapp.InstallHint("darwin"); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if len(fake.served) != 1 {
		t.Errorf("served %d times, want once", len(fake.served))
	}
}

func TestAppRejectsInvalidPathsBeforeAnything(t *testing.T) {
	parent := appRoot(t)
	for _, paths := range [][]string{
		{filepath.Join(parent, "missing")},
		{parent, filepath.Join(parent, "inner")},
	} {
		if err := os.MkdirAll(filepath.Join(parent, "inner"), 0o755); err != nil {
			t.Fatal(err)
		}
		fake, deps := newAppFake()
		stdout, stderr, err := runAppWith(appOptions(paths...), deps)
		if err == nil {
			t.Errorf("paths %v accepted", paths)
		}
		if stdout != "" || stderr != "" || fake.installedCalls != 0 || len(fake.ensured) != 0 || len(fake.served) != 0 {
			t.Errorf("paths %v: stdout %q, stderr %q, fake %+v; want nothing before the error", paths, stdout, stderr, fake)
		}
	}
}

func TestAppWarnsWhenTheRunningDaemonUsesOtherDepths(t *testing.T) {
	root := appRoot(t)
	for _, test := range []struct {
		name    string
		started bool
		depth   int
		scan    int
		warn    bool
	}{
		{"running with other depths", false, 3, 10, true},
		{"running with other scan depth", false, 5, 2, true},
		{"running with the same depths", false, 5, 10, false},
		{"started by this call", true, 3, 2, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, deps := newAppFake()
			deps.ensure = func(context.Context, daemonOptions) (daemon.Status, bool, error) {
				return daemon.Status{Protocol: daemon.ProtocolVersion, Depth: test.depth, ScanDepth: test.scan}, test.started, nil
			}
			stdout, stderr, err := runAppWith(appOptions(root), deps)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(stdout, "opened ") {
				t.Errorf("stdout = %q", stdout)
			}
			warning := "srcz daemon runs with --depth " + strconv.Itoa(test.depth) + " --scan-depth " + strconv.Itoa(test.scan) + "; stop it with `srcz daemon stop` to use --depth 5 --scan-depth 10\n"
			if test.warn && stderr != warning {
				t.Errorf("stderr = %q, want %q", stderr, warning)
			}
			if !test.warn && stderr != "" {
				t.Errorf("stderr = %q, want no warning", stderr)
			}
		})
	}
}

const testSSHConnection = "10.0.0.9 51234 10.0.0.5 22"

func sshEnv(name string) string {
	if name == "SSH_CONNECTION" {
		return testSSHConnection
	}
	return ""
}

func TestChooseLaunch(t *testing.T) {
	noEnv := func(string) string { return "" }
	for _, test := range []struct {
		name   string
		opts   options
		goos   string
		getenv func(string) string
		want   launch
	}{
		{"desktop session", options{}, "linux", desktopEnv, launchDesktop},
		{"macOS", options{}, "darwin", noEnv, launchDesktop},
		{"windows", options{}, "windows", noEnv, launchBrowser},
		{"explicit host", options{hostSet: true}, "linux", desktopEnv, launchBrowser},
		{"explicit port over SSH", options{portSet: true}, "linux", sshEnv, launchBrowser},
		{"SSH", options{}, "linux", sshEnv, launchNoUI},
		{"SSH on macOS", options{}, "darwin", sshEnv, launchNoUI},
		{"Linux without a display", options{}, "linux", noEnv, launchNoUI},
		{"remote over SSH", options{remote: true}, "linux", sshEnv, launchRemote},
		{"remote on a desktop", options{remote: true}, "darwin", noEnv, launchRemote},
		{"remote with an address", options{remote: true, hostSet: true, portSet: true}, "linux", sshEnv, launchRemote},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := chooseLaunch(test.opts, test.goos, test.getenv); got != test.want {
				t.Errorf("chooseLaunch() = %d, want %d", got, test.want)
			}
		})
	}
}

// No UI here must never quietly open a port: the old default did.
func TestAppWithoutAUIGivesAdviceAndServesNothing(t *testing.T) {
	root := appRoot(t)
	fake, deps := newAppFake()
	deps.getenv = sshEnv
	stdout, stderr, err := runAppWith(appOptions(root), deps)
	if !errors.Is(err, errNoUI) || !strings.Contains(err.Error(), "--remote") {
		t.Errorf("err = %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
	if len(fake.served) != 0 || len(fake.ensured) != 0 || len(fake.openedURLs) != 0 {
		t.Errorf("fake = %+v; want nothing started", fake)
	}
}

func TestAppHandsRemoteToTheRemoteFlow(t *testing.T) {
	root := appRoot(t)
	fake, deps := newAppFake()
	deps.getenv = sshEnv
	var got []options
	deps.remote = func(_ context.Context, opts options, _, _ io.Writer) error {
		got = append(got, opts)
		return nil
	}
	opts := appOptions(root)
	opts.remote = true
	if _, _, err := runAppWith(opts, deps); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []options{opts}) || len(fake.served) != 0 {
		t.Errorf("remote got %+v, fake = %+v", got, fake)
	}
}

func TestAppHyperlinksTheDesktopURLInTerminals(t *testing.T) {
	root := appRoot(t)
	_, deps := newAppFake()
	deps.terminal = func(io.Writer) bool { return true }
	stdout, _, err := runAppWith(appOptions(root), deps)
	if err != nil {
		t.Fatal(err)
	}
	target := "srcz://local/?project=" + url.QueryEscape(root)
	if want := "opened " + root + " in srcz\ndesktop: " + hyperlink(target, true) + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	deps.getenv = func(name string) string {
		if name == "TERM" {
			return "dumb"
		}
		return desktopEnv(name)
	}
	if stdout, _, _ := runAppWith(appOptions(root), deps); strings.Contains(stdout, "\x1b") {
		t.Errorf("TERM=dumb stdout = %q, want no escape sequences", stdout)
	}
}
