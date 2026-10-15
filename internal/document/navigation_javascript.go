package document

import (
	"path"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/javascript"
	"github.com/odvcencio/gotreesitter/grammars/tsx"
	"github.com/odvcencio/gotreesitter/grammars/typescript"
)

var (
	javaScriptGrammar = newGrammar(javascript.Language)
	typeScriptGrammar = newGrammar(typescript.Language)
	tsxGrammar        = newGrammar(tsx.Language)
)

// JavaScript, JSX, TypeScript, and TSX import each other, so they share one
// navigation group.
const javaScriptGroup = "javascript"

func javaScriptNavigator(g *grammar) syntaxNavigator {
	return syntaxNavigator{group: javaScriptGroup, grammar: g, build: buildJavaScriptModel, provides: javaScriptProvides, shares: sameModule}
}

// sameModule reports one file: modules see only their own top-level names
// and what they import.
func sameModule(file, candidate string) bool { return file == candidate }

// javaScriptExtensions lists source extensions, longest suffix first.
var javaScriptExtensions = []string{".d.ts", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}

// javaScriptProvides resolves a module specifier to candidate files without
// reading the filesystem. Relative specifiers resolve against the importer,
// with or without an extension (TypeScript's ./a.js names a.ts), or to an
// index file. @/ and ~/ aliases match by path suffix. Bare packages live in
// node_modules, outside navigation, and match nothing.
func javaScriptProvides(importer, module, candidate string) bool {
	stem := trimJavaScriptExtension(candidate)
	if stem == candidate {
		return false
	}
	exact := func(want string) bool { return stem == want || stem == want+"/index" }
	matches := exact
	var base string
	switch {
	case strings.HasPrefix(module, "./") || strings.HasPrefix(module, "../"):
		base = path.Join(path.Dir(importer), module)
	case strings.HasPrefix(module, "@/") || strings.HasPrefix(module, "~/"):
		base = module[2:]
		matches = func(want string) bool {
			return exact(want) || strings.HasSuffix(stem, "/"+want) || strings.HasSuffix(stem, "/"+want+"/index")
		}
	default:
		return false
	}
	return matches(base) || matches(trimJavaScriptExtension(base))
}

func trimJavaScriptExtension(name string) string {
	for _, extension := range javaScriptExtensions {
		if strings.HasSuffix(name, extension) && len(name) > len(extension) {
			return strings.TrimSuffix(name, extension)
		}
	}
	return name
}

// jsContext tracks where a node is: scope bounds block-scoped locals,
// function bounds parameters and var, and topLevel reports module level.
// members reports a top-level class, interface, or object type, whose
// members are indexed.
type jsContext struct {
	scope    Span
	function Span
	topLevel bool
	members  bool
}

var jsScopes = map[string]bool{
	"statement_block": true, "for_statement": true, "for_in_statement": true, "catch_clause": true,
	"switch_body": true, "class_body": true,
}

var jsFunctions = map[string]bool{
	"function_declaration": true, "generator_function_declaration": true, "function_expression": true,
	"generator_function": true, "function": true, "arrow_function": true, "method_definition": true,
}

var jsIdentifiers = map[string]bool{
	"identifier": true, "property_identifier": true, "type_identifier": true,
	"shorthand_property_identifier": true, "private_property_identifier": true,
}

// jsModuleLevel lists nodes that keep their children at module level:
// exports, ambient declarations, and namespace bodies.
var jsModuleLevel = map[string]bool{
	"program": true, "export_statement": true, "ambient_declaration": true, "expression_statement": true,
	"internal_module": true, "module": true,
}

var jsDeclarations = map[string]string{
	"function_declaration": "function", "generator_function_declaration": "function", "function_signature": "function",
	"class_declaration": "class", "abstract_class_declaration": "class", "interface_declaration": "interface",
	"type_alias_declaration": "type", "enum_declaration": "enum", "internal_module": "namespace", "module": "namespace",
}

var jsMembers = map[string]string{
	"method_definition": "method", "method_signature": "method", "abstract_method_signature": "method",
	"public_field_definition": "property", "field_definition": "property", "property_signature": "property",
}

func buildJavaScriptModel(w *walker, root *gotreesitter.Node) {
	file := w.span(root)
	file.End = Position{Line: file.End.Line + 1}
	w.jsVisit(root, jsContext{scope: file, function: file, topLevel: true})
}

