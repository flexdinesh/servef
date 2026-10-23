# Development

## Requirements

- Go 1.26 or later
- Node.js 26
- pnpm 11

## Install from edge

Install the latest `main` that passed CI:

```sh
go install github.com/flexdinesh/servef@edge
```

The Go install includes the embedded frontend; Node.js and pnpm are not required.

`main` does not track `internal/web/dist/`, so `go install ...@main` fails.
After CI passes on `main`, CI force-pushes `edge`: that `main` commit plus the
built frontend. `@latest` selects the latest stable version tag. Go module
proxies may briefly cache an older branch revision. To bypass the proxy for
this module:

```sh
GONOPROXY=github.com/flexdinesh/servef go install github.com/flexdinesh/servef@edge
```

## Commands

| Purpose | Command |
| --- | --- |
| Install Go dependencies | `go mod download` |
| Install JavaScript dependencies | `pnpm install` |
| Run Go and Vite together | `pnpm dev` |
| Run only the Go server | `pnpm dev:server` |
| Run only Vite against the Go server | `pnpm dev:web` |
| Run Vite with API fixtures | `pnpm dev:mock` |
| Build embedded frontend assets | `pnpm build:web` |
| Build a local binary | `pnpm build:web && go build -o ./bin/servef .` |
| Install the local CLI | `pnpm link:local` |
| Run Go tests | `go test ./...` |
| Run frontend tests | `pnpm test:web` |
| Test release tooling | `pnpm test:release` |
| Type-check TypeScript | `pnpm typecheck` |
| Run every check CI runs | `pnpm check` |
| Install browser-test dependencies | `pnpm exec playwright install --with-deps chromium` |
| Run browser tests | `pnpm test:browser` |
| Run browser tests on other ports | `PORT=18090 pnpm test:browser` |

`pnpm test:browser` rebuilds the embedded frontend first, since some specs load
the Go-served UI. Browser tests start API servers on `PORT` (default `18080`)
and the next two ports, plus the Vite server on `15173`. Each run builds its
Source Control git fixture in its own temporary folder.

CI runs `pnpm check` only. Browser tests run locally: `pnpm install` points
Git at `.githooks/`, whose `pre-push` hook runs `pnpm check` and
`pnpm test:browser` (decision 0032). Skip it once with `git push --no-verify`.

On Linux with unprivileged user namespaces, `pnpm test:browser` runs Playwright,
its servers, and Chromium in a private loopback-only network namespace. Host
interface changes, such as Docker creating container links, otherwise make
Chromium abort in-flight requests with `ERR_NETWORK_CHANGED` and leave pages
blank. The namespace also keeps test ports from colliding with host servers.
Elsewhere it runs Playwright directly; `pnpm exec playwright test` always does.

`pnpm link:local` rebuilds the embedded frontend and installs the current
checkout with `go install`.

Initial setup:

```sh
go mod download
pnpm install
pnpm build:web
```

Go builds, tests, and vet need `internal/web/dist/`; run `pnpm build:web` in
each fresh clone or worktree first.

## Web development

`pnpm dev` starts Go on port `8080` and Vite on port `5173`. `pnpm dev:mock` runs Vite without Go and
uses the default API scenario under `testdata/api`.

Frontend source and dependencies live in the `web` workspace. Production builds
write to `internal/web/dist`, which is embedded in the Go binary and ignored by
Git.

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
one family. `Language` must name a Highlights language (`text` for plain text);
only raster image families may leave it empty. The browser highlights Source,
Format, Markdown fences, and diffs with it (decision 0029); nothing guesses a
file's type. Add the family and its language to
`testdata/document-families.json`; web tests check the language exists.

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

## Docs site

User docs are a static [Vocs](https://vocs.dev) site in the `site` workspace.
Pages live in `site/src/pages`; navigation lives in `site/vocs.config.ts`.

| Purpose | Command |
| --- | --- |
| Run the docs dev server | `pnpm dev:site` |
| Build the static site | `pnpm build:site` |
| Preview the built site | `pnpm --filter @servef/site preview` |

`pnpm build:site` fails on dead internal links. Static HTML is written to
`site/dist/public`, which Git ignores. The site is not deployed yet.

## Release

See the [release guide](release.md).
