# Architecture

`servef` is one product implemented by build-time projects and distributed as
one Go binary, plus desktop packages that bundle it (decision 0068).

## System boundaries

- `apps/servef/main.go` and `apps/servef/cmd/`: CLI, process lifecycle, listener, browser launch, daemon commands and spawning, opening the desktop app (`servef [path]`), serving no-UI machines on their address with `web:` and `desktop:` links (decision 0076).
- `apps/servef/internal/runtime/`: the web app, HTTP server, and shutdown order shared by every transport; no listener.
- `apps/servef/internal/daemon/`: per-user Unix socket, single-instance lock, `/_daemon/*` control API (typed `Controller` operations and their HTTP handler), roots file, idle exit, client, and `Ensure`; no CLI flags or spawning.
- `apps/servef/internal/app/`: transport-agnostic application layer (decision 0080); domain services (projects, documents, events, source control) return `pkg/contract` types or typed errors; no `net/http`.
- `apps/servef/internal/diff/`: line diffs (hunks, blocks, collapsed gaps, counts) laid out as jsdiff and Pierre lay them out, with time and size bounds (decision 0083); knows bytes only.
- `apps/servef/pkg/contract/`: transport shapes, error kinds and codes, request parameters; the source `tools/contracts` generates `packages/contracts` from.
- `apps/servef/pkg/client/`: Go client over HTTP/TCP, HTTP/Unix socket, and in process.
- `apps/servef/internal/files/`: file discovery, always-skipped directories, and safe path resolution.
- `apps/servef/internal/projects/`: root validation, repository/worktree discovery, project resolution.
- `apps/servef/internal/git/`: go-git reads for Source Control; go-git types stay inside it.
- `apps/servef/internal/document/`: file families, previews, formatting, media types, binary/size policy.
- `apps/servef/internal/lsp/`: language-server data, process lifecycle, JSON-RPC client.
- `apps/servef/internal/control/`: authenticated discovery and shutdown of local servef processes.
- `apps/servef/internal/browser/`, `apps/servef/internal/version/`: browser launch and build version.
- `apps/servef/internal/desktopapp/`: desktop app detection (`servef://` handler), opening URLs in it, install hints.
- `apps/servef/internal/web/`: HTTP adapter (`routes.go`, kind-to-status mapping, server-sent events, raw bytes), shell/static delivery, embedded assets, and the domains not yet moved to `internal/app` (navigation, walkthroughs, metrics).
- `packages/contracts/`: generated TypeScript types and runtime guards (`@servef/contracts`); never edited by hand.
- `apps/web/`: React UI, Vite build, frontend tests and development tooling.
- `apps/desktop/`: Electron app (decision 0067); a client of the daemon socket that proxies `servef-app://` to it; never reads project files. Pure modules stay Electron-free.
- `tools/`: validation, release, and contract-generation tooling, its own Go module. The root `go.work` uses `apps/servef` and `tools` (decision 0078).
- `testdata/markdown/`: canonical filesystem corpus for Go and browser tests.
- `testdata/api/`: transport scenarios for frontend-only development.
- `docs/decisions/`: decision records for scope, file types, views, and limits.
- `apps/site/`: static Vocs user docs; build-time only, not part of the binary.
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
CLI -> daemon -> runtime -> web -> app -> diff, document, files, git, projects, watch
web -> document, files, git, lsp, projects (domains not yet in app)
app, web, client -> pkg/contract
client -> HTTP (TCP, Unix socket) | app + daemon.Controller (in process)
desktop -> daemon socket -> runtime
desktop -> ssh -> remote daemon socket -> runtime
CLI -> runtime
git, projects -> files
React -> @servef/contracts guards -> HTTP API -> HTTP adapter -> app
pkg/contract -> tools/contracts -> packages/contracts (committed, drift-checked)
web source -> Vite -> apps/servef/internal/web/dist -> go:embed -> servef binary
```

Go and pnpm retain their native dependency graphs. The repository-level
`package.json` is a command facade; `apps/web/package.json` owns browser dependencies;
`apps/desktop/package.json` owns the Electron app's build-time dependencies.
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

Vite writes hashed assets to `apps/servef/internal/web/dist`, which no branch
or tag tracks. `.github/workflows/build.yml` builds it before Go compilation
and publishes only GitHub release assets (decision 0079): a `vX.Y.Z` tag on a
`main` commit publishes that stable release, and every green push to `main`
replaces the assets of the `edge` prerelease. Installed releases contain no
Node.js runtime or frontend sidecar; the install scripts are the supported
install.

Both publish CLI archives and desktop packages (macOS dmg/zip, Linux
AppImage/deb per arch): electron-builder bundles that build's Go binary as
`resources/bin/servef` with Electron's Chromium and Node.
Bare `servef [path]` opens the installed app through the daemon and otherwise
serves in a browser.

## Growth rules

1. Organize Go around capabilities before adding abstraction layers.
2. Keep transport shapes in `pkg/contract` and domain operations in
   `internal/app`; `internal/web` only adapts them to HTTP (decision 0080).
3. Keep browser contracts runtime-validated with guards generated from
   `pkg/contract`; move remaining hand-written guards there as their domains
   migrate.
4. Create shared frontend packages only after a second consumer exists;
   generated contracts (`packages/contracts`, decision 0080) are shared by
   design.
5. Add task-graph caching only when multiple projects or CI timings justify it.
