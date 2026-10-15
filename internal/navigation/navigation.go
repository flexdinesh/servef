// Package navigation answers go-to-definition and find-references for a
// project. It keeps an in-memory index of definitions per project, built in
// the background and refreshed incrementally by file size and modification
// time. References are found on demand. Document handlers own every
// language rule; this package only stores, filters, and ranks.
package navigation

import (
	"bytes"
	"context"
	"errors"
	"path"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/flexdinesh/servef/internal/document"
)

// ErrNotNavigable is returned for files whose family has no Navigator.
var ErrNotNavigable = errors.New("file type does not support code navigation")

// ErrUnavailable is returned when the requested file cannot be read as text.
var ErrUnavailable = errors.New("file is unavailable")

const (
	// refreshInterval throttles rescans; edits show up within it.
	refreshInterval = time.Second
	// maxDefinitions and maxReferences bound responses.
	maxDefinitions = 50
	maxReferences  = 1000
	maxSnippet     = 200
	// maxWorkers bounds concurrent parses; each parse holds its tree and
	// parser arena, so CPU-wide parallelism multiplies peak memory.
	maxWorkers = 4
	// releaseAfter returns memory to the OS after a build parsing this
	// many files, since parse garbage dwarfs the index it leaves.
	releaseAfter = 256
)

// File is an in-scope file and its change stamp.
type File struct {
	Path     string
	Size     int64
	Modified time.Time
}

// Source supplies one project's files without exposing the filesystem. Files
// lists only files navigation may read: in CLI scope and not gitignored.
// Read returns false for files that are binary, too large, or gone.
type Source interface {
	Files() ([]File, error)
	Read(path string) ([]byte, bool, error)
}

// Location is a navigation target.
type Location struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Kind    string `json:"kind"`
	Snippet string `json:"snippet"`
}

// Result answers a navigation request. Symbol is empty when no identifier is
// at the position; Indexing reports that the first index build is still
// running, so definitions may be missing.
type Result struct {
	Symbol    string     `json:"symbol"`
	Indexing  bool       `json:"indexing"`
	Locations []Location `json:"locations"`
}

// Index holds one definition index per project, keyed by project root.
type Index struct {
	registry *document.Registry
	interval time.Duration
	mu       sync.Mutex
	projects map[string]*project
}

// New returns an empty index over the registry's navigable families.
func New(registry *document.Registry) *Index {
	return &Index{registry: registry, interval: refreshInterval, projects: make(map[string]*project)}
}

type entry struct {
	path    string
	symbol  document.Symbol
	snippet string
}

type indexedFile struct {
	size     int64
	modified time.Time
	group    string
	entries  []entry
}

type project struct {
	refresh  sync.Mutex // serializes rebuilds
	mu       sync.RWMutex
	files    map[string]indexedFile
	byName   map[string]map[string][]entry // group, name
	ready    bool
	building bool
	checked  time.Time
}

func (x *Index) project(key string) *project {
	x.mu.Lock()
	defer x.mu.Unlock()
	p, ok := x.projects[key]
	if !ok {
		p = &project{files: make(map[string]indexedFile), byName: make(map[string]map[string][]entry)}
		x.projects[key] = p
	}
	return p
}

// Warm starts building a project's index in the background unless it is
// built or building.
func (x *Index) Warm(key string, source Source) {
	p := x.project(key)
	p.mu.Lock()
	start := !p.ready && !p.building
	if start {
		p.building = true
	}
	p.mu.Unlock()
	if start {
		go func() { _ = x.update(p, source) }()
	}
}

// Ready reports whether a project's first index build has finished.
func (x *Index) Ready(key string) bool {
	p := x.project(key)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ready
}

// fresh refreshes a built index when it may be stale. Before the first build
// finishes it starts one and reports false.
func (x *Index) fresh(key string, source Source) bool {
	p := x.project(key)
	p.mu.RLock()
	ready, stale := p.ready, time.Since(p.checked) >= x.interval
	p.mu.RUnlock()
	if !ready {
		x.Warm(key, source)
		return false
	}
	if stale {
		_ = x.update(p, source)
	}
	return true
}