func (w *walker) jsVisit(n *gotreesitter.Node, ctx jsContext) {
	kind := w.kind(n)
	parentTopLevel := ctx.topLevel
	if !jsModuleLevel[kind] && !(kind == "statement_block" && ctx.topLevel && w.jsNamespaceBody(n)) {
		ctx.topLevel = false
	}
	switch kind {
	case "comment", "string", "template_string", "regex":
		if kind == "template_string" {
			w.jsChildren(n, ctx)
		}
		return
	case "import_statement":
		w.jsImport(n, ctx)
		return
	case "export_statement":
		if parentTopLevel && w.hasToken(n, "default") {
			w.jsDefaultExport(n)
		}
	}
	if jsScopes[kind] {
		ctx.scope = w.span(n)
	}
	if declared, ok := jsDeclarations[kind]; ok {
		name := w.field(n, "name")
		if name != nil && w.kind(name) != "string" {
			if parentTopLevel {
				w.symbol(name, declared, false)
			} else {
				w.local(name, declared, ctx.scope)
			}
		}
		inner := ctx
		inner.members = parentTopLevel && !jsFunctions[kind] && kind != "function_signature"
		inner.topLevel = (kind == "internal_module" || kind == "module") && parentTopLevel
		if jsFunctions[kind] {
			inner.scope, inner.function = w.span(n), w.span(n)
		} else if kind != "internal_module" && kind != "module" {
			// Type parameters of classes, interfaces, and aliases.
			inner.function = w.span(n)
		}
		w.jsChildren(n, inner, name)
		return
	}
	if member, ok := jsMembers[kind]; ok {
		name := w.field(n, "name")
		if name == nil {
			name = w.field(n, "property")
		}
		if name != nil && w.kind(name) != "computed_property_name" {
			if ctx.members {
				w.symbol(name, member, true)
			} else {
				w.memberDefinition(name)
			}
		}
		inner := ctx
		inner.members = false
		if jsFunctions[kind] {
			inner.scope, inner.function = w.span(n), w.span(n)
		}
		w.jsChildren(n, inner, name)
		return
	}
	switch kind {
	case "interface_body", "object_type":
		inner := ctx
		inner.members = ctx.members
		w.jsChildren(n, inner)
		return
	case "class_body":
		w.jsChildren(n, ctx)
		return
	case "function_expression", "generator_function", "function", "arrow_function":
		inner := ctx
		inner.scope, inner.function = w.span(n), w.span(n)
		name := w.field(n, "name")
		if name != nil {
			w.local(name, "function", inner.scope)
		}
		if parameter := w.field(n, "parameter"); parameter != nil {
			w.local(parameter, "parameter", inner.function)
			w.jsChildren(n, inner, name, parameter)
			return
		}
		w.jsChildren(n, inner, name)
		return
	case "formal_parameters":
		for _, parameter := range w.named(n) {
			w.jsParameter(parameter, ctx)
		}
		return
	case "variable_declarator":
		name := w.field(n, "name")
		if name != nil {
			valueKind := "variable"
			if parent := n.Parent(); parent != nil && w.hasToken(parent, "const") {
				valueKind = "constant"
			}
			scope := ctx.scope
			if parent := n.Parent(); parent != nil && w.kind(parent) == "variable_declaration" {
				scope = ctx.function
			}
			w.jsBindings(name, valueKind, scope, parentTopLevel || w.jsDeclaredAtTopLevel(n), ctx)
		}
		w.jsChildren(n, ctx, name)
		return
	case "for_in_statement":
		left := w.field(n, "left")
		if left != nil && (w.hasToken(n, "const") || w.hasToken(n, "let") || w.hasToken(n, "var")) {
			w.jsBindings(left, "variable", ctx.scope, false, ctx)
			w.jsChildren(n, ctx, left)
			return
		}
	case "catch_clause":
		if parameter := w.field(n, "parameter"); parameter != nil {
			w.jsBindings(parameter, "variable", ctx.scope, false, ctx)
			w.jsChildren(n, ctx, parameter)
			return
		}
	case "type_parameter":
		if name := w.field(n, "name"); name != nil {
			w.local(name, "type parameter", ctx.function)
			w.jsChildren(n, ctx, name)
			return
		}
	case "member_expression":
		object, property := w.field(n, "object"), w.field(n, "property")
		if object != nil {
			w.jsVisit(object, ctx)
		}
		if property != nil {
			w.reference(property, true, w.jsQualifier(object))
		}
		return
	case "nested_type_identifier":
		module, name := w.field(n, "module"), w.field(n, "name")
		if module != nil {
			w.jsVisit(module, ctx)
		}
		if name != nil {
			w.reference(name, true, w.jsQualifier(module))
		}
		return
	case "pair":
		// Object keys name properties; values are expressions.
		if key := w.field(n, "key"); key != nil && w.kind(key) == "property_identifier" {
			w.reference(key, true, "")
			w.jsChildren(n, ctx, key)
			return
		}
	case "jsx_attribute":
		for _, child := range w.named(n) {
			if w.kind(child) == "property_identifier" {
				w.reference(child, true, "")
			} else {
				w.jsVisit(child, ctx)
			}
		}
		return
	}
	if jsIdentifiers[kind] {
		member := kind == "property_identifier" || kind == "private_property_identifier"
		w.reference(n, member, "")
		return
	}
	w.jsChildren(n, ctx)
}

