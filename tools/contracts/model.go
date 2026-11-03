package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// model is the parsed contract: the generated files and every type they
// declare.
type model struct {
	files []*sourceFile
	types map[string]*typeDecl
}

// sourceFile is one generated contract file and the TypeScript module it
// becomes.
type sourceFile struct {
	name   string // projects.go
	module string // projects
	decls  []*typeDecl
}

type declKind int

const (
	structDecl declKind = iota
	enumDecl
)

// typeDecl is a struct (an interface and guard) or a named string type with
// constants (a union and guard).
type typeDecl struct {
	name   string
	doc    []string
	file   *sourceFile
	kind   declKind
	fields []field
	values []string
	rules  []rule
}

type field struct {
	json     string
	optional bool
	ref      typeRef
	doc      []string
}

type refKind int

const (
	stringRef refKind = iota
	booleanRef
	numberRef
	arrayRef
	nullableRef
	namedRef
)

// typeRef is a field's type: a JSON scalar, an array, a nullable named type
// (a Go pointer), or a named contract type.
type typeRef struct {
	kind refKind
	elem *typeRef
	name string
}

// rule is a cross-field implication: when holds, then must hold.
type rule struct {
	when, then condition
}

// condition compares a field with a JSON scalar literal.
type condition struct {
	field   string
	equal   bool
	literal string
}

var ruleLine = regexp.MustCompile(`^//contract:rule (\w+) (==|!=) (.+?) => (\w+) (==|!=) (.+)$`)

// load parses the generated contract files in dir. Every Go file in dir must
// be listed as generated or Go-only.
func load(dir string, generated, goOnly []string) (*model, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !slices.Contains(generated, name) && !slices.Contains(goOnly, name) {
			return nil, fmt.Errorf("%s: list it in tools/contracts as generated or Go-only", name)
		}
		present[name] = true
	}
	for _, name := range append(slices.Clone(generated), goOnly...) {
		if !present[name] {
			return nil, fmt.Errorf("%s: listed in tools/contracts but missing", name)
		}
	}
	m := &model{types: map[string]*typeDecl{}}
	files := token.NewFileSet()
	var enumValues []struct {
		typeName, value, at string
	}
	for _, name := range generated {
		parsed, err := parser.ParseFile(files, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		source := &sourceFile{name: name, module: strings.TrimSuffix(name, ".go")}
		m.files = append(m.files, source)
		for _, decl := range parsed.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok {
				return nil, fmt.Errorf("%s: generated files declare only types and constants", files.Position(decl.Pos()))
			}
			switch general.Tok {
			case token.IMPORT:
			case token.TYPE:
				for _, spec := range general.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						return nil, fmt.Errorf("%s: unexpected type spec", files.Position(spec.Pos()))
					}
					doc := typeSpec.Doc
					if doc == nil && len(general.Specs) == 1 {
						doc = general.Doc
					}
					parsedType, err := parseType(files, source, typeSpec, doc)
					if err != nil {
						return nil, err
					}
					if _, ok := m.types[parsedType.name]; ok {
						return nil, fmt.Errorf("%s: %s declared twice", files.Position(typeSpec.Pos()), parsedType.name)
					}
					m.types[parsedType.name] = parsedType
					source.decls = append(source.decls, parsedType)
				}
			case token.CONST:
				for _, spec := range general.Specs {
					value, ok := spec.(*ast.ValueSpec)
					at := files.Position(spec.Pos()).String()
					if !ok || value.Type == nil || len(value.Names) != 1 || len(value.Values) != 1 {
						return nil, fmt.Errorf("%s: generated constants are single, explicitly typed enum values", at)
					}
					typeName, ok := value.Type.(*ast.Ident)
					literal, isLiteral := value.Values[0].(*ast.BasicLit)
					if !ok || !isLiteral || literal.Kind != token.STRING {
						return nil, fmt.Errorf("%s: generated constants are string values of an enum type", at)
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", at, err)
					}
					enumValues = append(enumValues, struct{ typeName, value, at string }{typeName.Name, text, at})
				}
			default:
				return nil, fmt.Errorf("%s: generated files declare only types and constants", files.Position(general.Pos()))
			}
		}
	}
	for _, value := range enumValues {
		decl, ok := m.types[value.typeName]
		if !ok || decl.kind != enumDecl {
			return nil, fmt.Errorf("%s: %s is not an enum type in the generated files", value.at, value.typeName)
		}
		decl.values = append(decl.values, value.value)
	}
	for _, decl := range m.types {
		if decl.kind == enumDecl && len(decl.values) == 0 {
			return nil, fmt.Errorf("%s: enum %s has no values", decl.file.name, decl.name)
		}
		for _, field := range decl.fields {
			if err := m.checkRef(field.ref); err != nil {
				return nil, fmt.Errorf("%s: %s.%s: %w", decl.file.name, decl.name, field.json, err)
			}
		}
		for _, rule := range decl.rules {
			for _, condition := range []condition{rule.when, rule.then} {
				if !slices.ContainsFunc(decl.fields, func(f field) bool { return f.json == condition.field }) {
					return nil, fmt.Errorf("%s: %s rule names unknown field %q", decl.file.name, decl.name, condition.field)
				}
			}
		}
	}
	return m, nil
}

func (m *model) checkRef(ref typeRef) error {
	switch ref.kind {
	case arrayRef:
		return m.checkRef(*ref.elem)
	case nullableRef:
		decl, ok := m.types[ref.elem.name]
		if ref.elem.kind != namedRef || !ok || decl.kind != structDecl {
			return fmt.Errorf("pointers must name a generated struct")
		}
	case namedRef:
		if _, ok := m.types[ref.name]; !ok {
			return fmt.Errorf("%s is not declared in a generated file", ref.name)
		}
	}
	return nil
}

