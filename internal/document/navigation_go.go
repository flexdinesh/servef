package document

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	golang "github.com/odvcencio/gotreesitter/grammars/go"
)

var goGrammar = newGrammar(golang.Language)

func goNavigator() syntaxNavigator {
	return syntaxNavigator{group: "go", grammar: goGrammar, build: buildGoModel, provides: goProvides, shares: samePackage}
}

// samePackage reports files in one directory, which Go compiles as one
// package.
func samePackage(file, candidate string) bool { return path.Dir(file) == path.Dir(candidate) }

// goProvides matches an import path to the package directory it names by
// suffix: module paths are not read, so "example.com/app/internal/files"
// matches the directory internal/files.
func goProvides(_, module, candidate string) bool {
	dir := path.Dir(candidate)
	return dir != "." && (module == dir || strings.HasSuffix(module, "/"+dir))
}

// goContext tracks where a node is: scope bounds new locals, function bounds
// parameters, and global reports package level.
type goContext struct {
	scope    Span
	function Span
	global   bool
}

var goScopes = map[string]bool{
	"block": true, "for_statement": true, "if_statement": true, "expression_switch_statement": true,
	"type_switch_statement": true, "select_statement": true, "expression_case": true, "type_case": true,
	"default_case": true, "communication_case": true,
}

var goIdentifiers = map[string]bool{"identifier": true, "field_identifier": true, "type_identifier": true, "package_identifier": true}

func buildGoModel(w *walker, root *gotreesitter.Node) {
	file := w.span(root)
	// Package-level names are visible throughout, so the file bounds imports.
	file.End = Position{Line: file.End.Line + 1}
	w.goVisit(root, goContext{scope: file, function: file, global: true})
}

func (w *walker) goVisit(n *gotreesitter.Node, ctx goContext) {
	kind := w.kind(n)
	if goScopes[kind] {
		ctx.scope = w.span(n)
	}
	switch kind {
	case "package_clause", "comment", "interpreted_string_literal", "raw_string_literal":
		return
	case "import_spec":
		w.goImport(n, ctx)
		return
	case "function_declaration", "method_declaration", "func_literal":
		name := w.field(n, "name")
		if name != nil {
			if kind == "method_declaration" {
				w.symbol(name, "method", true)
			} else if ctx.global {
				w.symbol(name, "function", false)
			}
		}
		inner := goContext{scope: w.span(n), function: w.span(n)}
		w.goChildren(n, inner, name)
		return
	case "type_spec", "type_alias":
		name := w.field(n, "name")
		if name != nil {
			typeKind := "type"
			if body := w.field(n, "type"); body != nil && w.kind(body) == "interface_type" {
				typeKind = "interface"
			}
			if ctx.global {
				w.symbol(name, typeKind, false)
			} else {
				w.local(name, typeKind, ctx.scope)
			}
		}
		inner := ctx
		inner.function = w.span(n)
		w.goChildren(n, inner, name)
		return
	case "type_parameter_declaration", "parameter_declaration", "variadic_parameter_declaration":
		names := w.fields(n, "name")
		localKind := "parameter"
		if kind == "type_parameter_declaration" {
			localKind = "type parameter"
		}
		for _, name := range names {
			w.local(name, localKind, ctx.function)
		}
		w.goChildren(n, ctx, names...)
		return
	case "const_spec", "var_spec":
		names := w.fields(n, "name")
		valueKind := "variable"
		if kind == "const_spec" {
			valueKind = "constant"
		}
		for _, name := range names {
			if ctx.global {
				w.symbol(name, valueKind, false)
			} else {
				w.local(name, valueKind, ctx.scope)
			}
		}
		w.goChildren(n, ctx, names...)
		return
	case "field_declaration", "method_elem":
		names := w.fields(n, "name")
		memberKind := "field"
		if kind == "method_elem" {
			memberKind = "method"
		}
		for _, name := range names {
			if ctx.global {
				w.symbol(name, memberKind, true)
			} else {
				w.memberDefinition(name)
			}
		}
		w.goChildren(n, ctx, names...)
		return
	case "short_var_declaration", "range_clause", "receive_statement":
		left := w.field(n, "left")
		if left != nil && (kind == "short_var_declaration" || w.hasToken(n, ":=")) {
			w.goLocals(left, ctx.scope)
			w.goChildren(n, ctx, left)
			return
		}
	case "type_switch_statement":
		if alias := w.field(n, "alias"); alias != nil {
			w.goLocals(alias, ctx.scope)
			w.goChildren(n, ctx, alias)
			return
		}
	case "selector_expression":
		operand, field := w.field(n, "operand"), w.field(n, "field")
		if operand != nil {
			w.goVisit(operand, ctx)
		}
		if field != nil {
			w.reference(field, true, w.goQualifier(operand))
		}
		return
	case "qualified_type":
		pkg, name := w.field(n, "package"), w.field(n, "name")
		if pkg != nil {
			w.reference(pkg, false, "")
		}
		if name != nil {
			w.reference(name, true, w.goQualifier(pkg))
		}
		return
	case "keyed_element":
		// Composite literal keys name struct fields.
		if key := w.field(n, "key"); key != nil {
			if keys := w.named(key); len(keys) == 1 && w.kind(keys[0]) == "identifier" {
				w.reference(keys[0], true, "")
				w.goChildren(n, ctx, key)
				return
			}
		}
	}
	if goIdentifiers[kind] {
		w.reference(n, kind == "field_identifier", "")
		return
	}
	w.goChildren(n, ctx)
}

func (w *walker) goChildren(n *gotreesitter.Node, ctx goContext, skip ...*gotreesitter.Node) {
	for _, child := range w.named(n) {
		if !containsNode(skip, child) {
			w.goVisit(child, ctx)
		}
	}
}

func containsNode(nodes []*gotreesitter.Node, n *gotreesitter.Node) bool {
	for _, candidate := range nodes {
		if sameNode(candidate, n) {
			return true
		}
	}
	return false
}

func (w *walker) goLocals(list *gotreesitter.Node, scope Span) {
	for _, name := range w.named(list) {
		if w.kind(name) == "identifier" && w.source(name) != "_" {
			w.local(name, "variable", scope)
		}
	}
}

func (w *walker) goQualifier(operand *gotreesitter.Node) string {
	if operand != nil && (w.kind(operand) == "identifier" || w.kind(operand) == "package_identifier") {
		return w.source(operand)
	}
	return ""
}

func (w *walker) goImport(n *gotreesitter.Node, ctx goContext) {
	pathNode := w.field(n, "path")
	if pathNode == nil {
		return
	}
	module, err := strconv.Unquote(w.source(pathNode))
	if err != nil || module == "" {
		return
	}
	name := goPackageName(module)
	at := pathNode
	if alias := w.field(n, "name"); alias != nil {
		name = w.source(alias)
		at = alias
	}
	if name == "_" || name == "." {
		return
	}
	w.imported(at, ctx.scope, declaration{name: name, kind: "import", module: module, namespace: true})
}

var goMajorVersion = regexp.MustCompile(`^v[0-9]+$`)

// goPackageName guesses the package name an import path declares, by the
// usual conventions: the last element, skipping a major version suffix and
// dropping a go- prefix or .vN suffix.
func goPackageName(module string) string {
	parts := strings.Split(module, "/")
	name := parts[len(parts)-1]
	if goMajorVersion.MatchString(name) && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	name = strings.TrimPrefix(name, "go-")
	if dot := strings.Index(name, ".v"); dot > 0 {
		name = name[:dot]
	}
	return strings.ReplaceAll(name, "-", "")
}
