package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/browser"
	"github.com/flexdinesh/servef/internal/control"
	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/internal/desktopapp"
	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/spf13/cobra"
)

// Publishing without a server (decision 0065): the CLI checks targets with
// the checks POST /api/walkthroughs applies and writes the same store.

const (
	// publishTimeout bounds target checks, as the server's status timeout does.
	publishTimeout = 20 * time.Second
	// viewerTimeout bounds each running server's walkthrough probe.
	viewerTimeout = 2 * time.Second
	// announceTimeout bounds telling the daemon about a publish.
	announceTimeout = time.Second
)

// errNotPublished is returned after a rejected draft's problems are printed.
var errNotPublished = errors.New("walkthrough not published")

type publishOptions struct {
	source string // file path or "-" for stdin
	depth  int
	noOpen bool
	json   bool
	// Test seams, not flags.
	walkthroughDir string             // "" uses walkthrough.DefaultDirectory()
	controlDir     string             // "" uses control.DefaultDirectory()
	socket         string             // "" uses daemon.DefaultSocket()
	openBrowser    func(string) error // nil uses browser.Open
	getenv         func(string) string
	installed      func(context.Context) (bool, error) // nil uses desktopapp.Installed
	openApp        func(context.Context, string) error // nil uses desktopapp.Open
	goos           string                              // "" uses runtime.GOOS
}

type publishRunner func(context.Context, publishOptions, io.Reader, io.Writer, io.Writer) error