func parseType(files *token.FileSet, source *sourceFile, spec *ast.TypeSpec, doc *ast.CommentGroup) (*typeDecl, error) {
	at := files.Position(spec.Pos()).String()
	if !spec.Name.IsExported() || spec.TypeParams != nil || spec.Assign.IsValid() {
		return nil, fmt.Errorf("%s: generated types are exported, non-generic, and not aliases", at)
	}
	decl := &typeDecl{name: spec.Name.Name, file: source, doc: docLines(doc)}
	if doc != nil {
		for _, comment := range doc.List {
			if !strings.HasPrefix(comment.Text, "//contract:") {
				continue
			}
			parsed, err := parseRule(comment.Text)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", files.Position(comment.Pos()), err)
			}
			decl.rules = append(decl.rules, parsed)
		}
	}
	switch underlying := spec.Type.(type) {
	case *ast.Ident:
		if underlying.Name != "string" {
			return nil, fmt.Errorf("%s: named non-struct types must be string enums", at)
		}
		decl.kind = enumDecl
		if len(decl.rules) > 0 {
			return nil, fmt.Errorf("%s: rules belong to structs", at)
		}
	case *ast.StructType:
		decl.kind = structDecl
		seen := map[string]bool{}
		for _, item := range underlying.Fields.List {
			fieldAt := files.Position(item.Pos()).String()
			if len(item.Names) != 1 || !item.Names[0].IsExported() {
				return nil, fmt.Errorf("%s: generated fields are named, exported, and one per line", fieldAt)
			}
			if item.Tag == nil {
				return nil, fmt.Errorf("%s: %s needs a json tag", fieldAt, item.Names[0].Name)
			}
			tag, err := strconv.Unquote(item.Tag.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fieldAt, err)
			}
			name, options, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				return nil, fmt.Errorf("%s: %s needs a json name", fieldAt, item.Names[0].Name)
			}
			if seen[name] {
				return nil, fmt.Errorf("%s: json name %q repeats", fieldAt, name)
			}
			seen[name] = true
			ref, err := parseRef(item.Type)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fieldAt, err)
			}
			optional := options == "omitempty"
			if options != "" && !optional {
				return nil, fmt.Errorf("%s: json option %q is unsupported", fieldAt, options)
			}
			if optional && ref.kind != stringRef {
				return nil, fmt.Errorf("%s: omitempty is supported on strings only", fieldAt)
			}
			doc := docLines(item.Doc)
			// Docs name the field as TypeScript does.
			if len(doc) > 0 {
				if rest, ok := strings.CutPrefix(doc[0], item.Names[0].Name+" "); ok {
					doc[0] = name + " " + rest
				} else if rest, ok := strings.CutPrefix(doc[0], item.Names[0].Name+":"); ok {
					doc[0] = name + ":" + rest
				}
			}
			decl.fields = append(decl.fields, field{json: name, optional: optional, ref: ref, doc: doc})
		}
	default:
		return nil, fmt.Errorf("%s: generated types are structs or string enums", at)
	}
	return decl, nil
}

func parseRef(expression ast.Expr) (typeRef, error) {
	switch typed := expression.(type) {
	case *ast.Ident:
		switch typed.Name {
		case "string":
			return typeRef{kind: stringRef}, nil
		case "bool":
			return typeRef{kind: booleanRef}, nil
		case "int", "int32", "int64", "uint", "uint32", "uint64", "float64":
			return typeRef{kind: numberRef}, nil
		}
		if !typed.IsExported() {
			return typeRef{}, fmt.Errorf("type %s is unsupported", typed.Name)
		}
		return typeRef{kind: namedRef, name: typed.Name}, nil
	case *ast.ArrayType:
		if typed.Len != nil {
			return typeRef{}, fmt.Errorf("fixed-size arrays are unsupported")
		}
		elem, err := parseRef(typed.Elt)
		if err != nil {
			return typeRef{}, err
		}
		return typeRef{kind: arrayRef, elem: &elem}, nil
	case *ast.StarExpr:
		elem, err := parseRef(typed.X)
		if err != nil {
			return typeRef{}, err
		}
		return typeRef{kind: nullableRef, elem: &elem}, nil
	}
	return typeRef{}, fmt.Errorf("field type %T is unsupported", expression)
}

func parseRule(text string) (rule, error) {
	match := ruleLine.FindStringSubmatch(text)
	if match == nil {
		return rule{}, fmt.Errorf("want //contract:rule <field> ==|!= <literal> => <field> ==|!= <literal>, got %q", text)
	}
	when, err := newCondition(match[1], match[2], match[3])
	if err != nil {
		return rule{}, err
	}
	then, err := newCondition(match[4], match[5], match[6])
	if err != nil {
		return rule{}, err
	}
	return rule{when: when, then: then}, nil
}

func newCondition(fieldName, operator, literal string) (condition, error) {
	var value any
	if err := json.Unmarshal([]byte(literal), &value); err != nil {
		return condition{}, fmt.Errorf("rule literal %s is not JSON: %w", literal, err)
	}
	switch value.(type) {
	case nil, bool, float64, string:
	default:
		return condition{}, fmt.Errorf("rule literal %s is not a scalar", literal)
	}
	return condition{field: fieldName, equal: operator == "==", literal: literal}, nil
}

// docLines is a comment's text lines without directives or trailing blanks.
func docLines(group *ast.CommentGroup) []string {
	if group == nil {
		return nil
	}
	text := strings.TrimRight(group.Text(), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
