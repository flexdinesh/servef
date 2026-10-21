# servef

Browse any file in a local directory: rendered Markdown, image previews,
formatted JSON/YAML/HTML/CSS/JavaScript, and highlighted source for everything
else.

## Features

- Render Markdown content and local images
- Preview PNG, JPEG, GIF, WebP, AVIF, ICO, BMP, and SVG images
- Switch each open document between preview and highlighted, read-only source
- Format JSON, YAML, HTML, CSS, and JavaScript on demand for reading; original
  files stay unchanged
- Select lines or ranges from the line numbers (drag or Shift-click) to copy
  `path:12-20`; share `#L12-L20` links that open with the range highlighted
- Copy the open file's path from the status bar
- Highlight common languages and config files; show other text as plain text
- Filter the tree by filename glob; restrict served files through the CLI
- Serve several directories at once; switch between the git repositories found
  under them, pick a repository's worktree by branch, or open a folder outside
  any repository, with Cmd/Ctrl+O; each switch reads the file system afresh and
  restores that project's tabs and expanded folders, including after reloads
- Switch between Explorer, Search, and Source Control from a VS Code-style
  activity bar (Cmd/Ctrl+Shift+E, Cmd/Ctrl+Shift+F, Cmd/Ctrl+Shift+G); click
  the active item or press Cmd/Ctrl+B to hide the sidebar
- Review uncommitted changes in Source Control: every modified, added,
  untracked, deleted, or renamed file in the checkout since the last commit,
  with a count badge and Explorer decorations. Each opens as a highlighted
  diff tab with split or unified view, wrap, ignore whitespace, word or
  character inline changes, and expand unchanged. Reads git data directly;
  `git` need not be installed
- Navigate files in a compact, resizable tree
- Browse in rounded navigation and document panels with slim canvas gutters
- Keep visited documents in an editor-style tab bar; right-click a tab to close it,
  other tabs, tabs to its right, or all tabs
- Filter file names and paths with Pierre Trees (Cmd/Ctrl+P; Cmd/Ctrl+K alias)
- Search file content in the Search view (Cmd/Ctrl+Shift+F or Find in files in
  the status bar) like VS Code: every match grouped by file, Match Case, Match
  Whole Word, and regular expressions, files to include/exclude globs, and
  F4/Shift+F4 to step through matches; files git ignores are skipped, including
  global excludes and linked-worktree excludes
- Go to definition (Ctrl/Cmd+click or F12) and find references (Shift+F12)
  through installed language servers, with project-scoped results and VS Code-style
  shortcuts. Servers start on first navigation and stop after 10 idle minutes;
  disable navigation in Settings to prevent new server requests.
- Check LSP availability beside the language in the status bar. Missing servers
  show install commands for the machine running servef, prerequisites, docs,
  and Check again. Browsing works without language servers; navigation reports
  missing or unsupported servers explicitly.
- Show document details in the status bar; disclose process CPU/RAM from Ready
- Choose a theme (system, light, dark, editor palettes) and interface font size
  in Settings (gear in the status bar, Cmd/Ctrl+,); search, browse categories,
  or filter modified preferences. Changes apply immediately and save in the browser
- Run commands from a VS Code-style command palette (Cmd/Ctrl+Shift+P or F1);
  each setting is a command that opens Settings at it
- Render Mermaid diagrams as themed SVG with ondine; expand to zoom and pan

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

Latest `main` that passed CI:

```sh
go install github.com/flexdinesh/servef@edge
```

These installs include the embedded frontend; Node.js and pnpm are not required.
Other branches and commits do not include it and fail to build with
`go install`.

## Usage

Serve the current directory:

```sh
servef .
```

Or serve another directory:

```sh
servef path/to/markdown
```

Serve several directories with a repeatable `--path` (a positional path is
shorthand for one); overlapping directories fail before starting:

```sh
servef --path ~/work --path ~/personal
```

servef looks for git repositories (including linked worktrees) up to
`--scan-depth` folders deep in each directory (default `10`; `0` checks only
the served directories) and lists them in the project picker. It stops
descending at a repository and skips hidden and excluded folders. Only one
project is read at a time, and every switch reads it from disk. `--depth` limits
the file tree within a project.

