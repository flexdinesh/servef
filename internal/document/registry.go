package document

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Type describes a file family and the views the UI can offer. Source is always
// available. An empty PreviewKind means source-only support; extensions include
// their leading dot. IDs and extensions are case-insensitive CLI selectors.
type Type struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Extensions   []string `json:"extensions"`
	Language     string   `json:"language"`
	PreviewKind  string   `json:"previewKind"`
	PreviewLabel string   `json:"previewLabel"`
	DefaultMode  string   `json:"defaultMode"`
}

// Context supplies navigation policy without giving handlers filesystem access.
type Context struct {
	Path     string
	Supports func(string) bool
}

// Handler owns presentation and searchable text, never filesystem access.
// Preview returns escaped HTML, or an error without making source unavailable.
// Source-only handlers return empty HTML. SearchText must also handle malformed
// input so a preview failure does not remove a file from search.
type Handler interface {
	Type() Type
	Preview(source []byte, context Context) (string, error)
	SearchText(source []byte) (string, error)
}

// Registry is immutable after construction. The same selected registry governs
// discovery, direct document access, link navigation, and content search.
type Registry struct {
	handlers    []Handler
	byExtension map[string]Handler
}

func NewRegistry(handlers ...Handler) (*Registry, error) {
	registry := &Registry{handlers: append([]Handler{}, handlers...), byExtension: make(map[string]Handler)}
	ids := make(map[string]bool)
	names := make(map[string]string)
	for _, handler := range handlers {
		if handler == nil {
			return nil, fmt.Errorf("file handler is required")
		}
		definition := handler.Type()
		if definition.ID == "" || definition.ID != strings.ToLower(definition.ID) || ids[definition.ID] || definition.Label == "" || len(definition.Extensions) == 0 {
			return nil, fmt.Errorf("invalid or duplicate file type %q", definition.ID)
		}
		if definition.PreviewKind != "" && definition.PreviewKind != "markdown" && definition.PreviewKind != "code" {
			return nil, fmt.Errorf("invalid preview kind for %q", definition.ID)
		}
		if (definition.DefaultMode != "source" && definition.DefaultMode != "preview") || (definition.PreviewKind == "" && definition.DefaultMode != "source") || (definition.PreviewKind != "" && definition.PreviewLabel == "") {
			return nil, fmt.Errorf("invalid view policy for %q", definition.ID)
		}
		ids[definition.ID] = true
		for _, selector := range append([]string{definition.ID}, definition.Extensions...) {
			name := strings.TrimPrefix(selector, ".")
			if owner, ok := names[name]; ok && owner != definition.ID {
				return nil, fmt.Errorf("ambiguous file type selector %q", name)
			}
			names[name] = definition.ID
		}
		for _, extension := range definition.Extensions {
			if !strings.HasPrefix(extension, ".") || len(extension) < 2 || extension != strings.ToLower(extension) || registry.byExtension[extension] != nil {
				return nil, fmt.Errorf("invalid or duplicate extension %q", extension)
			}
			registry.byExtension[extension] = handler
		}
	}
	return registry, nil
}

// Builtins is the explicit composition point for supported formats. Adding a
// handler here exposes its metadata and behavior through every system boundary.
func Builtins() *Registry {
	registry, err := NewRegistry(newMarkdown(), NewJSON(), NewYAML())
	if err != nil {
		// Static built-in definitions are programmer-owned.
		panic(err)
	}
	return registry
}

func (r *Registry) Lookup(name string) (Handler, bool) {
	handler, ok := r.byExtension[strings.ToLower(filepath.Ext(name))]
	return handler, ok
}

func (r *Registry) Supports(name string) bool { _, ok := r.Lookup(name); return ok }

func (r *Registry) Types() []Type {
	definitions := make([]Type, 0, len(r.handlers))
	for _, handler := range r.handlers {
		definition := handler.Type()
		definition.Extensions = append([]string{}, definition.Extensions...)
		definitions = append(definitions, definition)
	}
	return definitions
}

// Select narrows the registry by family ID or extension (md, .md, markdown).
// No selectors means all registered types. Unsupported selectors fail early.
func (r *Registry) Select(selectors []string) (*Registry, error) {
	if len(selectors) == 0 {
		return r, nil
	}
	selected := make(map[string]bool)
	for _, selector := range selectors {
		name := strings.ToLower(strings.TrimSpace(selector))
		found := false
		for _, definition := range r.Types() {
			if name == definition.ID {
				selected[definition.ID] = true
				found = true
			}
			for _, extension := range definition.Extensions {
				if strings.TrimPrefix(name, ".") == strings.TrimPrefix(extension, ".") {
					selected[definition.ID] = true
					found = true
				}
			}
		}
		if !found {
			names := make([]string, 0, len(r.handlers))
			for _, definition := range r.Types() {
				names = append(names, definition.ID)
			}
			return nil, fmt.Errorf("unknown file type %q (supported: %s)", selector, strings.Join(names, ", "))
		}
	}
	handlers := make([]Handler, 0, len(selected))
	for _, handler := range r.handlers {
		if !selected[handler.Type().ID] {
			continue
		}
		handlers = append(handlers, handler)
	}
	return NewRegistry(handlers...)
}
