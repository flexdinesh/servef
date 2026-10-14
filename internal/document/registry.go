package document

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
)

// Type describes a file family and the views the UI can offer. Source is always
// available. An empty PreviewKind means no preview; extensions include their
// leading dot; filenames are exact base names. Matching is case-insensitive.
// Formattable is derived by the registry from the Formatter capability;
// handlers leave it unset.
type Type struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Extensions   []string `json:"extensions"`
	Filenames    []string `json:"filenames"`
	Language     string   `json:"language"`
	PreviewKind  string   `json:"previewKind"`
	PreviewLabel string   `json:"previewLabel"`
	DefaultMode  string   `json:"defaultMode"`
	Formattable  bool     `json:"formattable"`
}

// HasTextSource reports whether files of this type have a text source view.
// Raster images (image previews without a language) are binary regardless of
// their bytes.
func (t Type) HasTextSource() bool { return t.PreviewKind != PreviewImage || t.Language != "" }

// Preview kinds rendered by the UI. Image previews use the file's raw bytes.
const (
	PreviewMarkdown = "markdown"
	PreviewImage    = "image"
)

// Context supplies navigation policy without giving handlers filesystem access.
// Project scopes rewritten links to the current project; Supports reports
// whether a link target names an in-scope file.
type Context struct {
	Path     string
	Project  string
	Supports func(string) bool
}

// Handler owns presentation and searchable text, never filesystem access.
// Preview returns escaped HTML, or an error without making source unavailable.
// Handlers without an HTML preview return empty HTML. SearchText must also
// handle malformed input so a preview failure does not remove a file from search.
type Handler interface {
	Type() Type
	Preview(source []byte, context Context) (string, error)
	SearchText(source []byte) (string, error)
}

// Formatter is an optional Handler capability for a read-only formatted view.
// Errors leave the source view available.
type Formatter interface {
	Format(source []byte) ([]byte, error)
}

// ErrNotFormattable is returned when formatting a family without a Formatter.
var ErrNotFormattable = errors.New("file type is not formattable")

// Family is a registered handler with registry-derived capabilities.
type Family struct {
	handler    Handler
	definition Type
	formatter  Formatter
}

// Type returns the family metadata, including derived capabilities.
func (f Family) Type() Type { return cloneType(f.definition) }

func (f Family) Preview(source []byte, context Context) (string, error) {
	return f.handler.Preview(source, context)
}

func (f Family) SearchText(source []byte) (string, error) { return f.handler.SearchText(source) }

// FormatLines formats a copy of source and returns escaped, highlighted HTML
// per formatted line.
func (f Family) FormatLines(source []byte) ([]string, error) {
	if f.formatter == nil {
		return nil, ErrNotFormattable
	}
	formatted, err := f.formatter.Format(append([]byte{}, source...))
	if err != nil {
		return nil, err
	}
	return HighlightLines(formatted, f.definition.Language), nil
}

// Registry is immutable after construction. It decides how a file is shown,
// never which files are in scope.
type Registry struct {
	families    []Family
	fallback    Family
	byFilename  map[string]Family
	byExtension map[string]Family
}

