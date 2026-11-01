# Architecture

`servef` is one product implemented by two build-time projects and distributed
as one Go binary.

## System boundaries

- `main.go` and `cmd/`: CLI, process lifecycle, listener, browser launch.
- `internal/files/`: file discovery, filename-glob scope, and safe path resolution.
- `internal/projects/`: root validation, repository/worktree discovery, project resolution.
- `internal/git/`: go-git reads for Source Control; go-git types stay inside it.
- `internal/document/`: file families, previews, formatting, media types, binary/size policy.
- `internal/lsp/`: language-server data, process lifecycle, JSON-RPC client.
- `internal/control/`: authenticated discovery and shutdown of local servef processes.
- `internal/browser/`, `internal/version/`: browser launch and build version.
- `internal/web/`: HTTP routes (`routes.go`), indexed reads, shell/static delivery, embedded assets.
- `web/`: React UI, Vite build, frontend tests and development tooling.
- `testdata/markdown/`: canonical filesystem corpus for Go and browser tests.
- `testdata/api/`: transport scenarios for frontend-only development.
- `docs/decisions/`: decision records for scope, file types, views, and limits.
- `site/`: static Vocs user docs; build-time only, not part of the binary.
- `tests/browser/`: browser tests across the Go/React boundary.

The filesystem root passed to the CLI is the primary runtime trust boundary.
The server indexes paths matching `--filename` and resolves requests only through
that index. It binds localhost by default and selects the first free port from
7971 through 7980; exposing it requires an explicit `--host` IP address.
Requests must name `localhost` or an IP literal in `Host`; other hostnames get
403, which blocks DNS rebinding.

## Dependency direction

```text
CLI -> web -> document, files, git, lsp, projects
git, projects -> files
React -> HTTP API -> HTTP application
web source -> Vite -> internal/web/dist -> go:embed -> servef binary
```

Go and pnpm retain their native dependency graphs. The repository-level
`package.json` is a command facade; `web/package.json` owns browser dependencies.
Add another Go module or pnpm package only for an independently consumed unit.

## Development and tests

`mise run check:pr` validates an immutable committed snapshot before separate PR
creation or updates. `tools/validate/` owns snapshot selection, host static checks,
certificates, resource admission, cancellation, and artifact retention. `.dagger/`
owns all test execution, prerequisite builds, and immutable container preparation.
Host static checks use private worktree-local state; development and formatting
remain host operations. Public mise tasks orchestrate, and hidden host/container
leaves never call public wrappers. This tooling is independent of the distributed
application. See [validation](validation.md).

- `mise run dev`: Go over `testdata/markdown` plus Vite with API proxying.
- `mise run dev:mock`: Vite plus committed JSON fixtures; no Go process.
- `mise run test:go`: Go unit and HTTP integration behavior in Dagger.
- `mise run test:web`: frontend logic and fixture-contract behavior in Dagger.
- `mise run test:browser`: production bootstrap and end-to-end behavior.

Targeted tests capture current edits without certifying PR readiness. Complete
committed coverage is keyed by relevant inputs; history-only rebases can reuse
it. An active rerun invalidates prior success until a new successful result is
published. `check:status` verifies coverage without running tests or starting an
engine. No local convention provides remote enforcement.

## Distribution

Vite writes hashed assets to `internal/web/dist`, which `main` ignores. Release
tags and the CI-published `edge` branch commit it on top of `main`, so
`go install github.com/flexdinesh/servef@latest` (or `@edge`) needs no
JavaScript toolchain. Installed releases contain no Node.js runtime or frontend
sidecar.

## Growth rules

1. Organize Go around capabilities before adding abstraction layers.
2. Keep HTTP DTOs at the transport boundary; move document/search behavior out
   of `internal/web` as those capabilities grow.
3. Keep browser contracts runtime-validated. Introduce OpenAPI and generated
   client types when manual contract maintenance becomes material.
4. Create shared frontend packages only after a second consumer exists.
5. Add task-graph caching only when multiple projects or CI timings justify it.
