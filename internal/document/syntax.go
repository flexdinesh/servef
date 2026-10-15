package document

import (
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

// parseTimeoutMicros bounds one parse so a pathological file cannot stall
// navigation; the tree parsed so far is still used.
const parseTimeoutMicros = 2_000_000

// grammar loads a tree-sitter language and its parser pool on first use, so
// startup never pays for grammars that are not navigated.
type grammar struct {
	load func() *gotreesitter.Language
	once sync.Once
	lang *gotreesitter.Language
	pool *gotreesitter.ParserPool
}

func newGrammar(load func() *gotreesitter.Language) *grammar { return &grammar{load: load} }

func (g *grammar) init() {
	g.once.Do(func() {
		g.lang = g.load()
		if g.lang != nil {
			g.pool = gotreesitter.NewParserPool(g.lang, gotreesitter.WithParserPoolTimeoutMicros(parseTimeoutMicros))
		}
	})
}

// analyze parses source and extracts a model with build. Parser panics
// become errors so malformed input never takes down the server.
func (g *grammar) analyze(source []byte, build func(*walker, *gotreesitter.Node)) (model syntaxModel, err error) {
	g.init()
	if g.pool == nil {
		return syntaxModel{}, errors.New("grammar unavailable")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			model, err = syntaxModel{}, fmt.Errorf("parse failed: %v", recovered)
		}
	}()
	tree, err := g.pool.Parse(source)
	if err != nil {
		return syntaxModel{}, err
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		return syntaxModel{}, errors.New("parse produced no tree")
	}
	w := &walker{lang: g.lang, text: newSourceText(source)}
	build(w, root)
	return w.model, nil
}

// sourceText converts tree-sitter byte points to browser positions.
type sourceText struct {
	source []byte
	starts []int
}

func newSourceText(source []byte) sourceText {
	starts := []int{0}
	for index, b := range source {
		if b == '\n' {
			starts = append(starts, index+1)
		}
	}
	return sourceText{source: source, starts: starts}
}

func (t sourceText) position(point gotreesitter.Point) Position {
	row := min(int(point.Row), len(t.starts)-1)
	start := t.starts[row]
	end := min(start+int(point.Column), len(t.source))
	return Position{Line: row + 1, Column: utf16Length(t.source[start:end])}
}

func utf16Length(text []byte) int {
	length := 0
	for len(text) > 0 {
		r, size := utf8.DecodeRune(text)
		text = text[size:]
		if r >= 0x10000 {
			length += 2
		} else {
			length++
		}
	}
	return length
}

// identifier is one identifier node. Qualifier is the plain identifier
// before a dot (pkg.Name, ns.name), which may name an import.
type identifier struct {
	name       string
	span       Span
	member     bool
	qualifier  string
	definition bool
	module     string
}

// declaration is a name bound within the file. Imports carry their module;
// namespace imports (Go packages, import * as ns) name a module, not a
// symbol.
type declaration struct {
	name      string
	kind      string
	position  Position
	scope     Span
	module    string
	imported  string
	namespace bool
}

func (d declaration) isImport() bool { return d.module != "" }

// syntaxModel is what navigation needs from one file.
type syntaxModel struct {
	symbols      []Symbol
	declarations []declaration
	identifiers  []identifier
}

// resolve finds the declaration name refers to at a position: the innermost
// enclosing scope wins; within it, the nearest declaration at or before the
// position, else the first after it (hoisting).
func (m syntaxModel) resolve(name string, at Position) (declaration, bool) {
	var best declaration
	found := false
	for _, candidate := range m.declarations {
		if candidate.name != name || !candidate.scope.Contains(at) {
			continue
		}
		if !found || innerScope(candidate.scope, best.scope) || sameScope(candidate.scope, best.scope) && nearer(candidate.position, best.position, at) {
			best, found = candidate, true
		}
	}
	return best, found
}

func innerScope(a, b Span) bool {
	return !sameScope(a, b) && !a.Start.Before(b.Start) && !b.End.Before(a.End)
}

func sameScope(a, b Span) bool { return a == b }

// nearer prefers declarations at or before at, the latest first; otherwise
// the earliest after it.
func nearer(candidate, current, at Position) bool {
	candidateBefore, currentBefore := !at.Before(candidate), !at.Before(current)
	if candidateBefore != currentBefore {
		return candidateBefore
	}
	if candidateBefore {
		return current.Before(candidate)
	}
	return candidate.Before(current)
}

// namespaceQualifier reports whether qualifier names a namespace import at a
// position, making qualifier.name a symbol of that module.
func (m syntaxModel) namespaceImport(qualifier string, at Position) (declaration, bool) {
	if qualifier == "" {
		return declaration{}, false
	}
	found, ok := m.resolve(qualifier, at)
	return found, ok && found.namespace
}

func (m syntaxModel) binding(at Position) (Binding, bool) {
	for _, id := range m.identifiers {
		if !id.span.Contains(at) {
			continue
		}
		binding := Binding{Name: id.name, Position: id.span.Start, Member: id.member, Definition: id.definition}
		if module, ok := m.namespaceImport(id.qualifier, id.span.Start); id.member && ok {
			binding.Member = false
			binding.Import = &ImportBinding{Name: id.name, Module: module.module, Statement: module.position}
			return binding, true
		}
		if id.member {
			return binding, true
		}
		found, ok := m.resolve(id.name, id.span.Start)
		if !ok {
			return binding, true
		}
		if found.isImport() && !found.namespace {
			binding.Import = &ImportBinding{Name: found.imported, Module: found.module, Statement: found.position}
			return binding, true
		}
		binding.Local = &LocalBinding{Kind: found.kind, Definition: found.position, Scope: found.scope}
		binding.Definition = binding.Definition || found.position == id.span.Start
		return binding, true
	}
	return Binding{}, false
}

