// Package app is servef's transport-agnostic application layer (decision
// 0080): per-domain services whose operations take and return pkg/contract
// types and fail with *Error (a kind and an optional code). It knows no HTTP;
// internal/web adapts it to HTTP, and pkg/client calls it in process.
package app

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/flexdinesh/servef/internal/watch"
)

// Config composes the application. Roots confine every project; ScanDepth
// bounds repository discovery; Depth bounds the file scan within a project.
type Config struct {
	// Roots is required; each operation reads one snapshot, so it may grow
	// while serving.
	Roots     *projects.RootSet
	ScanDepth int
	Depth     int
	// Types decides presentation and is required; every file within Depth is
	// in scope.
	Types *document.Registry
	// LanguageServers answers code navigation; nil serves none. Its owner
	// closes it.
	LanguageServers *lsp.Manager
	// SessionKey names the session whose browser memory a page reads; empty
	// uses the roots joined by newlines.
	SessionKey string
	// OpenFile opens indexed files; nil uses os.Open.
	OpenFile func(name string) (fs.File, error)
	// Watches follows projects for live updates; Events requires it.
	Watches *watch.Hub
	// StatsBudget bounds the bytes one compare-stats read takes; zero or
	// less uses DefaultStatsBudget.
	StatsBudget int64
	// WalkthroughDirectory stores published walkthroughs; empty uses
	// walkthrough.DefaultDirectory.
	WalkthroughDirectory string
}

// App holds the domain services. Each is safe for concurrent use.
type App struct {
	Projects      *Projects
	Documents     *Documents
	Events        *Events
	SourceControl *SourceControl
	Navigation    *Navigation
	Metrics       *Metrics
	Walkthroughs  *Walkthroughs
}

// state is the configuration every service shares.
type state struct {
	config Config
}

// New composes the services over config.
func New(config Config) (*App, error) {
	if config.Roots == nil {
		return nil, errors.New("app: no roots")
	}
	if config.Types == nil {
		return nil, errors.New("app: no document types")
	}
	if config.LanguageServers == nil {
		config.LanguageServers = lsp.NewManager(nil, lsp.Options{})
	}
	if config.OpenFile == nil {
		config.OpenFile = func(name string) (fs.File, error) { return os.Open(name) }
	}
	if config.WalkthroughDirectory == "" {
		directory, err := walkthrough.DefaultDirectory()
		if err != nil {
			return nil, err
		}
		config.WalkthroughDirectory = directory
	}
	shared := &state{config: config}
	targets := walkthroughTargets{roots: config.Roots, depth: config.Depth, types: config.Types, openFile: config.OpenFile}
	return &App{
		Projects:      &Projects{state: shared},
		Documents:     &Documents{state: shared},
		Events:        &Events{state: shared},
		SourceControl: &SourceControl{state: shared},
		Navigation:    &Navigation{state: shared, servers: config.LanguageServers},
		Metrics:       &Metrics{},
		Walkthroughs:  &Walkthroughs{state: shared, service: walkthrough.NewService(walkthrough.NewStore(config.WalkthroughDirectory), targets)},
	}, nil
}

// navigationSupported reports whether a family has a language server
// (page-level navigationSupported).
func (s *state) navigationSupported(family string) bool {
	_, ok := s.config.LanguageServers.ServerFor(family)
	return ok
}

// sessionKey names the session whose browser memory a page reads:
// Config.SessionKey, else the served roots joined by newlines.
func (s *state) sessionKey(roots []string) string {
	if s.config.SessionKey != "" {
		return s.config.SessionKey
	}
	return strings.Join(roots, "\n")
}

func (s *state) scan(root string) (*files.Index, error) {
	return files.Scan(root, s.config.Depth, func(string) bool { return true })
}

func warningStrings(warnings []files.Warning) []string {
	result := make([]string, len(warnings))
	for i, warning := range warnings {
		result[i] = warning.Error()
	}
	return result
}

// mapSlice maps values, keeping nil nil so encodings match the domain's.
func mapSlice[T, U any](values []T, convert func(T) U) []U {
	if values == nil {
		return nil
	}
	result := make([]U, len(values))
	for i, value := range values {
		result[i] = convert(value)
	}
	return result
}
