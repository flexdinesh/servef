# servef

Browse Markdown, JSON, and YAML files from a local directory.

## Features

- Render Markdown content and local images
- Switch each open document between preview and highlighted, read-only source
- Format JSON and YAML for reading; original files stay unchanged
- Filter the tree by file type; restrict available types through the CLI
- Navigate files in a compact, resizable tree
- Keep visited documents in an editor-style tab bar
- Filter file names and paths with Pierre Trees (Cmd/Ctrl+P; Cmd/Ctrl+K alias)
- Search document content with Cmd/Ctrl+Shift+F or Find in files in the status bar
- Show document details in the status bar; disclose process CPU/RAM from Ready
- Switch between system, light, dark, and editor palettes
- Render Mermaid diagrams with Excalidraw

## Install

### Homebrew

```sh
brew install flexdinesh/tap/servef
```

### Go

Requires Go 1.24 or later.

Latest stable release:

```sh
go install github.com/flexdinesh/servef@latest
```

Specific stable version:

```sh
go install github.com/flexdinesh/servef@v0.1.3
```

All Go installs include the embedded frontend; Node.js and pnpm are not required.

## Usage

Serve the current directory:

```sh
servef .
```

Or serve another directory:

```sh
servef path/to/markdown
```

Without `--port`, servef uses the first free port from `7971` through `7980`.
Use `--host` to bind a specific IP address or `--port` to select an exact port.
If the range is full, servef can stop all verified instances after confirmation.

By default (`--file-type "all"`, implicit), all explicitly supported types are available: Markdown (`.md`,
`.markdown`), JSON (`.json`), and YAML (`.yaml`, `.yml`). Other file types are
excluded.

Restrict the session to Markdown:

```sh
servef . --file-type "md"
```

Include multiple families in one comma-separated argument:

```sh
servef . --file-type "md,json,yaml"
```

Selectors accept family names or extensions, case-insensitively (`MD`, `.md`,
`markdown`, `yml`). Selecting an extension includes its whole family. Unknown
selectors and empty list entries fail before starting the server. Use `all` alone.

The CLI scope applies to discovery, direct document access, and content search.
The explorer filter only changes tree visibility, preserving open documents.
A single allowed type disables the filter and shows “Set by CLI.” Global content
search covers every CLI-allowed file, regardless of the tree filter. Markdown
image dependencies remain available within the served root.

JSON/YAML open in Formatted mode. Source retains original text. Invalid files
remain searchable and readable in Source, with a formatting error in preview.
YAML formatting retains comments, anchors/aliases, scalar styles, and all documents;
spacing and comment placement can change in the formatted view.

See all options:

```sh
servef --help
```

## Adding a file type

File support is composed in `internal/document.Builtins()`:

- `internal/files` owns directory traversal, exclusions, depth, and safe path
  resolution. It receives an explicit matcher and knows no file formats.
- `internal/document` owns the registry, type metadata, escaped highlighting,
  previews, and searchable text. Handlers receive bytes and navigation context;
  they never read the filesystem. Markdown's parser and link/image rewriting
  live here, separate from HTTP.
- `internal/web` reads indexed files and transports results. React uses the
  server's type IDs and view capabilities for filters, labels, and default modes.
  No frontend extension list is needed.

Implement `document.Handler` for a new specialized preview, then register it in
`Builtins()`. Preview errors must preserve source access; `SearchText` should work
on malformed input. Return escaped HTML using the existing preview conventions.
The built-in renderers use locally embedded assets.

For future source-only languages, register a `CodeHandler` with no formatter:

```go
document.CodeHandler{
    Definition: document.Type{
        ID: "go", Label: "Go", Extensions: []string{".go"},
        Language: "go", DefaultMode: "source",
    },
}
```

That registration enables discovery, CLI selection, filtering, highlighted source,
and raw-text search. An empty `PreviewKind` hides the view toggle. For formatted
text previews, use `CodeHandler.Format` and declare `PreviewKind: "code"`,
`PreviewLabel: "Formatted"`, and the desired default mode. Highlighting uses the
bundled Chroma lexer named by `Language`, with escaped plain-text fallback.

Add behavior tests for family detection/aliases, source preservation, preview
failure, search, and the API contract. JSON/YAML fixtures under
`testdata/markdown/config` exercise the same development and browser workflows
as Markdown.