// NewRegistry composes a source-only fallback family for unregistered files
// with additional handlers.
func NewRegistry(fallback Handler, handlers ...Handler) (*Registry, error) {
	registry := &Registry{byFilename: make(map[string]Family), byExtension: make(map[string]Family)}
	ids := make(map[string]bool)
	names := make(map[string]string)
	for index, handler := range append([]Handler{fallback}, handlers...) {
		if handler == nil {
			return nil, errors.New("file handler is required")
		}
		family, err := newFamily(handler)
		if err != nil {
			return nil, err
		}
		definition := family.definition
		if ids[definition.ID] {
			return nil, fmt.Errorf("duplicate file type %q", definition.ID)
		}
		ids[definition.ID] = true
		if index == 0 {
			if definition.PreviewKind != "" || definition.Formattable {
				return nil, fmt.Errorf("fallback file type %q must be source-only", definition.ID)
			}
			registry.fallback = family
		} else if len(definition.Extensions) == 0 && len(definition.Filenames) == 0 {
			return nil, fmt.Errorf("file type %q needs an extension or filename", definition.ID)
		}
		// IDs, extensions, and filenames share one case-insensitive namespace so
		// each name belongs to exactly one family.
		selectors := append(append([]string{definition.ID}, definition.Extensions...), definition.Filenames...)
		for _, selector := range selectors {
			name := strings.TrimPrefix(strings.ToLower(selector), ".")
			if owner, ok := names[name]; ok && owner != definition.ID {
				return nil, fmt.Errorf("ambiguous file type selector %q", selector)
			}
			names[name] = definition.ID
		}
		for _, extension := range definition.Extensions {
			if !strings.HasPrefix(extension, ".") || len(extension) < 2 || extension != strings.ToLower(extension) || strings.ContainsAny(extension, "/\\") || strings.Count(extension, ".") != 1 {
				return nil, fmt.Errorf("invalid extension %q", extension)
			}
			if _, ok := registry.byExtension[extension]; ok {
				return nil, fmt.Errorf("duplicate extension %q", extension)
			}
			registry.byExtension[extension] = family
		}
		for _, filename := range definition.Filenames {
			if filename == "" || filename == "." || filename == ".." || strings.ContainsAny(filename, "/\\") {
				return nil, fmt.Errorf("invalid filename %q", filename)
			}
			folded := strings.ToLower(filename)
			if _, ok := registry.byFilename[folded]; ok {
				return nil, fmt.Errorf("duplicate filename %q", filename)
			}
			registry.byFilename[folded] = family
		}
		registry.families = append(registry.families, family)
	}
	return registry, nil
}

func newFamily(handler Handler) (Family, error) {
	definition := cloneType(handler.Type())
	formatter, formattable := handler.(Formatter)
	if definition.ID == "" || definition.ID != strings.ToLower(definition.ID) || definition.Label == "" {
		return Family{}, fmt.Errorf("invalid file type %q", definition.ID)
	}
	if definition.Formattable && !formattable {
		return Family{}, fmt.Errorf("file type %q declares formatting without a Formatter", definition.ID)
	}
	definition.Formattable = formattable
	switch definition.PreviewKind {
	case "", PreviewMarkdown, PreviewImage:
	default:
		return Family{}, fmt.Errorf("invalid preview kind for %q", definition.ID)
	}
	if (definition.DefaultMode != "source" && definition.DefaultMode != "preview") ||
		(definition.PreviewKind == "" && definition.DefaultMode != "source") ||
		(definition.PreviewKind != "" && definition.PreviewLabel == "") ||
		(definition.PreviewKind != "" && definition.Formattable) {
		return Family{}, fmt.Errorf("invalid view policy for %q", definition.ID)
	}
	// Chroma only highlights: each family names its lexer explicitly. Raster
	// images have no text source and may omit a language.
	if definition.Language == "" {
		if definition.PreviewKind != PreviewImage {
			return Family{}, fmt.Errorf("file type %q needs a language", definition.ID)
		}
	} else if lexers.Get(definition.Language) == nil {
		return Family{}, fmt.Errorf("unknown highlight language %q for %q", definition.Language, definition.ID)
	}
	family := Family{handler: handler, definition: definition}
	if formattable {
		family.formatter = formatter
	}
	return family, nil
}

func cloneType(definition Type) Type {
	definition.Extensions = append([]string{}, definition.Extensions...)
	definition.Filenames = append([]string{}, definition.Filenames...)
	return definition
}

// Lookup returns the family for a file: exact base name, then extension, then
// the fallback. Every file has a family.
func (r *Registry) Lookup(name string) Family {
	base := strings.ToLower(path.Base(name))
	if family, ok := r.byFilename[base]; ok {
		return family
	}
	if family, ok := r.byExtension[path.Ext(base)]; ok {
		return family
	}
	return r.fallback
}

// Types lists registered families, fallback first.
func (r *Registry) Types() []Type {
	definitions := make([]Type, 0, len(r.families))
	for _, family := range r.families {
		definitions = append(definitions, family.Type())
	}
	return definitions
}
