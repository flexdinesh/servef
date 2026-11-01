# Development

## Requirements

[mise](https://mise.jdx.dev) 2026.10.18 or later. It installs Go, Node.js, and
pnpm from `mise.lock`, loads `.env`, and runs every composite task (decision
0038). Go comes from `mise.toml`, Node.js from `.node-version`, and pnpm from
`package.json` `packageManager`. Once per clone (worktrees of a trusted clone
are trusted too):

```sh
mise trust
mise install
mise run setup
```

`mise upgrade` moves `mise.lock` to newer matching versions; `mise lock`
refreshes its checksums after editing `[tools]`. When raising `min_version`,
raise `jdx/mise-action` `version` in `.github/workflows/` with it.

## Install from edge

Install the latest `main` with embedded frontend assets:

```sh
go install github.com/flexdinesh/servef@edge
```

The Go install includes the embedded frontend; Node.js and pnpm are not required.

`main` does not track `internal/web/dist/`, so `go install ...@main` fails.
After the frontend build on `main`, CI force-pushes `edge`: that `main` commit plus the
built frontend. `@latest` selects the latest stable version tag. Go module
proxies may briefly cache an older branch revision. To bypass the proxy for
this module:

```sh
GONOPROXY=github.com/flexdinesh/servef go install github.com/flexdinesh/servef@edge
```

## Commands

`mise tasks` lists every task. `pnpm dev`, `pnpm check`, `pnpm test:browser`,
and `pnpm link:local` delegate to the mise tasks below.

| Purpose | Command |
| --- | --- |
| Install dependencies and build frontend assets | `mise run setup` |
| Run Go and Vite together | `mise run dev` |
| Run only the Go server | `mise run dev:server` |
| Run only Vite against the Go server | `mise run dev:web` |
| Publish sample walkthroughs to the dev server | `mise run dev:walkthroughs` |
| Run Vite with API fixtures | `mise run dev:mock` |
| Build embedded frontend assets (skipped when unchanged) | `mise run build:web` |
| Build a local binary | `mise run build:web && go build -o ./bin/servef .` |
| Install the local CLI | `mise run install` |
| Run Go tests | `mise run test:go` |
| Run Go tests with the race detector | `mise run test:race` |
| Run frontend tests | `mise run test:web` |
| Test release tooling | `mise run test:release` |
| Type-check TypeScript | `mise run typecheck` |
| Lint (oxlint: React Hooks, UI primitives; stylelint: design tokens) | `mise run lint` |
| Check Go formatting and module tidiness | `mise run fmt:check`, `mise run tidy:check` |
| Validate everything in local containers | `mise run validate` |
| Install browser-test dependencies | `pnpm exec playwright install --with-deps chromium` |
| Run browser tests | `mise run test:browser` |
| Run selected browser coverage | `mise run validate -- --group browser -- tests/browser/live-updates.spec.ts` |
| Inspect current validation coverage | `mise run validate:status` |
| Run full checks, including race tests and docs build | `mise run check:full` |

Validation uses committed HEAD with a clean tracked tree. Run
`mise run validate` explicitly before PR creation and each PR update; `check`
and `check:full` are aliases. Five isolated local Dagger groups cover Go
(including race), frontend, tooling, docs build, and browsers. Public targeted
check commands use the same isolation. Native commands remain optional editing
checks and do not establish snapshot validation evidence.

See [local validation](validation.md) for explicit engine setup, source/cache
identities, resource limits, force reruns, artifacts, status, and PR wrappers.
There is no mandatory pre-push suite and no remote validation gate. Existing
CI edge publication and release builds remain unchanged.

Browser validation uses a prepared Go binary, ordinary and Git fixture servers,
Vite, and private per-test live-update servers inside one container. Each run
owns its writable fixtures, homes, caches, and outputs. Native browser runs can
still use `mise run build:web` followed by
`pnpm exec node scripts/test-browser.ts`; this optional path uses automatic
ports and conditional Linux namespace protection.

Go builds, tests, and vet need `internal/web/dist/`; the isolated graph supplies
it. Before running Go directly in a fresh worktree, run `mise run build:web`.
Build outputs are ignored and never copied from the host into validation.

## Web development

`mise run dev` starts Go on port `8080` and Vite on port `5173`. `mise run dev:mock` runs Vite without Go and
uses the default API scenario under `testdata/api`.

`mise run dev` and `mise run dev:server` serve `testdata/markdown` and, for
Source Control, the browser tests' git fixture (`tests/browser/git-fixture.ts`).
Each start rebuilds it in the gitignored `.dev/git-fixture`, discarding edits
there; pick it in the project picker. Without git, it is a plain folder.

`mise run dev` also publishes sample walkthroughs to both projects
(`scripts/dev-walkthroughs.ts`) once the server is up; rerun it with
`mise run dev:walkthroughs`. The dev server keeps walkthroughs in the gitignored
`.dev/cache`, not your user cache.

To use other ports, copy `.env.example` to `.env` (gitignored) and set
`SERVEF_DEV_SERVER_PORT` (Go) and `SERVEF_DEV_WEB_PORT` (Vite). Vite proxies
`/api` to the Go port. mise loads `.env` for tasks and activated shells.

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
