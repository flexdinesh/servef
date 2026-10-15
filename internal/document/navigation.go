package document

// Position addresses a source character the way the browser counts it: a
// 1-based line and a 0-based column in UTF-16 code units.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Before reports whether p comes before other.
func (p Position) Before(other Position) bool {
	return p.Line < other.Line || p.Line == other.Line && p.Column < other.Column
}

// Span is a half-open source range.
type Span struct {
	Start Position
	End   Position
}

// Contains reports whether p lies within the span; the end is exclusive.
func (s Span) Contains(p Position) bool { return !p.Before(s.Start) && p.Before(s.End) }

// Symbol is a definition other files reach by name. Member symbols (methods,
// fields, properties) are reached through a receiver or object.
type Symbol struct {
	Name     string
	Kind     string
	Member   bool
	Position Position
}

// Occurrence is an identifier in code (never in strings or comments).
// Local occurrences are bound by a declaration in an enclosing scope of the
// same file, so they never name a project-wide symbol. Module is set for
// names imported or qualified by an import (pkg.Name).
type Occurrence struct {
	Position   Position
	Member     bool
	Local      bool
	Definition bool
	Module     string
}

// Binding describes the identifier at a position as the file alone sees it.
// Local and Import are mutually exclusive; with neither, the name refers to
// a symbol defined elsewhere in the project.
type Binding struct {
	Name     string
	Position Position
	Member   bool
	// Definition reports that the identifier declares what it names.
	Definition bool
	Local      *LocalBinding
	Import     *ImportBinding
}

// LocalBinding is a declaration within the file; Scope bounds its uses.
type LocalBinding struct {
	Kind       string
	Definition Position
	Scope      Span
}

// ImportBinding is a name imported from Module. Statement locates the import
// for when the module is not in the project.
type ImportBinding struct {
	Name      string
	Module    string
	Statement Position
}

// Navigator is an optional Handler capability for code navigation. Methods
// receive bytes only; the project index supplies other files. Malformed
// source yields partial results, never a panic.
type Navigator interface {
	// NavigationGroup names the families whose files share symbols.
	NavigationGroup() string
	// Symbols lists definitions other files can reach.
	Symbols(source []byte) ([]Symbol, error)
	// Bind resolves the identifier at a position; false when there is none.
	Bind(source []byte, at Position) (Binding, bool, error)
	// Occurrences lists identifiers spelled name.
	Occurrences(source []byte, name string) ([]Occurrence, error)
	// Provides reports whether the file at candidate can define names that
	// importer imports from module. Paths are project-relative.
	Provides(importer, module, candidate string) bool
	// Shares reports whether unqualified, unimported names in file can refer
	// to top-level definitions in candidate: a Go package's directory, a
	// JavaScript module's own file.
	Shares(file, candidate string) bool
}
