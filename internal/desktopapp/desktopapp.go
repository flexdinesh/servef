// Package desktopapp detects the servef desktop app and opens servef:// URLs
// in it through the operating system's URL handler.
package desktopapp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrNotInstalled reports that no servef:// handler is registered.
var ErrNotInstalled = errors.New("servef desktop app is not installed")

// launchWait bounds how long Open waits for xdg-open to report failure.
const launchWait = 3 * time.Second

// runner executes commands.
type runner interface {
	// Run runs a command and returns its standard output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// Launch starts a command detached from the terminal. It returns the
	// command's exit error if it ends within the runner's wait; a command
	// still running then counts as launched.
	Launch(name string, args ...string) error
}

type execRunner struct {
	wait time.Duration
}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Launch exists because xdg-open without a recognized desktop environment
// runs the handler in the foreground: waiting would tie the app to the
// terminal, and the terminal's signals would reach it.
func (r execRunner) Launch(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.SysProcAttr = detached()
	if err := command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	timer := time.NewTimer(r.wait)
	defer timer.Stop()
	select {
	case err := <-exited:
		return err
	case <-timer.C:
		return nil
	}
}

// Installed reports whether the desktop app handles servef:// URLs. On Linux
// it asks xdg-mime for the x-scheme-handler/servef default; on macOS it is
// always true, since only open can tell; elsewhere it is false.
func Installed(ctx context.Context) (bool, error) {
	return installed(ctx, runtime.GOOS, execRunner{wait: launchWait})
}

// Open opens url, a servef:// URL, in the desktop app. A missing handler
// wraps ErrNotInstalled.
func Open(ctx context.Context, url string) error {
	return open(ctx, url, runtime.GOOS, execRunner{wait: launchWait})
}

func installed(ctx context.Context, goos string, run runner) (bool, error) {
	switch goos {
	case "darwin":
		return true, nil
	case "linux":
		output, err := run.Run(ctx, "xdg-mime", "query", "default", "x-scheme-handler/servef")
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("query servef:// handler: %w", err)
		}
		return strings.TrimSpace(string(output)) != "", nil
	default:
		return false, nil
	}
}

func open(ctx context.Context, url, goos string, run runner) error {
	switch goos {
	case "darwin":
		_, err := run.Run(ctx, "open", url)
		// open exits non-zero when no application handles the scheme.
		var exit interface{ ExitCode() int }
		if errors.As(err, &exit) {
			return fmt.Errorf("%w (open %s: %v)", ErrNotInstalled, url, err)
		}
		if err != nil {
			return fmt.Errorf("open %s: %w", url, err)
		}
		return nil
	case "linux":
		ok, err := installed(ctx, goos, run)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotInstalled
		}
		if err := run.Launch("xdg-open", url); err != nil {
			return fmt.Errorf("xdg-open %s: %w", url, err)
		}
		return nil
	default:
		return ErrNotInstalled
	}
}

const (
	releasesURL = "https://github.com/flexdinesh/servef/releases"
	docsURL     = "https://github.com/flexdinesh/servef/blob/main/site/src/pages/docs/desktop.mdx"
	// installScript installs the app and CLI from the latest release (decision 0073).
	installScript = "curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop.sh | sh"
)

// InstallHint returns install instructions for the desktop app on goos.
func InstallHint(goos string) string {
	footer := "  `servef browser` serves in a web browser instead.\n" +
		"  docs: " + docsURL + "\n"
	switch goos {
	case "darwin":
		return "  install the servef desktop app:\n" +
			"    " + installScript + "\n" +
			"  or by hand:\n" +
			"    1. download servef-desktop_<version>_darwin_<arm64|amd64>.dmg from " + releasesURL + "\n" +
			"    2. drag servef to Applications\n" +
			"    3. preview builds are ad-hoc signed: on first launch right-click servef and choose Open\n" +
			"       (or run: xattr -dr com.apple.quarantine /Applications/servef.app)\n" +
			footer
	case "linux":
		return "  install the servef desktop app:\n" +
			"    " + installScript + "\n" +
			"  or, on Debian and Ubuntu, from " + releasesURL + ":\n" +
			"    sudo apt install ./servef-desktop_<version>_linux_<amd64|arm64>.deb\n" +
			"  a hand-installed AppImage registers servef:// only with desktop integration (e.g. AppImageLauncher).\n" +
			footer
	default:
		return "  the servef desktop app is not available on this OS.\n"
	}
}
