package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// validate checks a decoded JSON value against a generated type as the
// TypeScript guards do, and also rejects unknown object keys.
func (m *model) validate(typeName string, value any) error {
	return m.validateRef(typeRef{kind: namedRef, name: typeName}, value, "$")
}

func (m *model) validateRef(ref typeRef, value any, at string) error {
	switch ref.kind {
	case stringRef:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: want a string", at)
		}
	case booleanRef:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: want a boolean", at)
		}
	case numberRef:
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s: want a number", at)
		}
	case arrayRef:
		entries, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: want an array", at)
		}
		for i, entry := range entries {
			if err := m.validateRef(*ref.elem, entry, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	case nullableRef:
		if value == nil {
			return nil
		}
		return m.validateRef(*ref.elem, value, at)
	case namedRef:
		return m.validateDecl(m.types[ref.name], value, at)
	}
	return nil
}

func (m *model) validateDecl(decl *typeDecl, value any, at string) error {
	if decl.kind == enumDecl {
		text, ok := value.(string)
		if !ok || !slices.Contains(decl.values, text) {
			return fmt.Errorf("%s: want one of %q", at, decl.values)
		}
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: want a %s object", at, decl.name)
	}
	for key := range object {
		if !slices.ContainsFunc(decl.fields, func(f field) bool { return f.json == key }) {
			return fmt.Errorf("%s.%s: %s has no such field", at, key, decl.name)
		}
	}
	for _, field := range decl.fields {
		entry, present := object[field.json]
		if !present {
			if field.optional {
				continue
			}
			return fmt.Errorf("%s.%s: missing", at, field.json)
		}
		if err := m.validateRef(field.ref, entry, at+"."+field.json); err != nil {
			return err
		}
	}
	// An absent optional string is "", as Go omits it.
	compared := object
	for _, field := range decl.fields {
		if _, present := object[field.json]; !present && field.optional && field.ref.kind == stringRef {
			compared = maps.Clone(compared)
			compared[field.json] = ""
		}
	}
	for _, rule := range decl.rules {
		if rule.when.holds(compared) && !rule.then.holds(compared) {
			return fmt.Errorf("%s: %s breaks rule %s => %s", at, decl.name, rule.when, rule.then)
		}
	}
	return nil
}

// holds compares the object's field with the literal as JSON values.
func (c condition) holds(object map[string]any) bool {
	var literal any
	_ = json.Unmarshal([]byte(c.literal), &literal)
	equal := object[c.field] == literal
	return equal == c.equal
}

func (c condition) String() string {
	operator := "=="
	if !c.equal {
		operator = "!="
	}
	return c.field + " " + operator + " " + c.literal
}