func (m syntaxModel) occurrences(name string) []Occurrence {
	var result []Occurrence
	for _, id := range m.identifiers {
		if id.name != name {
			continue
		}
		occurrence := Occurrence{Position: id.span.Start, Member: id.member, Definition: id.definition, Module: id.module}
		if module, ok := m.namespaceImport(id.qualifier, id.span.Start); id.member && ok {
			occurrence.Member = false
			occurrence.Module = module.module
		} else if !id.member && id.module == "" {
			if found, ok := m.resolve(name, id.span.Start); ok {
				if found.isImport() && !found.namespace {
					occurrence.Module = found.module
				} else {
					occurrence.Local = true
				}
			}
		}
		result = append(result, occurrence)
	}
	return result
}

// walker builds a syntaxModel from a tree.
type walker struct {
	lang  *gotreesitter.Language
	text  sourceText
	model syntaxModel
}

func (w *walker) kind(n *gotreesitter.Node) string { return n.Type(w.lang) }

func (w *walker) source(n *gotreesitter.Node) string { return n.Text(w.text.source) }

func (w *walker) span(n *gotreesitter.Node) Span {
	return Span{Start: w.text.position(n.StartPoint()), End: w.text.position(n.EndPoint())}
}

// fields returns the children stored under a field name, in order.
func (w *walker) fields(n *gotreesitter.Node, name string) []*gotreesitter.Node {
	var result []*gotreesitter.Node
	for index, count := 0, n.ChildCount(); index < count; index++ {
		if n.FieldNameForChild(index, w.lang) == name {
			if child := n.Child(index); child != nil {
				result = append(result, child)
			}
		}
	}
	return result
}

func (w *walker) field(n *gotreesitter.Node, name string) *gotreesitter.Node {
	if found := w.fields(n, name); len(found) > 0 {
		return found[0]
	}
	return nil
}

// named lists n's named children in one pass; gotreesitter's NamedChild
// scans from the first child, so indexing it is quadratic on wide nodes.
func (w *walker) named(n *gotreesitter.Node) []*gotreesitter.Node {
	count := n.ChildCount()
	result := make([]*gotreesitter.Node, 0, count)
	for index := 0; index < count; index++ {
		if child := n.Child(index); child != nil && child.IsNamed() {
			result = append(result, child)
		}
	}
	return result
}

// hasToken reports whether n has an anonymous child spelled token.
func (w *walker) hasToken(n *gotreesitter.Node, token string) bool {
	for index, count := 0, n.ChildCount(); index < count; index++ {
		if child := n.Child(index); child != nil && !child.IsNamed() && w.kind(child) == token {
			return true
		}
	}
	return false
}

func sameNode(a, b *gotreesitter.Node) bool {
	return a != nil && b != nil && a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte() && a.Symbol() == b.Symbol()
}

func (w *walker) reference(n *gotreesitter.Node, member bool, qualifier string) {
	w.model.identifiers = append(w.model.identifiers, identifier{name: w.source(n), span: w.span(n), member: member, qualifier: qualifier})
}

// symbol records a project-wide definition named by n.
func (w *walker) symbol(n *gotreesitter.Node, kind string, member bool) {
	position := w.span(n).Start
	w.model.symbols = append(w.model.symbols, Symbol{Name: w.source(n), Kind: kind, Member: member, Position: position})
	w.model.identifiers = append(w.model.identifiers, identifier{name: w.source(n), span: w.span(n), member: member, definition: true})
}

// member records a member definition that is not indexed, such as a field of
// a local type.
func (w *walker) memberDefinition(n *gotreesitter.Node) {
	w.model.identifiers = append(w.model.identifiers, identifier{name: w.source(n), span: w.span(n), member: true, definition: true})
}

// local records a declaration visible within scope.
func (w *walker) local(n *gotreesitter.Node, kind string, scope Span) {
	w.model.declarations = append(w.model.declarations, declaration{name: w.source(n), kind: kind, position: w.span(n).Start, scope: scope})
	w.model.identifiers = append(w.model.identifiers, identifier{name: w.source(n), span: w.span(n), definition: true})
}

// imported records an import binding visible across the file.
func (w *walker) imported(n *gotreesitter.Node, scope Span, binding declaration) {
	binding.position = w.span(n).Start
	binding.scope = scope
	w.model.declarations = append(w.model.declarations, binding)
	if binding.name != "" {
		w.model.identifiers = append(w.model.identifiers, identifier{name: binding.name, span: w.span(n), definition: true})
	}
}

// syntaxNavigator implements Navigator over a grammar and a per-language
// model builder.
type syntaxNavigator struct {
	group    string
	grammar  *grammar
	build    func(*walker, *gotreesitter.Node)
	provides func(importer, module, candidate string) bool
	shares   func(file, candidate string) bool
}

func (n syntaxNavigator) NavigationGroup() string { return n.group }

func (n syntaxNavigator) Symbols(source []byte) ([]Symbol, error) {
	model, err := n.grammar.analyze(source, n.build)
	return model.symbols, err
}

func (n syntaxNavigator) Bind(source []byte, at Position) (Binding, bool, error) {
	model, err := n.grammar.analyze(source, n.build)
	if err != nil {
		return Binding{}, false, err
	}
	binding, ok := model.binding(at)
	return binding, ok, nil
}

func (n syntaxNavigator) Occurrences(source []byte, name string) ([]Occurrence, error) {
	model, err := n.grammar.analyze(source, n.build)
	return model.occurrences(name), err
}

func (n syntaxNavigator) Provides(importer, module, candidate string) bool {
	return n.provides(importer, module, candidate)
}

func (n syntaxNavigator) Shares(file, candidate string) bool { return n.shares(file, candidate) }
