package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
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
}

func defaultAppDeps() appDeps {
	return appDeps{
		goos:      runtime.GOOS,
		getenv:    os.Getenv,
		installed: desktopapp.Installed,
		openApp:   desktopapp.Open,
		ensure:    ensureAppDaemon,
		openRoot: func(ctx context.Context, socket, path string) (daemon.OpenResult, error) {
			return daemon.NewClient(socket).Open(ctx, path)
		},
		browser: run,
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
	if inSSHSession(deps.getenv) {
		return fallback("servef remote is not available yet", "")
	}
	if deps.goos != "darwin" && deps.goos != "linux" {
		return serveInBrowser()
	}

	rootSet, err := projects.NewRootSet(opts.paths)
	if err != nil {
		return err
	}
	roots := rootSet.List()
	// NewRootSet rejected missing roots; an unreadable root still fails.
	if discovery := projects.Discover(roots, opts.scanDepth); len(discovery.Skipped) > 0 {
		return discovery.Skipped[0].Err
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

	socket := deps.socket
	if socket == "" {
		if socket, err = daemon.DefaultSocket(); err != nil {
			return fallback(fmt.Sprintf("servef daemon is unavailable (%v)", err), "")
		}
	}
	status, started, err := deps.ensure(ctx, daemonOptions{socket: socket, depth: opts.depth, scanDepth: opts.scanDepth, idleExit: daemon.DefaultIdleExit})
	var mismatch *daemon.ProtocolMismatchError
	if errors.As(err, &mismatch) {
		return fallback(err.Error(), "")
	}
	if err != nil {
		return fallback(fmt.Sprintf("servef daemon is unavailable (%v; see %s.log)", err, socket), "")
	}
	if !started && (status.Depth != opts.depth || status.ScanDepth != opts.scanDepth) {
		_, _ = fmt.Fprintf(stderr, "servef daemon runs with --depth %d --scan-depth %d; stop it with `servef daemon stop` to use --depth %d --scan-depth %d\n",
			status.Depth, status.ScanDepth, opts.depth, opts.scanDepth)
	}

	openCtx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	project := ""
	for _, root := range roots {
		result, err := deps.openRoot(openCtx, socket, root)
		if err != nil {
			return fallback(fmt.Sprintf("servef daemon could not open %s (%v)", root, err), "")
		}
		if project == "" {
			project = result.Project
		}
	}

	target := "servef://local/?" + url.Values{"project": {project}}.Encode()
	if opts.noOpen {
		_, _ = fmt.Fprintln(stdout, target)
		return nil
	}
	if err := deps.openApp(ctx, target); err != nil {
		if errors.Is(err, desktopapp.ErrNotInstalled) {
			return fallback(desktopapp.ErrNotInstalled.Error(), hint)
		}
		return fallback(fmt.Sprintf("could not open the servef desktop app (%v)", err), hint)
	}
	_, _ = fmt.Fprintf(stdout, "opened %s in servef\nurl: %s\n", project, target)
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
