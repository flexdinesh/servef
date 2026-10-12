# servef

Browse any file in a local directory: rendered Markdown, image previews,
formatted JSON/YAML, and highlighted source for everything else.

## Features

- Render Markdown content and local images
- Preview PNG, JPEG, GIF, WebP, AVIF, ICO, BMP, and SVG images
- Switch each open document between preview and highlighted, read-only source
- Format JSON and YAML on demand for reading; original files stay unchanged
- Highlight common languages and config files; show other text as plain text
- Filter the tree by filename glob; restrict served files through the CLI
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

By default (`--filename "*"`, implicit), every file is served, whatever its
type. Restrict the session with one comma-separated list of glob patterns:

```sh
servef . --filename '*.md'
servef . --filename '*.md,*.go,Dockerfile'
```

Patterns use Go `path.Match` syntax (`*`, `?`, `[...]`) and match each file's
base name case-insensitively. Empty entries and malformed patterns fail before
starting the server. Directory exclusions (`.git`, `node_modules`, `vendor`, and
`--exclude`) still apply.

The CLI scope applies to discovery, direct document access, Markdown link
navigation, and content search. The explorer's filename filter accepts the same
patterns and only changes tree visibility, preserving open documents. Global
content search covers every in-scope text file regardless of the tree filter.
Markdown image dependencies remain available within the served root.

How a file is shown depends on its type, not on the scope:

- Markdown and images open in Preview; SVG also has Source.
- JSON/YAML open in Source; Format shows a formatted, highlighted view on
  demand. Invalid files stay readable and searchable in Source.
  YAML formatting retains comments, anchors/aliases, scalar styles, and all
  documents; spacing and comment placement can change in the formatted view.
- Known languages and config files show highlighted source. Unknown text files
  show plain text.
- Binary files (a NUL byte in the first 8000 bytes) other than images show a
  "Binary file" note and are excluded from content search.
- Files over 4 MiB show plain, unhighlighted source without preview or
  formatting; image previews stay available. Files over 8 MiB are also
  excluded from content search.
- Non-image files over 32 MiB are not read; they show a "Too large to display"
  note.

Markdown Preview hides leading frontmatter enclosed by matching `---` (YAML) or
`+++` (TOML) lines, and HTML comments. Source preserves both. Unclosed frontmatter
stays visible; comment syntax inside code stays literal.

See all options:

```sh
servef --help
```

## Adding a file type

Every in-scope file is browsable; registration only decides how it is shown.
Families are composed in `internal/document.Builtins()`:

- `internal/files` owns directory traversal, exclusions, depth, the filename
  glob filter, and safe path resolution. It knows no file formats.
- `internal/document` owns the registry, type metadata, escaped highlighting,
  previews, formatting, binary/size policy, and searchable text. Handlers
  receive bytes and navigation context; they never read the filesystem.
  Markdown's parser and link/image rewriting live here, separate from HTTP.
- `internal/web` reads indexed files and transports results. React uses the
  server's type IDs and view capabilities for labels, toggles, and default
  modes. No frontend extension list is needed.

`Registry.Lookup` resolves an exact base name (`Filenames`), then the extension
(`Extensions`), then the source-only fallback family (`text`, plain text). Names
are case-insensitive, and IDs, extensions, and filenames must each belong to
one family. `Language` must name a bundled Chroma lexer (`plaintext` for plain
text); only raster image families may leave it empty. Chroma only highlights;
it never guesses a file's type.

For a source-only language, register a `CodeHandler`:

```go
document.CodeHandler{
    Definition: document.Type{
        ID: "zig", Label: "Zig", Extensions: []string{".zig"},
        Language: "zig", DefaultMode: "source",
    },
}
```

Use `Filenames: []string{"Dockerfile"}` for files identified by name. That
registration enables highlighted source, tree labels, and raw-text search. An
empty `PreviewKind` hides the view toggle.

For a read-only formatted view, register a `FormattedCodeHandler` with a
`FormatSource` function. Handlers implementing `document.Formatter` are
reported as `formattable`; the UI offers Format beside Source and fetches
`/api/format` on demand. Formatters receive a copy of the bytes and must
preserve meaningful values.

Implement `document.Handler` for a new specialized preview (`PreviewKind`
`markdown` or `image`), then register it in `Builtins()`. Preview errors must
preserve source access; `SearchText` should work on malformed input. Return
escaped HTML using the existing preview conventions. The built-in renderers use
locally embedded assets.

Add behavior tests for lookup (filenames, extensions, fallback), source
preservation, preview/format failure, search, and the API contract. Samples
under `testdata/markdown` (Markdown, JSON/YAML, images, code, a binary file)
exercise the same development and browser workflows.
