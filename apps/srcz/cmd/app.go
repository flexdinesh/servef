package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"

	"github.com/flexdinesh/srcz/internal/daemon"
	"github.com/flexdinesh/srcz/internal/desktopapp"
	"github.com/flexdinesh/srcz/internal/projects"
)

// appDeps are the effects bare srcz uses to open the desktop app; tests
// replace them.
type appDeps struct {
	goos      string
	getenv    func(string) string
	socket    string // "" uses daemon.DefaultSocket()
	installed func(context.Context) (bool, error)
	openApp   func(context.Context, string) error
	ensure    func(context.Context, daemonOptions) (daemon.Status, bool, error)
	openRoot  func(ctx context.Context, socket, path string) (daemon.OpenResult, error)
	browser   runner
	terminal  func(io.Writer) bool
	remote    func(context.Context, options, io.Writer, io.Writer) error
}

func (d appDeps) daemonRoots() daemonRootsDeps {
	return daemonRootsDeps{socket: d.socket, ensure: d.ensure, openRoot: d.openRoot}
}

func defaultAppDeps() appDeps {
	return appDeps{
		goos:      runtime.GOOS,
		getenv:    os.Getenv,
		installed: desktopapp.Installed,
		openApp:   desktopapp.Open,
		ensure:    ensureAppDaemon,
		openRoot:  openDaemonRoot,
		browser:   run,
		terminal:  isTerminal,
		remote:    runRemote,
	}
}

// launch is what bare srcz does.
type launch int

const (
	launchDesktop launch = iota
	launchBrowser
	launchRemote
	launchNoUI
)

// chooseLaunch decides from the flags and the machine. An explicit address
// asks for a server; a machine with no UI gets advice, never an open port.
func chooseLaunch(opts options, goos string, getenv func(string) string) launch {
	switch {
	case opts.remote:
		return launchRemote
	case opts.hostSet || opts.portSet:
		return launchBrowser
	case noUI(goos, getenv):
		return launchNoUI
	case goos != "darwin" && goos != "linux":
		return launchBrowser
	default:
		return launchDesktop
	}
}

// errNoUI is bare srcz on a machine with no UI (decision 0098).
var errNoUI = errors.New("no display here; run `srcz <path> --remote` to open this machine's files from your computer")

// runApp opens the paths in the desktop app through the daemon, falling back
// to browser mode with a notice when the app or daemon is unavailable.
func runApp(ctx context.Context, opts options, stdin io.Reader, stdout, stderr io.Writer) error {
	return openInApp(ctx, opts, stdin, stdout, stderr, defaultAppDeps())
}

func openInApp(ctx context.Context, opts options, stdin io.Reader, stdout, stderr io.Writer, deps appDeps) error {
	serveInBrowser := func() error { return deps.browser(ctx, opts, stdin, stdout, stderr) }
	fallback := func(notice, hint string) error {
		_, _ = fmt.Fprintf(stderr, "%s; serving in browser mode\n", notice)
		_, _ = io.WriteString(stderr, hint)
		return serveInBrowser()
	}

	switch chooseLaunch(opts, deps.goos, deps.getenv) {
	case launchRemote:
		return deps.remote(ctx, opts, stdout, stderr)
	case launchBrowser:
		return serveInBrowser()
	case launchNoUI:
		return errNoUI
	case launchDesktop:
	}

	roots, err := validateRoots(opts.paths, opts.scanDepth)
	if err != nil {
		return err
	}

	hint := desktopapp.InstallHint(deps.goos)
	if deps.goos == "linux" {
		installed, err := deps.installed(ctx)
		if err != nil {
			return fallback(fmt.Sprintf("could not detect the srcz desktop app (%v)", err), hint)
		}
		if !installed {
			return fallback(desktopapp.ErrNotInstalled.Error(), hint)
		}
	}

	project, err := openDaemonRoots(ctx, roots, opts.depth, opts.scanDepth, deps.daemonRoots(), stderr)
	if err != nil {
		return fallback(err.Error(), "")
	}

	target := "srcz://local/?" + url.Values{"project": {project}}.Encode()
	link := hyperlink(target, deps.terminal(stdout) && deps.getenv("TERM") != "dumb")
	if opts.noOpen {
		_, _ = fmt.Fprintln(stdout, link)
		return nil
	}
	if err := deps.openApp(ctx, target); err != nil {
		if errors.Is(err, desktopapp.ErrNotInstalled) {
			return fallback(desktopapp.ErrNotInstalled.Error(), hint)
		}
		return fallback(fmt.Sprintf("could not open the srcz desktop app (%v)", err), hint)
	}
	_, _ = fmt.Fprintf(stdout, "opened %s in srcz\ndesktop: %s\n", project, link)
	return nil
}

// ensureAppDaemon starts the daemon unless it runs, as `srcz daemon ensure`
// does.
func ensureAppDaemon(ctx context.Context, opts daemonOptions) (daemon.Status, bool, error) {
	if err := daemonSupported(); err != nil {
		return daemon.Status{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	return daemon.Ensure(ctx, opts.socket, startDaemon(opts))
}

func openDaemonRoot(ctx context.Context, socket, path string) (daemon.OpenResult, error) {
	return daemon.NewClient(socket).Open(ctx, path)
}

// validateRoots checks paths and discovery as browser mode does.
func validateRoots(paths []string, scanDepth int) ([]string, error) {
	rootSet, err := projects.NewRootSet(paths)
	if err != nil {
		return nil, err
	}
	roots := rootSet.List()
	// NewRootSet rejected missing roots; an unreadable root still fails.
	if discovery := projects.Discover(roots, scanDepth); len(discovery.Skipped) > 0 {
		return nil, discovery.Skipped[0].Err
	}
	return roots, nil
}

// daemonRootsDeps are the daemon effects bare srcz uses.
type daemonRootsDeps struct {
	socket   string // "" uses daemon.DefaultSocket()
	ensure   func(context.Context, daemonOptions) (daemon.Status, bool, error)
	openRoot func(ctx context.Context, socket, path string) (daemon.OpenResult, error)
}

// openDaemonRoots ensures the daemon and opens each root in it, returning
// the first root's project. Its errors read as notices. depth and scanDepth
// apply to a daemon it starts; a running one with others gets a warning.
func openDaemonRoots(ctx context.Context, roots []string, depth, scanDepth int, deps daemonRootsDeps, stderr io.Writer) (string, error) {
	socket := deps.socket
	if socket == "" {
		var err error
		if socket, err = daemon.DefaultSocket(); err != nil {
			return "", fmt.Errorf("srcz daemon is unavailable (%w)", err)
		}
	}
	status, started, err := deps.ensure(ctx, daemonOptions{socket: socket, depth: depth, scanDepth: scanDepth, idleExit: daemon.DefaultIdleExit})
	var mismatch *daemon.ProtocolMismatchError
	if errors.As(err, &mismatch) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("srcz daemon is unavailable (%w; see %s.log)", err, socket)
	}
	if !started && (status.Depth != depth || status.ScanDepth != scanDepth) {
		_, _ = fmt.Fprintf(stderr, "srcz daemon runs with --depth %d --scan-depth %d; stop it with `srcz daemon stop` to use --depth %d --scan-depth %d\n",
			status.Depth, status.ScanDepth, depth, scanDepth)
	}

	openCtx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	project := ""
	for _, root := range roots {
		result, err := deps.openRoot(openCtx, socket, root)
		if err != nil {
			return "", fmt.Errorf("srcz daemon could not open %s (%w)", root, err)
		}
		if project == "" {
			project = result.Project
		}
	}
	return project, nil
}
