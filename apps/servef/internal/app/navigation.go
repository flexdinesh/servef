package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Navigation answers code navigation from language servers only (decision
// 0018): status, definitions, and references. It validates indexed project
// access and maps server locations to project paths; internal/lsp owns the
// servers. Status never starts a server.
type Navigation struct {
	*state
	servers *lsp.Manager
}

// navigationSource shares one request's CLI-scoped scan. Every read and
// returned location passes indexed-path resolution and eligibility checks.
type navigationSource struct {
	nav   *Navigation
	root  string
	once  sync.Once
	index *files.Index
	err   error
}

func (n *Navigation) source(root string) *navigationSource {
	return &navigationSource{nav: n, root: root}
}

func (s *navigationSource) scan() (*files.Index, error) {
	s.once.Do(func() { s.index, s.err = s.nav.scan(s.root) })
	return s.index, s.err
}

// Passive status and capped results inspect only metadata and the binary prefix.
func (s *navigationSource) readFile(name string, source bool) ([]byte, string) {
	if source {
		text, _, reason := s.snapshot(name)
		return text, reason
	}
	index, err := s.scan()
	if err != nil {
		return nil, "Could not scan the project."
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		return nil, "That file is unavailable."
	}
	if index.Ignored(name) {
		return nil, "Gitignored files do not support code navigation."
	}
	family := s.nav.config.Types.Lookup(name).Type()
	file, err := s.nav.config.OpenFile(resolved)
	if err != nil {
		return nil, "That file is unavailable."
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, "That file is unavailable."
	}
	if info.Size() > document.PreviewLimit {
		return nil, "Files over 4 MiB do not support code navigation."
	}
	if !family.HasTextSource() {
		return nil, "Binary files do not support code navigation."
	}
	_, binary, err := document.ReadText(file, document.SniffLength)
	if err != nil {
		return nil, "That file is unavailable."
	}
	if binary {
		return nil, "Binary files do not support code navigation."
	}
	return nil, ""
}

// snapshot pairs text with the descriptor's change stamp. A file changed or
// replaced while being read is retried, so its overlay never receives newer
// metadata alongside older bytes.
func (s *navigationSource) snapshot(name string) ([]byte, lsp.File, string) {
	index, err := s.scan()
	if err != nil {
		return nil, lsp.File{}, "Could not scan the project."
	}
	if index.Ignored(name) {
		return nil, lsp.File{}, "Gitignored files do not support code navigation."
	}
	for attempt := 0; attempt < 2; attempt++ {
		resolved, err := index.Resolve(name)
		if err != nil {
			return nil, lsp.File{}, "That file is unavailable."
		}
		text, stamp, reason, changed := s.readSnapshot(resolved, s.nav.config.Types.Lookup(name).Type())
		if !changed {
			return text, stamp, reason
		}
	}
	return nil, lsp.File{}, "That file changed while being read. Try again."
}

func (s *navigationSource) readSnapshot(resolved string, family document.Type) ([]byte, lsp.File, string, bool) {
	file, err := s.nav.config.OpenFile(resolved)
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	if before.Size() > document.PreviewLimit {
		return nil, lsp.File{}, "Files over 4 MiB do not support code navigation.", false
	}
	if !family.HasTextSource() {
		return nil, lsp.File{}, "Binary files do not support code navigation.", false
	}
	text, err := io.ReadAll(io.LimitReader(file, document.PreviewLimit+1))
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	after, err := file.Stat()
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	current, err := os.Stat(resolved)
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	if !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || after.Size() != current.Size() || !after.ModTime().Equal(current.ModTime()) || int64(len(text)) != after.Size() {
		return nil, lsp.File{}, "", true
	}
	if document.IsBinary(text[:min(len(text), document.SniffLength)]) {
		return nil, lsp.File{}, "Binary files do not support code navigation.", false
	}
	return text, lsp.File{Path: resolved, Size: after.Size(), Modified: after.ModTime()}, "", false
}

const (
	serverStartWait          = 20 * time.Second
	serverRequestTimeout     = 10 * time.Second
	navigationRequestTimeout = 25 * time.Second
	maxServerDefinitions     = 50
	maxServerReferences      = 1000
)

// navigationFailure is a navigation error; every one carries a code.
func navigationFailure(kind contract.ErrorKind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// projectFailure is a navigation request's unresolved project, with its
// kind and message and the unavailable code.
func projectFailure(err error) *Error {
	failure := AsError(err)
	return navigationFailure(failure.Kind, contract.CodeNavigationUnavailable, failure.Message)
}

// Definition returns the language server's definitions at a position.
func (n *Navigation) Definition(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return n.navigate(ctx, request, false)
}

// References returns the language server's references at a position.
func (n *Navigation) References(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return n.navigate(ctx, request, true)
}

// navigate answers only from LSP. Empty success means the server returned
// no eligible results, never a hidden startup or query failure.
func (n *Navigation) navigate(ctx context.Context, navigation contract.NavigationRequest, references bool) (contract.NavigationResult, error) {
	project, err := ResolveProject(n.config.Roots.List(), navigation.Project)
	if err != nil {
		return contract.NavigationResult{}, projectFailure(err)
	}
	name, line, column := navigation.Path, navigation.Line, navigation.Column
	if line < 1 || column < 0 || name == "" {
		return contract.NavigationResult{}, navigationFailure(contract.KindInvalid, contract.CodeNavigationInvalid, "A file, line, and column are required.")
	}
	source := n.source(project.Root)
	_, reason := source.readFile(name, false)
	if reason != "" {
		return contract.NavigationResult{}, navigationFailure(contract.KindNotFound, contract.CodeNavigationUnavailable, reason)
	}
	family := n.config.Types.Lookup(name).Type().ID
	server, supported := n.servers.ServerFor(family)
	if !supported {
		return contract.NavigationResult{}, navigationFailure(contract.KindInvalid, contract.CodeNavigationUnsupported, "No language server is configured for this language.")
	}
	if !navigation.LanguageServers {
		return contract.NavigationResult{}, navigationFailure(contract.KindInvalid, contract.CodeNavigationDisabled, "Language servers are disabled in Settings.")
	}

	ctx, cancel := context.WithTimeout(ctx, navigationRequestTimeout)
	defer cancel()
	starting, cancelStart := context.WithTimeout(ctx, serverStartWait)
	err = n.servers.Start(starting, project.Root, family)
	cancelStart()
	if err != nil {
		return contract.NavigationResult{}, serverFailure(err, true)
	}
	// Startup may take seconds. Re-scan and re-read after readiness, including
	// changes to gitignore, deletion, replacement, and the document itself.
	source = n.source(project.Root)
	text, documentFile, reason := source.snapshot(name)
	if reason != "" {
		return contract.NavigationResult{}, navigationFailure(contract.KindNotFound, contract.CodeNavigationUnavailable, reason)
	}
	request := lsp.Request{Root: project.Root, Family: family, Text: text, Document: documentFile, At: lsp.Position{Line: line, Column: column}}
	index, err := source.scan()
	if err != nil {
		return contract.NavigationResult{}, navigationFailure(contract.KindInternal, contract.CodeNavigationUnavailable, "Could not scan the project.")
	}
	for _, nameOnDisk := range index.Files {
		if index.Ignored(nameOnDisk) {
			continue
		}
		if _, served := server.Families[n.config.Types.Lookup(nameOnDisk).Type().ID]; !served {
			continue
		}
		resolved, err := index.Resolve(nameOnDisk)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || info.Size() > document.PreviewLimit {
			continue
		}
		file := lsp.File{Path: resolved, Size: info.Size(), Modified: info.ModTime()}
		if nameOnDisk == name {
			file = documentFile
		}
		request.Files = append(request.Files, file)
	}
	if len(server.WorkspaceFiles) > 0 {
		contextFilter, err := files.NewNameFilter(server.WorkspaceFiles)
		if err != nil {
			return contract.NavigationResult{}, navigationFailure(contract.KindInternal, contract.CodeNavigationUnavailable, "Invalid language server workspace configuration.")
		}
		contextFiles, err := files.Scan(project.Root, n.config.Depth, contextFilter.Match)
		if err != nil {
			return contract.NavigationResult{}, navigationFailure(contract.KindInternal, contract.CodeNavigationUnavailable, "Could not scan workspace configuration.")
		}
		seen := map[string]bool{}
		for _, file := range request.Files {
			seen[file.Path] = true
		}
		for _, name := range contextFiles.Files {
			if contextFiles.Ignored(name) {
				continue
			}
			resolved, err := contextFiles.Resolve(name)
			if err != nil || seen[resolved] {
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			request.Files = append(request.Files, lsp.File{Path: resolved, Size: info.Size(), Modified: info.ModTime()})
			seen[resolved] = true
		}
	}
	if request.Document.Path == "" {
		return contract.NavigationResult{}, navigationFailure(contract.KindNotFound, contract.CodeNavigationUnavailable, "That file is unavailable.")
	}
	querying, cancelQuery := context.WithTimeout(ctx, serverRequestTimeout)
	defer cancelQuery()
	var locations []lsp.Location
	kind, limit := "definition", maxServerDefinitions
	if references {
		locations, err = n.servers.References(querying, request)
		kind, limit = "reference", maxServerReferences
	} else {
		locations, err = n.servers.Definition(querying, request)
	}
	if err != nil {
		return contract.NavigationResult{}, serverFailure(err, false)
	}
	response := source.locations(locations, kind, limit)
	response.Symbol, response.Server = navigationWord(text, request.At), server.Name
	if current, ok := n.servers.Status(project.Root, family); ok {
		response.Server = current.Server.Name
	}
	return response, nil
}

// serverFailure explains a language server's start or query failure.
func serverFailure(err error, starting bool) *Error {
	switch {
	case errors.Is(err, lsp.ErrMissing):
		return navigationFailure(contract.KindUnavailable, contract.CodeNavigationMissing, "The language server is not installed. Open language status for installation instructions.")
	case errors.Is(err, lsp.ErrNotReady) && starting:
		return navigationFailure(contract.KindUnavailable, contract.CodeNavigationStarting, "The language server is still starting. Try again shortly.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return navigationFailure(contract.KindUnavailable, contract.CodeNavigationTimeout, "The language server request timed out. Try again.")
	case errors.Is(err, lsp.ErrUnsupported):
		return navigationFailure(contract.KindInvalid, contract.CodeNavigationUnsupportedMethod, "The language server does not support this navigation action.")
	default:
		return navigationFailure(contract.KindUnavailable, contract.CodeNavigationFailed, "Language server failed: "+err.Error())
	}
}

// relative rejects outside, ignored, out-of-scope and escaping symlink paths.
func (s *navigationSource) relative(absolute string) (string, bool) {
	if !filepath.IsAbs(absolute) {
		return "", false
	}
	index, err := s.scan()
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(s.root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if _, err := index.Resolve(relative); err != nil || index.Ignored(relative) {
		return "", false
	}
	return relative, true
}

func (s *navigationSource) locations(found []lsp.Location, kind string, limit int) contract.NavigationResult {
	response := contract.NavigationResult{Locations: []contract.NavigationLocation{}}
	seen := map[lsp.Location]bool{}
	type target struct {
		text   []byte
		reason string
	}
	// At most one target's source stays resident. Result counts are bounded,
	// and multiple hits in a target avoid repeated filesystem reads.
	lastName := ""
	last := target{}
	for _, location := range found {
		relative, ok := s.relative(location.Path)
		if !ok {
			response.Filtered++
			continue
		}
		if relative != lastName {
			last.text, last.reason = s.readFile(relative, len(response.Locations) < limit)
			lastName = relative
		}
		if last.reason != "" || location.Line < 1 || location.Column < 0 {
			response.Filtered++
			continue
		}
		location.Path = relative
		if seen[location] {
			continue
		}
		seen[location] = true
		if len(response.Locations) == limit {
			response.Truncated = true
			continue
		}
		response.Locations = append(response.Locations, contract.NavigationLocation{Path: relative, Line: location.Line, Column: location.Column, EndLine: location.End.Line, EndColumn: location.End.Column, Kind: kind, Snippet: navigationSnippet(last.text, location.Line)})
	}
	if kind == "reference" {
		sort.SliceStable(response.Locations, func(i, j int) bool {
			left, right := response.Locations[i], response.Locations[j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Column < right.Column
		})
	}
	return response
}

func navigationSnippet(text []byte, line int) string {
	for line > 1 {
		index := bytes.IndexByte(text, '\n')
		if index < 0 {
			return ""
		}
		text, line = text[index+1:], line-1
	}
	if end := bytes.IndexByte(text, '\n'); end >= 0 {
		text = text[:end]
	}
	// Snippets are display hints. Keep unusually long generated lines bounded.
	text = bytes.TrimSpace(text)
	end := 0
	for count := 0; count < 240 && end < len(text); count++ {
		_, size := utf8.DecodeRune(text[end:])
		end += size
	}
	return string(text[:end])
}

// navigationWord supplies a display label only. It never resolves or ranks symbols.
func navigationWord(text []byte, at lsp.Position) string {
	for line := 1; line < at.Line; line++ {
		index := bytes.IndexByte(text, '\n')
		if index < 0 {
			return ""
		}
		text = text[index+1:]
	}
	if end := bytes.IndexByte(text, '\n'); end >= 0 {
		text = text[:end]
	}
	runes := []rune(string(text))
	index, units := 0, 0
	for index < len(runes) && units < at.Column {
		units += utf16.RuneLen(runes[index])
		index++
	}
	if units != at.Column {
		return ""
	}
	name := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' }
	start, end := index, index
	for start > 0 && name(runes[start-1]) {
		start--
	}
	for end < len(runes) && name(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

// Status reports whether a file can navigate and its language server's
// state. Refresh rediscovers binaries; neither starts a process.
func (n *Navigation) Status(request contract.NavigationStatusRequest) (contract.NavigationStatus, error) {
	project, err := ResolveProject(n.config.Roots.List(), request.Project)
	if err != nil {
		return contract.NavigationStatus{}, projectFailure(err)
	}
	if request.Path == "" {
		return contract.NavigationStatus{}, navigationFailure(contract.KindInvalid, contract.CodeNavigationInvalid, "A file is required.")
	}
	if request.Refresh {
		n.servers.Refresh()
	}
	_, reason := n.source(project.Root).readFile(request.Path, false)
	response := contract.NavigationStatus{Enabled: request.LanguageServers, Eligible: reason == "", Reason: reason}
	family := n.config.Types.Lookup(request.Path)
	if current, ok := n.servers.Status(project.Root, family.Type().ID); ok {
		response.Supported = true
		response.Server = &contract.LanguageServerStatus{
			Name:         current.Server.Name,
			Language:     current.Server.Language,
			State:        contract.LanguageServerState(current.State),
			Message:      current.Message,
			Install:      mapSlice(current.Server.Install, installContract),
			Docs:         current.Server.Docs,
			Memory:       current.Memory,
			IdleMinutes:  int(n.servers.Idle() / time.Minute),
			Capabilities: capabilitiesContract(current.Capabilities),
		}
	} else if response.Reason == "" {
		response.Reason = "No language server is configured for this language."
	}
	return response, nil
}

func installContract(install lsp.Install) contract.LanguageServerInstall {
	return contract.LanguageServerInstall{Label: install.Label, Command: install.Command, Prerequisites: install.Prerequisites, Docs: install.Docs}
}

func capabilitiesContract(capabilities *lsp.Capabilities) *contract.LanguageServerCapabilities {
	if capabilities == nil {
		return nil
	}
	return &contract.LanguageServerCapabilities{Definition: capabilities.Definition, References: capabilities.References}
}