func (w *walker) jsChildren(n *gotreesitter.Node, ctx jsContext, skip ...*gotreesitter.Node) {
	for _, child := range w.named(n) {
		if !containsNode(skip, child) {
			w.jsVisit(child, ctx)
		}
	}
}

// jsNamespaceBody reports a namespace's body, whose declarations stay at
// module level.
func (w *walker) jsNamespaceBody(n *gotreesitter.Node) bool {
	parent := n.Parent()
	return parent != nil && (w.kind(parent) == "internal_module" || w.kind(parent) == "module")
}

// jsDeclaredAtTopLevel reports a variable declarator directly in a
// module-level statement.
func (w *walker) jsDeclaredAtTopLevel(declarator *gotreesitter.Node) bool {
	statement := declarator.Parent()
	if statement == nil {
		return false
	}
	for parent := statement.Parent(); parent != nil; parent = parent.Parent() {
		kind := w.kind(parent)
		if kind == "program" {
			return true
		}
		if kind == "statement_block" && w.jsNamespaceBody(parent) {
			continue
		}
		if !jsModuleLevel[kind] {
			return false
		}
	}
	return false
}

func (w *walker) jsParameter(parameter *gotreesitter.Node, ctx jsContext) {
	switch w.kind(parameter) {
	case "required_parameter", "optional_parameter":
		pattern := w.field(parameter, "pattern")
		if pattern != nil {
			w.jsBindings(pattern, "parameter", ctx.function, false, ctx)
		}
		w.jsChildren(parameter, ctx, pattern)
	default:
		w.jsBindings(parameter, "parameter", ctx.function, false, ctx)
	}
}

// jsBindings declares every name a binding pattern introduces; default
// values and computed keys are visited as expressions.
func (w *walker) jsBindings(pattern *gotreesitter.Node, kind string, scope Span, topLevel bool, ctx jsContext) {
	switch w.kind(pattern) {
	case "identifier", "shorthand_property_identifier_pattern":
		if topLevel {
			w.symbol(pattern, kind, false)
		} else {
			w.local(pattern, kind, scope)
		}
	case "object_pattern", "array_pattern", "rest_pattern":
		for _, child := range w.named(pattern) {
			w.jsBindings(child, kind, scope, topLevel, ctx)
		}
	case "pair_pattern":
		if key := w.field(pattern, "key"); key != nil && w.kind(key) == "property_identifier" {
			w.reference(key, true, "")
		}
		if value := w.field(pattern, "value"); value != nil {
			w.jsBindings(value, kind, scope, topLevel, ctx)
		}
	case "assignment_pattern", "object_assignment_pattern":
		left, right := w.field(pattern, "left"), w.field(pattern, "right")
		if left != nil {
			w.jsBindings(left, kind, scope, topLevel, ctx)
		}
		if right != nil {
			w.jsVisit(right, ctx)
		}
	default:
		w.jsVisit(pattern, ctx)
	}
}

func (w *walker) jsQualifier(object *gotreesitter.Node) string {
	if object != nil && w.kind(object) == "identifier" {
		return w.source(object)
	}
	return ""
}

func (w *walker) jsImport(n *gotreesitter.Node, ctx jsContext) {
	source := w.field(n, "source")
	if source == nil {
		return
	}
	module := strings.Trim(w.source(source), "\"'`")
	if module == "" {
		return
	}
	var clause *gotreesitter.Node
	for _, child := range w.named(n) {
		if w.kind(child) == "import_clause" {
			clause = child
		}
	}
	if clause == nil {
		return
	}
	for _, part := range w.named(clause) {
		switch w.kind(part) {
		case "identifier":
			w.imported(part, ctx.scope, declaration{name: w.source(part), kind: "import", module: module, imported: "default"})
		case "namespace_import":
			for _, name := range w.named(part) {
				if w.kind(name) == "identifier" {
					w.imported(name, ctx.scope, declaration{name: w.source(name), kind: "import", module: module, namespace: true})
				}
			}
		case "named_imports":
			for _, specifier := range w.named(part) {
				name, alias := w.field(specifier, "name"), w.field(specifier, "alias")
				if name == nil {
					continue
				}
				local := name
				if alias != nil {
					local = alias
					// The original name refers to the exported symbol.
					w.model.identifiers = append(w.model.identifiers, identifier{name: w.source(name), span: w.span(name), module: module})
				}
				w.imported(local, ctx.scope, declaration{name: w.source(local), kind: "import", module: module, imported: w.source(name)})
			}
		}
	}
}

// jsDefaultExport indexes the default export as "default", at the declared
// name when there is one.
func (w *walker) jsDefaultExport(n *gotreesitter.Node) {
	target := n
	if declared := w.field(n, "declaration"); declared != nil {
		if name := w.field(declared, "name"); name != nil {
			target = name
		}
	} else if value := w.field(n, "value"); value != nil {
		target = value
	}
	w.model.symbols = append(w.model.symbols, Symbol{Name: "default", Kind: "default export", Position: w.span(target).Start})
}