// update rescans the source and reparses files whose stamp changed.
func (x *Index) update(p *project, source Source) error {
	p.refresh.Lock()
	defer p.refresh.Unlock()
	defer func() {
		p.mu.Lock()
		p.building = false
		p.mu.Unlock()
	}()
	listed, err := source.Files()
	if err != nil {
		return err
	}
	p.mu.RLock()
	previous := p.files
	p.mu.RUnlock()

	next := make(map[string]indexedFile, len(listed))
	var changed []File
	for _, file := range listed {
		navigator, ok := x.registry.Lookup(file.Path).Navigator()
		if !ok {
			continue
		}
		if old, ok := previous[file.Path]; ok && old.size == file.Size && old.modified.Equal(file.Modified) {
			next[file.Path] = old
			continue
		}
		next[file.Path] = indexedFile{size: file.Size, modified: file.Modified, group: navigator.NavigationGroup()}
		changed = append(changed, file)
	}
	parsed := parallel(changed, func(file File) indexedFile {
		indexed := next[file.Path]
		source, ok, err := source.Read(file.Path)
		if err != nil || !ok {
			return indexed
		}
		navigator, _ := x.registry.Lookup(file.Path).Navigator()
		symbols, _ := navigator.Symbols(source)
		lines := splitLines(source)
		indexed.entries = make([]entry, 0, len(symbols))
		for _, symbol := range symbols {
			indexed.entries = append(indexed.entries, entry{path: file.Path, symbol: symbol, snippet: snippet(lines, symbol.Position.Line)})
		}
		return indexed
	})
	for index, file := range changed {
		next[file.Path] = parsed[index]
	}
	byName := make(map[string]map[string][]entry)
	for _, file := range next {
		names := byName[file.group]
		if names == nil {
			names = make(map[string][]entry)
			byName[file.group] = names
		}
		for _, e := range file.entries {
			names[e.symbol.Name] = append(names[e.symbol.Name], e)
		}
	}
	p.mu.Lock()
	p.files, p.byName, p.ready, p.checked = next, byName, true, time.Now()
	p.mu.Unlock()
	if len(changed) >= releaseAfter {
		debug.FreeOSMemory()
	}
	return nil
}

