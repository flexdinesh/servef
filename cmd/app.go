package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"

	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/internal/desktopapp"
	"github.com/flexdinesh/servef/internal/projects"
)

// appDeps are the effects bare servef uses to open the desktop app; tests
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
	// route is the default route's local address on no-UI machines.
	route func() net.IP
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
		route:     outboundAddress,
	}
}

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

	// An explicit address asks for a server.
	if opts.hostSet || opts.portSet {
		return serveInBrowser()
	}
	// Without a UI here, serve on this machine's address so another machine
	// can open the printed links (decision 0076).
	if noUI(deps.goos, deps.getenv) {
		opts.host = detectHost(deps.getenv, deps.route)
		if opts.host == "" {
			opts.host = defaultHost
			_, _ = fmt.Fprintln(stderr, "could not detect this machine's address; serving on localhost")
		}
		opts.noOpen = true
		return serveInBrowser()
	}
	if deps.goos != "darwin" && deps.goos != "linux" {
		return serveInBrowser()
	}

	roots, err := validateRoots(opts.paths, opts.scanDepth)
	if err != nil {
		return err
	}

	hint := desktopapp.InstallHint(deps.goos)
	if deps.goos == "linux" {
		installed, err := deps.installed(ctx)
		if err != nil {
			return fallback(fmt.Sprintf("could not detect the servef desktop app (%v)", err), hint)
		}
		if !installed {
			return fallback(desktopapp.ErrNotInstalled.Error(), hint)
		}
	}

	project, err := openDaemonRoots(ctx, roots, opts.depth, opts.scanDepth, deps.daemonRoots(), stderr)
	if err != nil {
		return fallback(err.Error(), "")
	}

	target := "servef://local/?" + url.Values{"project": {project}}.Encode()
	link := hyperlink(target, deps.terminal(stdout) && deps.getenv("TERM") != "dumb")
	if opts.noOpen {
		_, _ = fmt.Fprintln(stdout, link)
		return nil
	}
	if err := deps.openApp(ctx, target); err != nil {
		if errors.Is(err, desktopapp.ErrNotInstalled) {
			return fallback(desktopapp.ErrNotInstalled.Error(), hint)
		}
		return fallback(fmt.Sprintf("could not open the servef desktop app (%v)", err), hint)
	}
	_, _ = fmt.Fprintf(stdout, "opened %s in servef\ndesktop: %s\n", project, link)
	return nil
}

// ensureAppDaemon starts the daemon unless it runs, as `servef daemon ensure`
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

// daemonRootsDeps are the daemon effects bare servef uses.
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
			return "", fmt.Errorf("servef daemon is unavailable (%w)", err)
		}
	}
	status, started, err := deps.ensure(ctx, daemonOptions{socket: socket, depth: depth, scanDepth: scanDepth, idleExit: daemon.DefaultIdleExit})
	var mismatch *daemon.ProtocolMismatchError
	if errors.As(err, &mismatch) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("servef daemon is unavailable (%w; see %s.log)", err, socket)
	}
	if !started && (status.Depth != depth || status.ScanDepth != scanDepth) {
		_, _ = fmt.Fprintf(stderr, "servef daemon runs with --depth %d --scan-depth %d; stop it with `servef daemon stop` to use --depth %d --scan-depth %d\n",
			status.Depth, status.ScanDepth, depth, scanDepth)
	}

	openCtx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	project := ""
	for _, root := range roots {
		result, err := deps.openRoot(openCtx, socket, root)
		if err != nil {
			return "", fmt.Errorf("servef daemon could not open %s (%w)", root, err)
		}
		if project == "" {
			project = result.Project
		}
	}
	return project, nil
}
