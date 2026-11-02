package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/spf13/cobra"
)

// daemonTimeout bounds ensure and stop.
const daemonTimeout = 10 * time.Second

type daemonOptions struct {
	socket    string
	depth     int
	scanDepth int
	idleExit  time.Duration
	json      bool
	// rootsFile overrides daemon.DefaultRootsFile() (hidden; dev and test
	// isolation). An absolute path; "" uses the default.
	rootsFile string
}

// daemonAction runs one daemon subcommand with its output streams.
type daemonAction func(ctx context.Context, opts daemonOptions, stdout, stderr io.Writer) error

// daemonActions runs each daemon subcommand; tests replace them.
type daemonActions struct {
	run, ensure, stop, status daemonAction
}

func newDaemonCommand() *cobra.Command {
	return newDaemonCommandWith(daemonActions{run: runDaemon, ensure: ensureDaemon, stop: stopDaemon, status: statusDaemon})
}

func newDaemonCommandWith(actions daemonActions) *cobra.Command {
	var socket string
	command := &cobra.Command{
		Use:           "daemon",
		Short:         "Run the servef daemon (plumbing for the desktop app)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	command.PersistentFlags().StringVar(&socket, "socket", "", "daemon socket path (default: a private per-user directory)")
	subcommand := func(use, short string, action daemonAction, starts, reports bool) *cobra.Command {
		var opts daemonOptions
		sub := &cobra.Command{
			Use:           use,
			Short:         short,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE: func(command *cobra.Command, _ []string) error {
				if err := daemonSupported(); err != nil {
					return err
				}
				if starts {
					if err := validateDaemonOptions(opts); err != nil {
						return err
					}
					if opts.rootsFile != "" {
						// A started daemon runs in /, so forward an absolute path.
						rootsFile, err := filepath.Abs(opts.rootsFile)
						if err != nil {
							return fmt.Errorf("resolve --roots-file: %w", err)
						}
						opts.rootsFile = rootsFile
					}
				}
				opts.socket = socket
				if opts.socket == "" {
					var err error
					if opts.socket, err = daemon.DefaultSocket(); err != nil {
						return err
					}
				}
				return action(command.Context(), opts, command.OutOrStdout(), command.ErrOrStderr())
			},
		}
		if starts {
			sub.Flags().IntVar(&opts.depth, "depth", 5, "maximum directory depth to scan within a project")
			sub.Flags().IntVar(&opts.scanDepth, "scan-depth", 10, "maximum directory depth to search for git repositories")
			sub.Flags().DurationVar(&opts.idleExit, "idle-exit", daemon.DefaultIdleExit, "stop after this long with no requests (0 never stops)")
			sub.Flags().StringVar(&opts.rootsFile, "roots-file", "", "persisted roots file (default: the user config directory)")
			_ = sub.Flags().MarkHidden("roots-file")
		}
		if reports {
			sub.Flags().BoolVar(&opts.json, "json", false, "print status as JSON")
		}
		return sub
	}
	command.AddCommand(
		subcommand("run", "Serve the daemon in the foreground", actions.run, true, false),
		subcommand("ensure", "Start the daemon unless it is running, then print its status", actions.ensure, true, true),
		subcommand("stop", "Stop the running daemon", actions.stop, false, false),
		subcommand("status", "Print the running daemon's status", actions.status, false, true),
	)
	return command
}

func validateDaemonOptions(opts daemonOptions) error {
	if opts.depth < 0 {
		return errors.New("--depth must be zero or greater")
	}
	if opts.scanDepth < 0 {
		return errors.New("--scan-depth must be zero or greater")
	}
	if opts.idleExit < 0 {
		return errors.New("--idle-exit must be zero or greater")
	}
	return nil
}

func runDaemon(ctx context.Context, opts daemonOptions, _, stderr io.Writer) error {
	rootsFile := opts.rootsFile
	if rootsFile == "" {
		var err error
		if rootsFile, err = daemon.DefaultRootsFile(); err != nil {
			return err
		}
	}
	err := daemon.Run(ctx, daemon.Config{Socket: opts.socket, RootsFile: rootsFile, Depth: opts.depth, ScanDepth: opts.scanDepth, IdleExit: opts.idleExit, Log: stderr})
	if errors.Is(err, daemon.ErrRunning) {
		return fmt.Errorf("%w at %s", err, opts.socket)
	}
	return err
}

func ensureDaemon(ctx context.Context, opts daemonOptions, stdout, _ io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	status, started, err := daemon.Ensure(ctx, opts.socket, startDaemon(opts))
	var mismatch *daemon.ProtocolMismatchError
	if err != nil && !errors.As(err, &mismatch) {
		return fmt.Errorf("%w; see %s.log", err, opts.socket)
	}
	if opts.json {
		if writeErr := writeDaemonJSON(stdout, struct {
			daemon.Status
			Started bool `json:"started"`
		}{status, started}); writeErr != nil {
			return writeErr
		}
	} else {
		writeDaemonStatus(stdout, status)
	}
	return err
}

func stopDaemon(ctx context.Context, opts daemonOptions, stdout, _ io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, daemonTimeout)
	defer cancel()
	client := daemon.NewClient(opts.socket)
	if _, err := client.Status(ctx); errors.Is(err, daemon.ErrNotRunning) {
		_, _ = fmt.Fprintln(stdout, "not running")
		return nil
	} else if err != nil {
		return err
	}
	if err := client.Stop(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "stopped")
	return nil
}