// parallel maps items across bounded workers, preserving order.
func parallel[T, R any](items []T, work func(T) R) []R {
	results := make([]R, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range max(1, min(runtime.GOMAXPROCS(0), maxWorkers, len(items))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = work(items[index])
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func (x *Index) lookup(key, group, name string) []entry {
	p := x.project(key)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]entry{}, p.byName[group][name]...)
}

// bind reads the file at name and resolves the identifier at a position.
func (x *Index) bind(source Source, name string, at document.Position) (document.Navigator, []byte, document.Binding, bool, error) {
	navigator, ok := x.registry.Lookup(name).Navigator()
	if !ok {
		return nil, nil, document.Binding{}, false, ErrNotNavigable
	}
	text, ok, err := source.Read(name)
	if err != nil {
		return nil, nil, document.Binding{}, false, err
	}
	if !ok {
		return nil, nil, document.Binding{}, false, ErrUnavailable
	}
	binding, found, err := navigator.Bind(text, at)
	return navigator, text, binding, found, err
}

// Definition finds where the identifier at a position in a file is defined.
// A definition site resolves to itself, so the caller can show its
// references instead, as editors do.
func (x *Index) Definition(key string, source Source, name string, at document.Position) (Result, error) {
	navigator, text, binding, found, err := x.bind(source, name, at)
	if err != nil || !found {
		return Result{Locations: []Location{}}, err
	}
	result := Result{Symbol: binding.Name, Locations: []Location{}}
	lines := splitLines(text)
	here := func(position document.Position, kind string) Result {
		result.Locations = []Location{{Path: name, Line: position.Line, Column: position.Column, Kind: kind, Snippet: snippet(lines, position.Line)}}
		return result
	}
	switch {
	case binding.Local != nil:
		return here(binding.Local.Definition, binding.Local.Kind), nil
	case binding.Definition:
		return here(binding.Position, "definition"), nil
	}
	result.Indexing = !x.fresh(key, source)
	targets := x.targets(key, navigator, name, binding)
	if len(targets) == 0 && binding.Import != nil && !result.Indexing {
		// The module is outside the project, such as a standard library.
		return here(binding.Import.Statement, "import"), nil
	}
	result.Locations = locations(targets)
	return result, nil
}

// targets resolves a non-local binding to the definitions it names, ranked.
// Imports resolve through the module, falling back to the name across the
// project for re-exports (barrel files). Unqualified names prefer
// definitions the file shares (its package or module).
func (x *Index) targets(key string, navigator document.Navigator, name string, binding document.Binding) []entry {
	group := navigator.NavigationGroup()
	if imported := binding.Import; imported != nil {
		var provided []entry
		for _, candidate := range x.lookup(key, group, imported.Name) {
			if navigator.Provides(name, imported.Module, candidate.path) {
				provided = append(provided, candidate)
			}
		}
		if len(provided) == 0 && imported.Name != "default" {
			provided = preferMembers(x.lookup(key, group, imported.Name), false)
		}
		return rank(provided, name)
	}
	candidates := rank(preferMembers(x.lookup(key, group, binding.Name), binding.Member), name)
	if binding.Member {
		return candidates
	}
	var shared []entry
	for _, candidate := range candidates {
		if navigator.Shares(name, candidate.path) {
			shared = append(shared, candidate)
		}
	}
	if len(shared) > 0 {
		return shared
	}
	return candidates
}

// preferMembers keeps candidates whose member-ness matches the use, unless
// none do.
func preferMembers(candidates []entry, member bool) []entry {
	var matching []entry
	for _, candidate := range candidates {
		if candidate.symbol.Member == member {
			matching = append(matching, candidate)
		}
	}
	if len(matching) == 0 {
		return candidates
	}
	return matching
}

// rank orders candidates by closeness to the file: same file, same
// directory, then the longest shared directory prefix, then path order.
func rank(candidates []entry, from string) []entry {
	sort.SliceStable(candidates, func(a, b int) bool {
		ca, cb := closeness(candidates[a].path, from), closeness(candidates[b].path, from)
		if ca != cb {
			return ca > cb
		}
		if candidates[a].path != candidates[b].path {
			return candidates[a].path < candidates[b].path
		}
		return candidates[a].symbol.Position.Before(candidates[b].symbol.Position)
	})
	return candidates
}

func closeness(candidate, from string) int {
	if candidate == from {
		return 1 << 20
	}
	if path.Dir(candidate) == path.Dir(from) {
		return 1 << 19
	}
	shared := 0
	a, b := strings.Split(path.Dir(candidate), "/"), strings.Split(path.Dir(from), "/")
	for shared < len(a) && shared < len(b) && a[shared] == b[shared] {
		shared++
	}
	return shared
}

func locations(entries []entry) []Location {
	result := make([]Location, 0, min(len(entries), maxDefinitions))
	for _, e := range entries[:min(len(entries), maxDefinitions)] {
		result = append(result, Location{Path: e.path, Line: e.symbol.Position.Line, Column: e.symbol.Position.Column, Kind: e.symbol.Kind, Snippet: e.snippet})
	}
	return result
}

// References finds every use of the identifier at a position, including its
// definitions. Locals stay within their scope. Members match by spelling,
// since receivers are not typed. Other names match where they can reach a
// definition of the symbol: through an import of its module, or unqualified
// within the package or module that defines it.
// References lists occurrences of the name at a position that resolve to
// the same symbol. It stops at maxReferences or when ctx ends.
func (x *Index) References(ctx context.Context, key string, source Source, name string, at document.Position) (Result, error) {
	navigator, text, binding, found, err := x.bind(source, name, at)
	if err != nil || !found {
		return Result{Locations: []Location{}}, err
	}
	symbol := binding.Name
	if binding.Import != nil && binding.Import.Name != "default" {
		symbol = binding.Import.Name
	}
	result := Result{Symbol: symbol, Locations: []Location{}}
	if binding.Local != nil {
		occurrences, err := navigator.Occurrences(text, symbol)
		if err != nil {
			return result, err
		}
		lines := splitLines(text)
		for _, occurrence := range occurrences {
			if !occurrence.Member && binding.Local.Scope.Contains(occurrence.Position) {
				result.Locations = append(result.Locations, referenceLocation(name, lines, occurrence))
			}
		}
		return result, nil
	}

	group := navigator.NavigationGroup()
	var targets []entry
	if !binding.Member {
		result.Indexing = !x.fresh(key, source)
		if binding.Definition {
			targets = []entry{{path: name, symbol: document.Symbol{Name: symbol, Position: binding.Position}}}
		} else {
			targets = x.targets(key, navigator, name, binding)
		}
	}
	// reaches reports whether an occurrence in file can name one of the
	// targets. Without targets (undefined in the project), imports match by
	// module and other names by spelling.
	reaches := func(file string, occurrence document.Occurrence) bool {
		if occurrence.Local || occurrence.Member != binding.Member {
			return false
		}
		if binding.Member {
			return true
		}
		if len(targets) == 0 {
			if binding.Import != nil {
				return occurrence.Module == binding.Import.Module
			}
			return occurrence.Module == ""
		}
		for _, target := range targets {
			if occurrence.Module != "" && navigator.Provides(file, occurrence.Module, target.path) ||
				occurrence.Module == "" && navigator.Shares(file, target.path) {
				return true
			}
		}
		// A module that defines nothing by this name may re-export it.
		return occurrence.Module != "" && !x.moduleDefines(key, navigator, file, occurrence.Module, symbol)
	}

	listed, err := source.Files()
	if err != nil {
		return result, err
	}
	var candidates []string
	for _, file := range listed {
		if other, ok := x.registry.Lookup(file.Path).Navigator(); ok && other.NavigationGroup() == group {
			candidates = append(candidates, file.Path)
		}
	}
	// The requested file first, then path order.
	sort.Slice(candidates, func(a, b int) bool {
		if (candidates[a] == name) != (candidates[b] == name) {
			return candidates[a] == name
		}
		return candidates[a] < candidates[b]
	})
	// Batches keep path order and let a capped or cancelled request stop.
	needle := []byte(symbol)
	for start := 0; start < len(candidates); start += maxWorkers * 8 {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		batch := candidates[start:min(start+maxWorkers*8, len(candidates))]
		perFile := parallel(batch, func(candidate string) []Location {
			if ctx.Err() != nil {
				return nil
			}
			text, ok, err := source.Read(candidate)
			if err != nil || !ok || !containsWord(text, needle) {
				return nil
			}
			other, _ := x.registry.Lookup(candidate).Navigator()
			occurrences, _ := other.Occurrences(text, symbol)
			lines := splitLines(text)
			var matched []Location
			for _, occurrence := range occurrences {
				if reaches(candidate, occurrence) {
					matched = append(matched, referenceLocation(candidate, lines, occurrence))
				}
			}
			return matched
		})
		for _, matched := range perFile {
			for _, location := range matched {
				if len(result.Locations) == maxReferences {
					return result, nil
				}
				result.Locations = append(result.Locations, location)
			}
		}
	}
	return result, nil
}

// containsWord reports whether text holds word not inside a longer name,
// so files that only mention it as a substring skip parsing.
func containsWord(text, word []byte) bool {
	for offset := 0; ; {
		at := bytes.Index(text[offset:], word)
		if at < 0 {
			return false
		}
		start, end := offset+at, offset+at+len(word)
		before, _ := utf8.DecodeLastRune(text[:start])
		after, _ := utf8.DecodeRune(text[end:])
		if !nameRune(before) && !nameRune(after) {
			return true
		}
		offset = start + 1
	}
}

func nameRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// moduleDefines reports whether a module imported from file defines name in
// an indexed file.
func (x *Index) moduleDefines(key string, navigator document.Navigator, file, module, name string) bool {
	for _, candidate := range x.lookup(key, navigator.NavigationGroup(), name) {
		if navigator.Provides(file, module, candidate.path) {
			return true
		}
	}
	return false
}

func referenceLocation(name string, lines []string, occurrence document.Occurrence) Location {
	kind := "reference"
	if occurrence.Definition {
		kind = "definition"
	}
	return Location{Path: name, Line: occurrence.Position.Line, Column: occurrence.Position.Column, Kind: kind, Snippet: snippet(lines, occurrence.Position.Line)}
}

func splitLines(source []byte) []string {
	return strings.Split(string(source), "\n")
}

// snippet is the trimmed text of a 1-based line, cut to a readable length.
func snippet(lines []string, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	text := strings.TrimSpace(strings.TrimSuffix(lines[line-1], "\r"))
	if utf8.RuneCountInString(text) <= maxSnippet {
		return text
	}
	return string([]rune(text)[:maxSnippet]) + "…"
}