func newWalkthroughCommand(publish publishRunner) *cobra.Command {
	command := &cobra.Command{
		Use:           "walkthrough",
		Short:         "Publish walkthroughs without a running server",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Runnable so NoArgs rejects unknown subcommands; bare, it prints help.
		RunE: func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	var opts publishOptions
	publishCommand := &cobra.Command{
		Use:   "publish <file|->",
		Short: "Check and store a walkthrough; - reads standard input",
		Long: "Check a walkthrough's targets and store it where servef reads walkthroughs.\n" +
			"No server is needed: an open browser lists it within seconds.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, args []string) error {
			if opts.depth < 0 {
				return errors.New("--depth must be zero or greater")
			}
			opts.source = args[0]
			return publish(command.Context(), opts, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	flags := publishCommand.Flags()
	flags.IntVar(&opts.depth, "depth", 5, "maximum directory depth of working-tree files within the project")
	flags.BoolVar(&opts.noOpen, "no-open", false, "do not open the desktop app or browser, even when the walkthrough asks to")
	flags.BoolVar(&opts.json, "json", false, "print the result as one JSON object")
	schemaCommand := &cobra.Command{
		Use:           "schema",
		Short:         "Print the walkthrough JSON Schema",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := command.OutOrStdout().Write(walkthrough.Schema)
			return err
		},
	}
	command.AddCommand(publishCommand, schemaCommand)
	return command
}

type publishResult struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Created bool   `json:"created"`
	URL     string `json:"url,omitempty"`
	// DesktopURL is URL's servef:// form when URL is a web URL.
	DesktopURL string `json:"desktopUrl,omitempty"`
}

func publishWalkthrough(ctx context.Context, opts publishOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	draft, err := readDraft(opts.source, stdin)
	if err != nil {
		return err
	}
	directory := opts.walkthroughDir
	if directory == "" {
		directory, err = walkthrough.DefaultDirectory()
		if err != nil {
			return err
		}
	}
	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	service := walkthrough.NewService(walkthrough.NewStore(directory), standaloneTargets{depth: opts.depth, types: document.Builtins()})
	published, err := service.Publish(publishCtx, draft)
	var problems walkthrough.Problems
	if errors.As(err, &problems) {
		_, _ = fmt.Fprintln(stderr, "invalid walkthrough:")
		for _, problem := range problems {
			_, _ = fmt.Fprintf(stderr, "  %s: %s\n", problem.Field, problem.Message)
		}
		return errNotPublished
	}
	if err != nil {
		return fmt.Errorf("publish walkthrough: %w", err)
	}

	stored := published.Walkthrough
	getenv := opts.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	goos := opts.goos
	if goos == "" {
		goos = goruntime.GOOS
	}
	local := !noUI(goos, getenv)
	open := draft.Open && shouldOpenBrowser(opts.noOpen, goos, getenv)

	// The desktop app on this machine shows walkthroughs of projects the
	// daemon serves; without a UI here, only http servers can show them
	// (decision 0076).
	viewerQuery := url.Values{"project": {stored.Project}, "wt": {stored.ID}}
	viewer := ""
	announced := announceToDaemon(ctx, opts.socket, stored.Project, stored.ID, published.Created, stderr)
	if local && announced && desktopAppInstalled(ctx, opts) {
		viewer = "servef://local/view?" + viewerQuery.Encode()
		if open {
			openApp := opts.openApp
			if openApp == nil {
				openApp = desktopapp.Open
			}
			if err := openApp(ctx, viewer); err != nil {
				_, _ = fmt.Fprintf(stderr, "could not open the servef desktop app: %v\n", err)
				viewer = ""
			}
		}
	}
	if viewer == "" {
		controlDir := opts.controlDir
		if controlDir == "" {
			controlDir, err = control.DefaultDirectory()
			if err != nil {
				return err
			}
		}
		var mismatches []string
		viewer, mismatches = findViewer(ctx, control.NewRegistry(controlDir), stored.Project, stored.ID)
		for _, origin := range mismatches {
			_, _ = fmt.Fprintf(stderr, "servef at %s serves this project but keeps walkthroughs elsewhere; POST to %s/api/walkthroughs\n", origin, origin)
		}
		if open && viewer != "" {
			openBrowser := opts.openBrowser
			if openBrowser == nil {
				openBrowser = browser.Open
			}
			if err := openBrowser(viewer); err != nil {
				_, _ = fmt.Fprintf(stderr, "could not open browser: %v\n", err)
			}
		}
	}

	desktop := ""
	if strings.HasPrefix(viewer, "http://") {
		desktop = desktopURL(viewer)
	}
	if opts.json {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(publishResult{ID: stored.ID, Project: stored.Project, Created: published.Created, URL: viewer, DesktopURL: desktop})
	}
	outcome := "replaced"
	if published.Created {
		outcome = "created"
	}
	_, _ = fmt.Fprintf(stdout, "published %s (%s) for %s\n", stored.ID, outcome, stored.Project)
	tty := linkTerminal(stdout, getenv)
	switch {
	case desktop != "":
		_, _ = fmt.Fprintf(stdout, "web: %s\ndesktop: %s\n", hyperlink(viewer, tty), hyperlink(desktop, tty))
	case viewer != "":
		_, _ = fmt.Fprintf(stdout, "desktop: %s\n", hyperlink(viewer, tty))
	case draft.Open:
		_, _ = fmt.Fprintf(stdout, "no running servef shows this project, so nothing opened; run: servef %s, then open Walkthroughs\n", stored.Project)
	default:
		_, _ = fmt.Fprintf(stdout, "no running servef shows this project; run: servef %s, then open Walkthroughs\n", stored.Project)
	}
	return nil
}

// announceToDaemon tells a running daemon about the publish so its control
// streams announce it, and reports whether it did. A daemon that is not
// running, unreachable, or not serving the project is ignored; one that
// serves the project but reads another store gets a warning.
func announceToDaemon(ctx context.Context, socket, project, id string, created bool, stderr io.Writer) bool {
	if daemonSupported() != nil {
		return false
	}
	if socket == "" {
		var err error
		if socket, err = daemon.DefaultSocket(); err != nil {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(ctx, announceTimeout)
	defer cancel()
	err := daemon.NewClient(socket).AnnounceWalkthrough(ctx, project, id, created)
	var failure *daemon.ControlError
	if errors.As(err, &failure) && failure.Status == http.StatusNotFound && failure.Code == "walkthroughUnavailable" {
		_, _ = fmt.Fprintf(stderr, "servef daemon at %s serves this project but keeps walkthroughs elsewhere\n", socket)
	}
	return err == nil
}

// desktopAppInstalled reports whether the desktop app handles servef:// URLs;
// a failed check counts as not installed.
func desktopAppInstalled(ctx context.Context, opts publishOptions) bool {
	installed := opts.installed
	if installed == nil {
		installed = desktopapp.Installed
	}
	ok, err := installed(ctx)
	return err == nil && ok
}

// readDraft reads one strict draft from a file, or standard input for -,
// within the size POST /api/walkthroughs accepts.
func readDraft(source string, stdin io.Reader) (walkthrough.Draft, error) {
	reader := stdin
	if source != "-" {
		file, err := os.Open(source)
		if err != nil {
			return walkthrough.Draft{}, err
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	contents, err := io.ReadAll(io.LimitReader(reader, walkthrough.MaxRequestBytes+1))
	if err != nil {
		return walkthrough.Draft{}, fmt.Errorf("read walkthrough: %w", err)
	}
	if len(contents) > walkthrough.MaxRequestBytes {
		return walkthrough.Draft{}, fmt.Errorf("a walkthrough is at most %d bytes", walkthrough.MaxRequestBytes)
	}
	return walkthrough.Decode(bytes.NewReader(contents))
}

// standaloneTargets resolves the draft's project as its own root, then runs
// the application layer's shared checks.
type standaloneTargets struct {
	depth int
	types *document.Registry
}

func (t standaloneTargets) Check(ctx context.Context, project string, steps []walkthrough.Step) (string, []walkthrough.Step, error) {
	root, err := projects.ResolveStandalone(project)
	if errors.Is(err, projects.ErrInsideRepository) {
		return "", nil, walkthrough.Problems{{Field: "project", Message: "is a folder inside a repository; use the repository root (git rev-parse --show-toplevel)"}}
	}
	if err != nil {
		return "", nil, walkthrough.Problems{{Field: "project", Message: "must be an existing absolute directory"}}
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		return "", nil, err
	}
	return app.NewWalkthroughTargets(roots, t.depth, t.types).Check(ctx, root, steps)
}

// findViewer asks each running registered server whether it shows the
// walkthrough. It returns the first one's view URL, and the origins of
// servers that serve the project but read another walkthrough store.
func findViewer(ctx context.Context, registry control.Registry, project, id string) (string, []string) {
	remotes, err := registry.List()
	if err != nil {
		return "", nil
	}
	client := &http.Client{
		Timeout: viewerTimeout,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: viewerTimeout}).DialContext,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	var mismatches []string
	for _, remote := range remotes {
		if !remote.Running(ctx) {
			continue
		}
		origin := remote.Origin()
		switch probeWalkthrough(ctx, client, origin, project, id) {
		case http.StatusOK:
			query := url.Values{"project": {project}, "wt": {id}}
			return origin + "/view?" + query.Encode(), mismatches
		case http.StatusNotFound:
			mismatches = append(mismatches, origin)
		}
	}
	return "", mismatches
}

// probeWalkthrough returns 200 when the server at origin shows the
// walkthrough, 404 when it serves the project without it, and 0 otherwise.
func probeWalkthrough(ctx context.Context, client *http.Client, origin, project, id string) int {
	query := url.Values{"project": {project}, "id": {id}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/walkthrough?"+query.Encode(), nil)
	if err != nil {
		return 0
	}
	response, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusOK {
		return http.StatusOK
	}
	var body struct {
		Code string `json:"code"`
	}
	if response.StatusCode == http.StatusNotFound && json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body) == nil && body.Code == "walkthroughUnavailable" {
		return http.StatusNotFound
	}
	return 0
}
