// Package cmd defines the servef command-line interface.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flexdinesh/servef/internal/browser"
	"github.com/flexdinesh/servef/internal/control"
	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/features"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/version"
	webui "github.com/flexdinesh/servef/internal/web"
	"github.com/spf13/cobra"
)

var defaultExclusions = []string{".git", "node_modules", "vendor"}

const defaultHost = "localhost"

type options struct {
	paths      []string
	depth      int
	scanDepth  int
	host       string
	port       int
	portSet    bool
	noOpen     bool
	exclusions []string
	features   features.Set
	filenames  files.NameFilter
	controlDir string
}

type runner func(context.Context, options, io.Reader, io.Writer, io.Writer) error

// defaultMemoryLimit is the soft heap limit when GOMEMLIMIT is unset. Code
// navigation parsing churns garbage; the limit makes the collector reclaim
// it sooner instead of letting the heap double, at a small CPU cost.
const defaultMemoryLimit = 1 << 30

// Execute runs the root command until the server exits or receives a signal.
func Execute() error {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(defaultMemoryLimit)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return newRootCommand(run).ExecuteContext(ctx)
}

func newRootCommand(runCommand runner) *cobra.Command {
	var opts options
	var featureNames []string
	var filenames string
	command := &cobra.Command{
		Use:           "servef [path]",
		Short:         "Browse local files in a web browser",
		Version:       version.String(),
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, args []string) error {
			// A positional path is shorthand for one --path.
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
			if opts.port < 0 || opts.port > 65535 {
				return errors.New("--port must be between 0 and 65535")
			}
			if opts.host != defaultHost && net.ParseIP(opts.host) == nil {
				return errors.New("--host must be a valid IP address")
			}
			opts.portSet = command.Flags().Changed("port")
			parsedFeatures, err := features.Parse(featureNames)
			if err != nil {
				return err
			}
			opts.features = parsedFeatures
			opts.filenames, err = parseFilenames(filenames)
			if err != nil {
				return err
			}
			return runCommand(command.Context(), opts, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	command.SetVersionTemplate("{{.Version}}\n")

	command.Flags().StringArrayVar(&opts.paths, "path", nil, "add a directory to serve (repeatable); a positional path is shorthand for one")
	command.Flags().StringVar(&filenames, "filename", "*", "comma-separated, case-insensitive base-name globs to include (e.g. '*.md,*.go,Dockerfile')")
	command.Flags().IntVar(&opts.depth, "depth", 5, "maximum directory depth to scan within a project")
	command.Flags().IntVar(&opts.scanDepth, "scan-depth", 10, "maximum directory depth to search for git repositories")
	command.Flags().StringVar(&opts.host, "host", defaultHost, "IP address to bind")
	command.Flags().IntVar(&opts.port, "port", 0, "port; defaults to the first free port from 7971 to 7980")
	command.Flags().BoolVar(&opts.noOpen, "no-open", false, "do not open the browser automatically")
	command.Flags().StringArrayVar(&opts.exclusions, "exclude", nil, "add a directory name to exclude (repeatable)")
	command.Flags().StringArrayVar(&featureNames, "feature", nil, "enable an experimental feature (repeatable)")
	return command
}

func run(ctx context.Context, opts options, stdin io.Reader, stdout, stderr io.Writer) error {
	roots, err := projects.CanonicalRoots(opts.paths)
	if err != nil {
		return err
	}
	exclusions := append(append([]string{}, defaultExclusions...), opts.exclusions...)

	// Validate the root before binding a port or opening a browser.
	discoveryStarted := time.Now()
	filenames := opts.filenames
	if len(filenames.Patterns()) == 0 {
		filenames = files.AllNames()
	}
	discovery, err := projects.Discover(roots, opts.scanDepth, exclusions)
	discoveryDuration := time.Since(discoveryStarted)
	if err != nil {
		return err
	}

	app, err := webui.New(webui.Config{Roots: roots, ScanDepth: opts.scanDepth, Depth: opts.depth, Exclusions: exclusions, Features: opts.features, Types: document.Builtins(), Filenames: filenames})
	if err != nil {
		return fmt.Errorf("create web interface: %w", err)
	}
	controlDir := opts.controlDir
	if controlDir == "" {
		controlDir, err = control.DefaultDirectory()
		if err != nil {
			return err
		}
	}
	registry := control.NewRegistry(controlDir)
	host := opts.host
	if host == "" {
		host = defaultHost
	}
	listener, err := openListener(host, opts, stdout, listen)
	if errors.Is(err, errDefaultPortsBusy) {
		servers, lookupErr := runningDefaultServers(ctx, registry, host)
		if lookupErr != nil {
			return lookupErr
		}
		if !confirmStopServers(stdin, stdout, len(servers)) {
			return nil
		}
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := stopServers(stopCtx, servers); err != nil {
			return err
		}
		listener, err = waitForListener(stopCtx, host, defaultPortStart, listen)
	}
	if err != nil {
		if opts.portSet {
			return fmt.Errorf("listen on %s port %d: %w", host, opts.port, err)
		}
		return fmt.Errorf("listen on %s: %w", host, err)
	}

	port, err := listenerPort(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return err
	}
	instance, err := registry.Register(host, port)
	if err != nil {
		_ = listener.Close()
		return err
	}
	defer func() { _ = instance.Close() }()
	server := &http.Server{
		Handler:           instance.Handler(app),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()

	url := serverURL(host, port)
	writeStartup(stdout, startupInfo{
		Version:      "servef " + version.Number(),
		Directories:  roots,
		URL:          url,
		ProjectCount: len(discovery.Projects),
		Discovery:    discoveryDuration,
	})
	if shouldOpenBrowser(opts.noOpen, os.Getenv) {
		go func() {
			if err := browser.Open(url); err != nil {
				_, _ = fmt.Fprintf(stderr, "  could not open browser: %v\n", err)
			}
		}()
	}

	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve files: %w", err)
		}
		return nil
	case <-ctx.Done():
	case <-instance.StopRequested():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("stop local server: %w", err)
	}
	return nil
}

// parseFilenames reads one comma-separated list of base-name globs.
func parseFilenames(value string) (files.NameFilter, error) {
	patterns := strings.Split(value, ",")
	for i, pattern := range patterns {
		patterns[i] = strings.TrimSpace(pattern)
		if patterns[i] == "" {
			return files.NameFilter{}, fmt.Errorf("--filename has an empty pattern in %q", value)
		}
	}
	filter, err := files.NewNameFilter(patterns)
	if err != nil {
		return files.NameFilter{}, fmt.Errorf("--filename: %w", err)
	}
	return filter, nil
}

func listen(host string, port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}

func listenerPort(address net.Addr) (int, error) {
	_, rawPort, err := net.SplitHostPort(address.String())
	if err != nil {
		return 0, fmt.Errorf("read local server address %q: %w", address.String(), err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return 0, fmt.Errorf("read local server port %q: %w", rawPort, err)
	}
	return port, nil
}

func serverURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
}

func shouldOpenBrowser(noOpen bool, getenv func(string) string) bool {
	if noOpen {
		return false
	}
	for _, name := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(name) != "" {
			return false
		}
	}
	return true
}
