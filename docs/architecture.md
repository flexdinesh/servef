# Architecture

`servef` is one product implemented by build-time projects and distributed as
one Go binary, plus desktop packages that bundle it (decision 0068).

## System boundaries

- `main.go` and `cmd/`: CLI, process lifecycle, listener, browser launch, daemon commands and spawning, opening the desktop app (`servef [path]`), serving no-UI machines on their address with `web:` and `desktop:` links (decision 0076).
- `internal/runtime/`: the web app, HTTP server, and shutdown order shared by every transport; no listener.
- `internal/daemon/`: per-user Unix socket, single-instance lock, `/_daemon/*` control API, roots file, idle exit, client, and `Ensure`; no CLI flags or spawning.
- `internal/files/`: file discovery, always-skipped directories, and safe path resolution.
- `internal/projects/`: root validation, repository/worktree discovery, project resolution.
- `internal/git/`: go-git reads for Source Control; go-git types stay inside it.
- `internal/document/`: file families, previews, formatting, media types, binary/size policy.
- `internal/lsp/`: language-server data, process lifecycle, JSON-RPC client.
- `internal/control/`: authenticated discovery and shutdown of local servef processes.
- `internal/browser/`, `internal/version/`: browser launch and build version.
- `internal/desktopapp/`: desktop app detection (`servef://` handler), opening URLs in it, install hints.
- `internal/web/`: HTTP routes (`routes.go`), indexed reads, shell/static delivery, embedded assets.
- `web/`: React UI, Vite build, frontend tests and development tooling.
- `desktop/`: Electron app (decision 0067); a client of the daemon socket that proxies `servef-app://` to it; never reads project files. Pure modules stay Electron-free.
- `testdata/markdown/`: canonical filesystem corpus for Go and browser tests.
- `testdata/api/`: transport scenarios for frontend-only development.
- `docs/decisions/`: decision records for scope, file types, views, and limits.
- `site/`: static Vocs user docs; build-time only, not part of the binary.
- `tests/browser/`: browser tests across the Go/React boundary.

The filesystem root passed to the CLI is the primary runtime trust boundary.
The server indexes every file within `--depth`, always skipping `.git`,
`node_modules`, and `vendor` directories, and resolves requests only through
that index. It binds localhost by default and selects the first free port from
7971 through 7980; exposing it requires an explicit `--host` IP address.
Requests must name `localhost` or an IP literal in `Host`; other hostnames get
403, which blocks DNS rebinding.

The daemon (decision 0061) has no TCP listener. Its trust boundary is the
socket: 0600 in a 0700 directory owned by the user, so only that user
connects and browsers cannot. Web routes keep the same `Host` check (clients
send `Host: localhost`); `/_daemon/*` control requests never reach the web
app and are refused when they carry `Origin` or `Sec-Fetch-Site`. Clients
open paths (`/_daemon/open`, which may replace served roots inside the path)
and hold a control event stream (`/_daemon/events`) for walkthrough
announcements; project event streams take a `lease` level (decision 0066).
The desktop app proxies renderer requests to the socket, refuses `/_daemon`
paths itself, and adds `Sec-Fetch-Site: same-origin` so proxied requests
never pass as control requests (decision 0067).

ssh connections (decision 0070) reach a host's daemon socket through the
system `ssh`: one app-owned ControlMaster per host, constant remote commands,
and a Unix-socket forward in a private 0700 runtime directory (symlinks and
other owners refused). The forwarded socket is as trusted as the local one,
so first contact with a host is always confirmed, unconfirmed `ssh-*`
origins are 404, and each host's pages stay in their own origin without the
preload bridge.

## Dependency direction

```text
CLI -> daemon -> runtime -> web -> document, files, git, lsp, projects
desktop -> daemon socket -> runtime
desktop -> ssh -> remote daemon socket -> runtime
CLI -> runtime
git, projects -> files
React -> HTTP API -> HTTP application
web source -> Vite -> internal/web/dist -> go:embed -> servef binary
```

Go and pnpm retain their native dependency graphs. The repository-level
`package.json` is a command facade; `web/package.json` owns browser dependencies;
`desktop/package.json` owns the Electron app's build-time dependencies.
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

Release tags also publish desktop packages (macOS dmg/zip, Linux
AppImage/deb per arch): electron-builder bundles that release's Go binary as
`resources/bin/servef` with Electron's Chromium and Node. `edge` has none.
Bare `servef [path]` opens the installed app through the daemon and otherwise
serves in a browser.

## Growth rules

1. Organize Go around capabilities before adding abstraction layers.
2. Keep HTTP DTOs at the transport boundary; move document/search behavior out
   of `internal/web` as those capabilities grow.
3. Keep browser contracts runtime-validated. Introduce OpenAPI and generated
   client types when manual contract maintenance becomes material.
4. Create shared frontend packages only after a second consumer exists.
5. Add task-graph caching only when multiple projects or CI timings justify it.