A repository opens whole: its own folders cannot be opened as projects, and
Open folder only browses folders outside repositories. A repository with
worktrees is one picker entry that opens the last-used worktree; press → (or
click its arrow) to choose a worktree by branch. Worktrees of a served
repository open wherever they live, even outside the served directories.
Repositories are named after their `origin` remote, else their folder. The
explorer header shows the repository and branch; when the repository has
worktrees, click the branch to switch. Linked worktrees get a colored fork icon.

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
Source Control ignores the CLI scope: it lists and diffs every change in the
repository checkout, regardless of `--filename`, `--depth`, and `--exclude`.

How a file is shown depends on its type, not on the scope:

- Markdown opens in Source. Choosing Preview opens other documents in the
  project in Preview until you choose Source, for the rest of the browser tab.
  Images open in Preview; SVG also has Source.
- JSON/YAML open in Source; Format shows a formatted, highlighted view on
  demand. Invalid files stay readable and searchable in Source.
  YAML formatting retains comments, anchors/aliases, scalar styles, and all
  documents; spacing and comment placement can change in the formatted view.
- HTML, CSS, and JavaScript also open in Source with Format on demand. Format
  only reindents and breaks lines: comments, strings, and values are kept
  verbatim, as are `pre`, `textarea`, `script`, `style`, and inline SVG
  content. HTML tag and attribute names show lowercase. JavaScript containing
  JSX or that fails to tokenize keeps Source with a format notice.
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

## Code navigation

Navigation uses installed language servers, enabled by default in Settings.
The first definition or references action starts a server; opening a file or
checking status does not. Servers stop after 10 idle minutes. Results stay
within the browsable project, CLI filename scope, and non-ignored files.
Binary files and source files over 4 MiB do not navigate.

| Language | Server |
| --- | --- |
| Go | gopls |
| JavaScript, JSX, TypeScript, TSX | Native TypeScript 7+ (`tsc --lsp --stdio`, `tsgo`), or typescript-language-server |
| Python | pyright-langserver |
| Rust | rust-analyzer |
| C, C++ | clangd |
| Java | jdtls |
| Kotlin | kotlin-lsp / intellij-server |
| Ruby | ruby-lsp |
| PHP | phpactor |
| Lua | lua-language-server |
| Swift | sourcekit-lsp |
| CSS, SCSS | vscode-css-language-server |

The bottom-right LSP status shows availability for every text language.
Click it for the selected server, errors, runtime prerequisites, documentation,
and install commands tailored to the machine running servef. Copy a command,
install the server in a terminal, then choose **Check again**. Some servers
require a download or a language toolchain; their status links to the install
instructions. Other languages show **LSP unavailable**.

Project-local tools take precedence where supported. Native TypeScript
discovery checks package metadata so a classic `tsc` is never launched as an
LSP. The classic TypeScript adapter requires a tsserver-bearing TypeScript
release; its install recipe uses `typescript@6`. Availability does not imply
every server implements both definitions and references; the status panel
shows negotiated support once running.

Language servers are optional external tools with their own runtime and memory
requirements. servef remains a self-contained binary for browsing. Disabling
Language Servers in Settings prevents this browser from requesting them;
another browser can continue using a shared server until it becomes idle.

## Adding a file type

Every in-scope file is browsable; registration only decides how it is shown.
Families are composed in `internal/document.Builtins()`:

- `internal/files` owns directory traversal, exclusions, depth, the filename
  glob filter, and safe path resolution. It knows no file formats.
- `internal/document` owns the registry, type metadata, escaped highlighting,
  previews, formatting, and binary/size policy. Handlers
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
registration enables highlighted source and tree labels; content search reads
every in-scope text file's original source. An
empty `PreviewKind` hides the view toggle.

For a read-only formatted view, register a `FormattedCodeHandler` with a
`FormatSource` function. Handlers implementing `document.Formatter` are
reported as `formattable`; the UI offers Format beside Source and fetches
`/api/format` on demand. Formatters receive a copy of the bytes and must
preserve meaningful values.

Implement `document.Handler` for a new specialized preview (`PreviewKind`
`markdown` or `image`), then register it in `Builtins()`. Preview errors must
preserve source access. Return
escaped HTML using the existing preview conventions. The built-in renderers use
locally embedded assets.

Add behavior tests for lookup (filenames, extensions, fallback), source
preservation, preview/format failure, search, and the API contract. Samples
under `testdata/markdown` (Markdown, JSON/YAML, HTML/CSS/JS, images, code, a
binary file)
exercise the same development and browser workflows.
