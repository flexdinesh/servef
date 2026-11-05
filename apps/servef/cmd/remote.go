package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/internal/remote"
	"github.com/flexdinesh/servef/internal/version"
	"github.com/flexdinesh/servef/pkg/connection"
	"github.com/flexdinesh/servef/pkg/contract"
	"github.com/spf13/cobra"
)

// remoteDeps are the effects `servef --remote` uses; tests replace them.
type remoteDeps struct {
	roots      daemonRootsDeps
	start      func(ctx context.Context, socket string, request contract.DaemonRemoteRequest) (contract.DaemonRemote, error)
	served     func(ctx context.Context, socket string) ([]string, error)
	candidates func() []string
	getenv     func(string) string
	terminal   func(io.Writer) bool
}

func defaultRemoteDeps() remoteDeps {
	return remoteDeps{
		roots:      daemonRootsDeps{ensure: ensureAppDaemon, openRoot: openDaemonRoot},
		start:      startDaemonRemote,
		served:     servedRoots,
		candidates: detectCandidates,
		getenv:     os.Getenv,
		terminal:   isTerminal,
	}
}

func runRemote(ctx context.Context, opts options, stdout, stderr io.Writer) error {
	return serveRemote(ctx, opts, stdout, stderr, defaultRemoteDeps())
}

// serveRemote opens the paths in the daemon, starts its TCP listener, and
// prints how to connect. The daemon outlives this command: it stops itself
// when no UI needs it (decision 0098).
func serveRemote(ctx context.Context, opts options, stdout, stderr io.Writer, deps remoteDeps) error {
	roots, err := validateRoots(opts.paths, opts.scanDepth)
	if err != nil {
		return err
	}
	socket, err := resolveSocket(deps.roots.socket)
	if err != nil {
		return err
	}
	deps.roots.socket = socket
	if _, err := openDaemonRoots(ctx, roots, opts.depth, opts.scanDepth, deps.roots, stderr); err != nil {
		return err
	}
	startCtx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	status, err := deps.start(startCtx, socket, remoteRequest(opts))
	if err != nil {
		return remoteFailure(err)
	}
	served, err := deps.served(startCtx, socket)
	if err != nil {
		return err
	}
	hosts := connectHosts(opts, deps.candidates)
	if len(hosts) == 0 {
		hosts = []string{defaultHost}
		_, _ = fmt.Fprintln(stderr, "could not detect this machine's address; edit the host in the string below, or tunnel with ssh -L")
	}
	endpoint, err := endpointOf(status, hosts)
	if err != nil {
		return err
	}
	info := remoteStartup{
		Version:     "servef " + version.Number(),
		Directories: served,
		Connect:     endpoint.Shell(),
		FirstWait:   opts.remoteFirstWait,
		Grace:       opts.remoteGrace,
	}
	if opts.web {
		if info.Web, err = endpoint.WebURL(); err != nil {
			return err
		}
		info.Links = deps.terminal(stdout) && deps.getenv("TERM") != "dumb"
	}
	writeRemoteStartup(stdout, info)
	return nil
}

func remoteRequest(opts options) contract.DaemonRemoteRequest {
	request := contract.DaemonRemoteRequest{FirstWait: opts.remoteFirstWait.String(), Grace: opts.remoteGrace.String()}
	if opts.hostSet {
		request.Host = opts.host
	}
	if opts.portSet {
		port := opts.port
		request.Port = &port
	}
	return request
}

// connectHosts is what the string offers: the one address asked for, or what
// this machine can be reached by when bound to every interface.
func connectHosts(opts options, candidates func() []string) []string {
	if ip := net.ParseIP(opts.host); opts.hostSet && (ip == nil || !ip.IsUnspecified()) {
		return []string{opts.host}
	}
	return candidates()
}

func detectCandidates() []string {
	return remote.Candidates(os.Getenv("SSH_CONNECTION"), remote.RouteAddress(), remote.Interfaces())
}

func endpointOf(status contract.DaemonRemote, hosts []string) (connection.TLS, error) {
	credential, err := connection.ParseCredential(status.Credential)
	if err != nil {
		return connection.TLS{}, fmt.Errorf("servef daemon sent an unreadable credential: %w", err)
	}
	return connection.TLS{Hosts: hosts, Port: status.Port, Credential: credential}, nil
}

func startDaemonRemote(ctx context.Context, socket string, request contract.DaemonRemoteRequest) (contract.DaemonRemote, error) {
	return daemon.NewClient(socket).RemoteStart(ctx, request)
}

func servedRoots(ctx context.Context, socket string) ([]string, error) {
	return daemon.NewClient(socket).Roots(ctx)
}

