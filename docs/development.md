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
| Run non-browser checks and the docs build | `mise run check` |
| Install browser-test dependencies | `pnpm exec playwright install --with-deps chromium` |
| Run browser tests | `mise run test:browser` |
| Run browser tests on explicit ports | `PORT=18090 SERVEF_BROWSER_WEB_PORT=15183 mise run test:browser` |
| Run pre-push checks | `mise run check:push` |
| Run full checks, including race tests and docs build | `mise run check:full` |

`mise run check` builds the embedded frontend once, then runs Go formatting,
tidiness, vet, tests, mise config validation, web and tooling tests,
typecheck, lint, and the docs build in parallel. `build:web` and `build:site`
declare their inputs, so mise skips them when none changed;
`mise run --force build:web` (or `build:site`) rebuilds anyway.

`check`, `check:push`, and `check:full` run `scripts/check.ts`, which takes
`:::`-separated groups of mise tasks from `mise.toml`. Groups run in order;
each group runs its tasks in parallel, up to `MISE_JOBS` (default 8). Each
task's stdout and stderr go to `.dev/check-logs/NN-<task>.log`, cleared at the
start of each run, and the terminal shows one `ok <task> 4.2s` line per task.
A failed task prints its last 100 log lines, the log path, and the
`mise run <task>` command that reruns it alone. The rest of its group
finishes, later groups print `skip`, and the run exits with the first failing
status. Set `SERVEF_CHECK_VERBOSE=1` to stream task output instead of writing
logs. Node tests use the dot reporter, Playwright uses dots unless stdout is a
terminal, and the app build prints only warnings and errors (decision 0057).

`mise run test:browser` builds the embedded frontend first, since some specs load
the Go-served UI. The runner compiles one Go binary, starts three API servers
and Vite on OS-assigned ports, and runs Chromium with two workers. Each run
owns its fixtures, cache, and binary in a temporary directory and cleans them
up on exit. `PORT` selects the main API port, with git and live-update servers
on `PORT+2` and `PORT+3`; `SERVEF_BROWSER_WEB_PORT` selects Vite's port. Unset
or zero means automatic. `mise run test:browser -- --workers=1` reduces
Chromium concurrency further.

Checks run locally. Outside CI, `pnpm install` points Git at `.githooks/`, whose
`pre-push` hook runs `mise run check:push`: formatting, module tidiness, mise
validation, vet, ordinary Go tests, web/tooling tests, typechecks, and lint,
then critical browser tests. Go tests already compile every package, so no
duplicate Go build runs. Race tests and the docs production build move to
`mise run check:full`; race tests need a C compiler. CI only publishes embedded
assets on `edge`; release builds packages and smoke-tests the shipped binary.
Skip the hook once with `git push --no-verify`. The hook is the full-check
gate: do not also run `check:push` by hand before pushing.

On Linux and macOS, `check:push` and `check:full` hold an OS-backed lock in
`git rev-parse --git-common-dir` for the complete run. All linked worktrees
wait on the same lock; independent clones and dev servers do not. The waiting
message means another worktree is checking. Cancellation/exit releases the
lock after its inheriting check processes exit; the empty lock file stays and
must not be deleted. This is mutual exclusion, not a FIFO queue. Set
`GOMAXPROCS` or `MISE_JOBS` to override their conservative default of two;
Node test files and Chromium also default to two concurrent workers.

Direct individual test commands do not acquire the shared lock. Prefer the
queued commands while other agents are checking. Browser coverage decisions
and intentionally omitted cases are recorded in [the test audit](browser-test-audit.md).

On Linux with unprivileged user namespaces, `mise run test:browser` runs Playwright,
its servers, and Chromium in a private loopback-only network namespace. Host
interface changes, such as Docker creating container links, otherwise make
Chromium abort in-flight requests with `ERR_NETWORK_CHANGED` and leave pages
blank. The namespace also keeps test ports from colliding with host servers.
Elsewhere the runner uses ordinary networking with the same automatic ports.
Run browser tests through `mise run test:browser`; direct Playwright execution
does not start servers (`pnpm exec playwright test --list` still lists cases).

`mise run install` rebuilds the embedded frontend when needed and installs the
current checkout with `go install`.

Go builds, tests, and vet need `internal/web/dist/`; mise tasks that need it
build it first. Before running `go` directly in a fresh clone or worktree, run
`mise run build:web`.

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
