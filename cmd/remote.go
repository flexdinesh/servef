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
	"github.com/spf13/cobra"
)

// servef remote (decision 0069): run on an SSH host, it opens the paths in
// that host's daemon and prints a servef://ssh link for the user's desktop
// app.

type remoteOptions struct {
	paths            []string
	depth, scanDepth int
	alias            string
}

// remoteDeps are the effects servef remote uses; tests replace them.
type remoteDeps struct {
	goos     string
	getenv   func(string) string
	hostname func() (string, error)
	username func() string
	terminal func(io.Writer) bool
	daemon   daemonRootsDeps
}

func defaultRemoteDeps() remoteDeps {
	return remoteDeps{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		hostname: os.Hostname,
		username: currentUsername,
		terminal: isTerminal,
		daemon:   daemonRootsDeps{ensure: ensureAppDaemon, openRoot: openDaemonRoot},
	}
}

// remoteUnavailableError is a failure after the paths validated: the daemon
// or this host's name is unavailable. Bare servef falls back to browser mode
// on it.
type remoteUnavailableError struct{ err error }

func (e *remoteUnavailableError) Error() string { return e.err.Error() }
func (e *remoteUnavailableError) Unwrap() error { return e.err }

type remoteRunner func(context.Context, remoteOptions, io.Writer, io.Writer) error

func newRemoteCommand(run remoteRunner) *cobra.Command {
	var opts remoteOptions
	command := &cobra.Command{
		Use:   "remote [path]",
		Short: "Open paths on this SSH host and print a link for the servef desktop app",
		Long: "Open paths in this host's servef daemon and print a servef://ssh link.\n" +
			"Open the link on your computer; the servef desktop app connects over SSH.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, args []string) error {
			opts.paths = append(args, opts.paths...)
			if len(opts.paths) == 0 {
				opts.paths = []string{"."}
			}
			if opts.depth < 0 {
				return errors.New("--depth must be zero or greater")
			}
			if opts.scanDepth < 0 {
				return errors.New("--scan-depth must be zero or greater")
			}
			return run(command.Context(), opts, command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	flags := command.Flags()
	flags.StringArrayVar(&opts.paths, "path", nil, "add a directory to serve (repeatable); a positional path is shorthand for one")
	flags.IntVar(&opts.depth, "depth", 5, "maximum directory depth to scan within a project")
	flags.IntVar(&opts.scanDepth, "scan-depth", 10, "maximum directory depth to search for git repositories")
	flags.StringVar(&opts.alias, "alias", "", "name this host in the link, such as its ~/.ssh/config Host alias on your computer (default: the hostname)")
	return command
}

func runRemote(ctx context.Context, opts remoteOptions, stdout, stderr io.Writer) error {
	_, err := openRemote(ctx, opts, stdout, stderr, defaultRemoteDeps())
	return err
}

// openRemote validates the paths, ensures the daemon, opens each root, prints
// the link to the first project, and returns it. Failures after validation
// are *remoteUnavailableError.
func openRemote(ctx context.Context, opts remoteOptions, stdout, stderr io.Writer, deps remoteDeps) (string, error) {
	if opts.alias != "" && !validAlias(opts.alias) {
		return "", errors.New("--alias must start with a letter or digit and contain only letters, digits, '.', '_', and '-' (at most 253 characters)")
	}
	if deps.goos != "darwin" && deps.goos != "linux" {
		return "", &remoteUnavailableError{daemon.ErrUnsupported}
	}
	roots, err := validateRoots(opts.paths, opts.scanDepth)
	if err != nil {
		return "", err
	}
	hints := hintsFromEnv(deps.getenv, deps.hostname, deps.username, opts.alias)
	if hints.Host() == "" {
		return "", &remoteUnavailableError{errors.New("could not read this host's name for the link; name it with servef remote --alias")}
	}

	project, err := openDaemonRoots(ctx, roots, opts.depth, opts.scanDepth, deps.daemon, stderr)
	if err != nil {
		return "", &remoteUnavailableError{err}
	}

	link := remoteLink(hints, "/", url.Values{"project": {project}})
	if !inSSHSession(deps.getenv) {
		_, _ = fmt.Fprintln(stderr, "not in an SSH session; the link's host hints may not identify this host to your computer")
	}
	for _, root := range roots {
		_, _ = fmt.Fprintf(stdout, "serving %s on %s\n", root, hints.Host())
	}
	tty := deps.terminal(stdout) && deps.getenv("TERM") != "dumb"
	_, _ = fmt.Fprintln(stdout, hyperlink(link, tty))
	_, _ = fmt.Fprintln(stdout, "open the link on your computer; it needs the servef desktop app")
	return link, nil
}

// isTerminal reports whether w is a terminal device.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