func resolveSocket(socket string) (string, error) {
	if socket != "" {
		return socket, nil
	}
	socket, err := daemon.DefaultSocket()
	if err != nil {
		return "", fmt.Errorf("servef daemon is unavailable (%w)", err)
	}
	return socket, nil
}

// remoteFailure explains an answer from a daemon older than --remote.
func remoteFailure(err error) error {
	var failure *daemon.ControlError
	if errors.As(err, &failure) && (failure.Status == http.StatusNotFound || failure.Code == contract.CodeRemoteUnavailable) {
		return errors.New("the running servef daemon cannot serve remotely; stop it with `servef daemon stop` and run this again")
	}
	return err
}

// newRemoteCommand manages the daemon's TCP listener.
func newRemoteCommand() *cobra.Command {
	var socket string
	command := &cobra.Command{
		Use:           "remote",
		Short:         "Manage serving to other machines, started by servef --remote",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Runnable so NoArgs rejects unknown subcommands; bare, it prints help.
		RunE: func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	command.PersistentFlags().StringVar(&socket, "socket", "", "daemon socket path (default: a private per-user directory)")
	subcommand := func(use, short string, action func(context.Context, *daemon.Client, io.Writer) error) *cobra.Command {
		return &cobra.Command{
			Use:           use,
			Short:         short,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE: func(command *cobra.Command, _ []string) error {
				resolved, err := resolveSocket(socket)
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(command.Context(), daemonTimeout)
				defer cancel()
				return action(ctx, daemon.NewClient(resolved), command.OutOrStdout())
			},
		}
	}
	var asJSON bool
	status := subcommand("status", "Print whether this machine serves over TCP, and the connect string", func(ctx context.Context, client *daemon.Client, stdout io.Writer) error {
		return remoteStatus(ctx, client, asJSON, detectCandidates, stdout)
	})
	status.Flags().BoolVar(&asJSON, "json", false, "print status as JSON")
	command.AddCommand(
		status,
		subcommand("stop", "Stop serving over TCP now", remoteStop),
		subcommand("rotate", "Replace the identity; existing connect strings stop working", func(ctx context.Context, client *daemon.Client, stdout io.Writer) error {
			return remoteRotate(ctx, client, remote.DefaultIdentityFile, detectCandidates, stdout)
		}),
	)
	return command
}

func remoteStatus(ctx context.Context, client *daemon.Client, asJSON bool, candidates func() []string, stdout io.Writer) error {
	status, err := client.RemoteStatus(ctx)
	if errors.Is(err, daemon.ErrNotRunning) {
		status, err = contract.DaemonRemote{}, nil
	}
	if err != nil {
		return remoteFailure(err)
	}
	if asJSON {
		return writeDaemonJSON(stdout, status)
	}
	if !status.Listening {
		_, _ = fmt.Fprintln(stdout, "not serving over TCP")
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "serving on port %d, %d connected\n", status.Port, status.Clients)
	return writeConnect(status, candidates, stdout)
}

func writeConnect(status contract.DaemonRemote, candidates func() []string, stdout io.Writer) error {
	hosts := candidates()
	if status.Host != "" && !unspecifiedHost(status.Host) {
		hosts = []string{status.Host}
	}
	if len(hosts) == 0 {
		hosts = []string{defaultHost}
	}
	endpoint, err := endpointOf(status, hosts)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "connect: servef connect %s\n", endpoint.Shell())
	return nil
}

func unspecifiedHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func remoteStop(ctx context.Context, client *daemon.Client, stdout io.Writer) error {
	status, err := client.RemoteStatus(ctx)
	if errors.Is(err, daemon.ErrNotRunning) || (err == nil && !status.Listening) {
		_, _ = fmt.Fprintln(stdout, "not serving over TCP")
		return nil
	}
	if err != nil {
		return remoteFailure(err)
	}
	if err := client.RemoteStop(ctx); err != nil {
		return remoteFailure(err)
	}
	_, _ = fmt.Fprintln(stdout, "stopped serving over TCP")
	return nil
}

// remoteRotate replaces the identity through the daemon, which must drop the
// old one, or directly when none runs.
func remoteRotate(ctx context.Context, client *daemon.Client, identityFile func() (string, error), candidates func() []string, stdout io.Writer) error {
	status, err := client.RemoteRotate(ctx)
	if errors.Is(err, daemon.ErrNotRunning) {
		path, pathErr := identityFile()
		if pathErr != nil {
			return pathErr
		}
		_, err = remote.RotateIdentity(path)
		status = contract.DaemonRemote{}
	}
	if err != nil {
		return remoteFailure(err)
	}
	_, _ = fmt.Fprintln(stdout, "rotated: existing connect strings no longer work")
	if !status.Listening {
		return nil
	}
	return writeConnect(status, candidates, stdout)
}

// minutesText writes a wait as people say it; other durations stay exact.
func minutesText(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return d.String()
}