func statusDaemon(ctx context.Context, opts daemonOptions, stdout, _ io.Writer) error {
	status, err := daemon.NewClient(opts.socket).Status(ctx)
	if errors.Is(err, daemon.ErrNotRunning) {
		return fmt.Errorf("%w at %s", err, opts.socket)
	}
	if err != nil {
		return err
	}
	if opts.json {
		if err := writeDaemonJSON(stdout, status); err != nil {
			return err
		}
	} else {
		writeDaemonStatus(stdout, status)
	}
	if status.Protocol != daemon.ProtocolVersion {
		return &daemon.ProtocolMismatchError{Daemon: status.Protocol, Client: daemon.ProtocolVersion, Socket: opts.socket, DaemonVersion: status.Version}
	}
	return nil
}

func writeDaemonJSON(stdout io.Writer, value any) error {
	return json.NewEncoder(stdout).Encode(value)
}

func writeDaemonStatus(stdout io.Writer, status daemon.Status) {
	roots := "roots"
	if len(status.Roots) == 1 {
		roots = "root"
	}
	_, _ = fmt.Fprintf(stdout, "servef daemon %s (protocol %d) at %s, pid %d, %d %s\n", status.Version, status.Protocol, status.Socket, status.PID, len(status.Roots), roots)
}

// startDaemon launches `servef daemon run` detached in its own session,
// logging to <socket>.log.
func startDaemon(opts daemonOptions) daemon.StartFunc {
	return func(context.Context) (<-chan error, error) {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate servef: %w", err)
		}
		log, err := daemon.OpenLog(opts.socket)
		if err != nil {
			return nil, fmt.Errorf("open daemon log: %w", err)
		}
		defer log.Close()
		stdin, err := os.Open(os.DevNull)
		if err != nil {
			return nil, err
		}
		defer stdin.Close()
		process := exec.Command(executable, daemonRunArgs(opts)...)
		process.Dir = "/"
		process.Stdin = stdin
		process.Stdout = log
		process.Stderr = log
		process.SysProcAttr = daemonProcAttr()
		if err := process.Start(); err != nil {
			return nil, err
		}
		exited := make(chan error, 1)
		go func() { exited <- process.Wait() }()
		return exited, nil
	}
}

// daemonRunArgs are the arguments ensure starts `servef daemon run` with.
// --roots-file is forwarded only when set.
func daemonRunArgs(opts daemonOptions) []string {
	args := []string{"daemon", "run",
		"--socket", opts.socket,
		"--depth", strconv.Itoa(opts.depth),
		"--scan-depth", strconv.Itoa(opts.scanDepth),
		"--idle-exit", opts.idleExit.String()}
	if opts.rootsFile != "" {
		args = append(args, "--roots-file", opts.rootsFile)
	}
	return args
}
