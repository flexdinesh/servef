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

`main` does not track `apps/servef/internal/web/dist/`. After the frontend build
on `main`, CI force-pushes `edge`: that `main` commit plus the built frontend.
The Go module lives in `apps/servef/` (decision 0078), so
`go install github.com/flexdinesh/servef@edge` no longer builds; use the edge
scripts below.

### Edge on another machine

To try unreleased code on a machine without a checkout, build `edge` there
(decision 0073). The CLI needs git and Go:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-edge.sh | sh
```

The desktop app and CLI need git and mise (linux/amd64 or darwin/arm64);
the script clones `edge` to a temporary directory, trusts its `mise.toml`,
installs the toolchain, and runs `mise run package:desktop:dir`, which takes
minutes:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop-edge.sh | sh
```

Both replace a stable install in the same places (`~/.local/bin`; Linux
`~/.local/share/servef-desktop/`, macOS `/Applications`). On Linux the
desktop script registers `servef://` and the Dolphin and Nemo actions for the
user; it stops if the deb is installed or the app is running. Builds report
`0.0.0-dev` and the edge commit. To go back, run a stable install script, or
remove everything (user data stays) with:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/uninstall.sh | sh
```

The scripts share code between `# shared:<name>:begin/end` markers; edit
every copy. `mise run test:release` runs shellcheck and
`tests/install-scripts.test.ts`, which checks the copies match and runs each
script against stub tools.

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
| Build a local binary | `mise run build:web && go build -o ./bin/servef ./apps/servef` |
| Install the local CLI | `mise run install` |
| Run Go tests | `mise run test:go` |
| Run Go tests with the race detector | `mise run test:race` |
| Run frontend tests | `mise run test:web` |
| Test release tooling | `mise run test:release` |
| Type-check TypeScript | `mise run typecheck` |
| Lint (oxlint: React Hooks, UI primitives; stylelint: design tokens) | `mise run lint` |
| Check Go formatting and module tidiness | `mise run fmt:check`, `mise run tidy:check` |
| Validate committed HEAD before PR work | `mise run check:pr` |
| Run browser tests | `mise run test:browser` |
| Run selected browser coverage | `mise run test:browser -- tests/browser/live-updates.spec.ts` |
| Run the desktop app against an isolated dev daemon | `mise run dev:desktop` |
| Run desktop unit and Electron smoke tests | `mise run test:desktop` |
| Measure desktop and daemon memory (host, needs a display) | `mise run measure:desktop` |
| Inspect current validation coverage | `mise run check:status` |
| Run full checks, including race tests and docs build | `mise run check:full` |
| Prune completed validation artifacts | `mise run check:prune` |

Targeted mise tasks capture current edits and relevant unignored new files in
immutable snapshots. All tests run in Dagger, including unit and runner tests;
test prerequisites and Go vet build their embedded frontend there. Formatting,
development, and safe static checks stay on the host. Static checks use private
worktree-local snapshots, homes, temporary directories, and dependency state.

Commit first, then run `mise run check:pr` before creating or updating a PR.
It requires clean committed HEAD and covers all eleven host/container groups.
`check`, `check:full`, and `validate` are compatibility aliases. Immediately
before the separate push/PR operation, run `mise run check:status` and confirm
HEAD still matches its reported commit. Targeted working-copy checks do not
establish PR evidence. `check:pr -- --force` reruns checks and builds; an active
rerun revokes earlier success until it completes successfully.

See [local validation](validation.md) for explicit engine setup, source/cache
identities, resource limits, force reruns, artifacts, and status.
There is no mandatory pre-push suite and no remote validation gate. Existing
CI edge publication and release builds remain unchanged. Validation never
publishes a PR; local conventions are bypassable.

Browser validation uses a prepared Go binary, ordinary and Git fixture servers,
Vite, and private per-test live-update servers inside one container. Each run
owns its writable fixtures, homes, caches, and outputs. Dagger installs the
matching Chromium version. Native macOS tests remain explicit optional
supplements for filesystem watching, process, and browser behavior; routine
tests stay in Dagger.

Go builds, tests, and vet need `apps/servef/internal/web/dist/`; the isolated graph supplies
it. Before host development builds in a fresh worktree, run `mise run build:web`.
Build outputs are ignored and never copied from the host into validation.

Public mise tasks orchestrate validation; hidden `_host:*` and `_container:*`
leaves contain commands and never call public wrappers. Do not run those leaves
directly as an alternative to isolated test execution.

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

Frontend source and dependencies live in the `apps/web` workspace. Production builds
write to `apps/servef/internal/web/dist`, which is embedded in the Go binary and ignored by
Git.

## Desktop development

`apps/desktop/` is the Electron app (decision 0067). `mise run dev:desktop` builds
the web dist, a dev binary at `.dev/desktop/servef`, and the desktop bundle,
fetches the Electron binary once, then runs the app against an isolated daemon:
socket, roots file, app data, and walkthrough cache all live under the
gitignored `.dev/`. It never touches your daemon or roots (on macOS the
walkthrough cache isolation is partial). Open Folder (Cmd/Ctrl-O) adds roots
to the dev daemon.

Pure modules under `apps/desktop/src/main/` never import `electron`; only
`main.ts`, `electron-*.ts`, and `preload.ts` do (oxlint enforces it). Unit
tests run under plain Node. `mise run test:desktop` runs them and the
Playwright `_electron` smoke under Xvfb in Dagger (Linux only). To run the
smoke on a host with a display: `mise run build:desktop`, then
`SERVEF_DESKTOP_BINARY=$PWD/.dev/desktop/servef pnpm --filter @servef/desktop test:smoke`
(add `SERVEF_DESKTOP_ELECTRON_ARGS=--ozone-platform=wayland` under Wayland).
Headless Chromium hides the page, which stops live updates; do not use it.

To try ssh connections (decisions 0070, 0076) in dev, install servef on a
host you reach with `ssh -o BatchMode=yes <host> true` and listed as a
`Host` alias in `~/.ssh/config`, run `mise run dev:desktop`, and pick the
host in File > Connect…. A `servef .` run on that host prints `web:` and
`desktop:` links to open directly. Dev-only seams: `SERVEF_DESKTOP_SSH` (ssh binary),
`SERVEF_DESKTOP_SSH_CONFIG` (`-F` for the master and the alias list),
`SERVEF_DESKTOP_RUNTIME_DIR`, and `SERVEF_DESKTOP_AUTO_CONFIRM=1` with
`SERVEF_DESKTOP_DIALOG_LOG`. `mise run test:desktop` also runs
`apps/desktop/test/ssh/` against a throwaway sshd on 127.0.0.1.

OS entry points (decision 0072): with the `dev:desktop` environment, run
`apps/desktop/node_modules/.bin/electron --no-sandbox apps/desktop --open-path <dir>…`
to cold-launch with folders, or again while the app runs to hand them over.
Open at Login appears in dev only with `SERVEF_DESKTOP_AUTOSTART_DIR=<absolute dir>`
(Linux), so dev never registers a dev Electron at login.

To inspect the Linux packages, run `mise run package:desktop`, then list the
deb (`dpkg-deb -c apps/desktop/dist/servef-desktop_0.0.0-dev_linux_$(go env GOARCH).deb`,
or `bsdtar -tf` on its `data.tar.xz`). It must contain
`usr/share/kio/servicemenus/servef-desktop.desktop`,
`usr/share/nemo/actions/servef-desktop.nemo_action`, and one entry under
`usr/share/applications/` without `inode/directory`; `desktop-file-validate`
checks that entry (not the KDE service menu). electron-builder's bundled fpm
needs `libcrypt.so.1` (Arch: `libxcrypt-compat`), `xz`, and `ar`.

## Adding a file type

Every served file is browsable; registration only decides how it is shown.
Families are composed in `apps/servef/internal/document.Builtins()`:

- `apps/servef/internal/files` owns directory traversal, the always-skipped `.git`,
  `node_modules`, and `vendor` directories, depth, base-name globs, and safe
  path resolution. It knows no file formats.
- `apps/servef/internal/document` owns the registry, type metadata, escaped highlighting,
  previews, formatting, and binary/size policy. Handlers
  receive bytes and navigation context; they never read the filesystem.
  Markdown's parser and link/image rewriting live here, separate from HTTP.
- `apps/servef/internal/web` reads indexed files and transports results. React uses the
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

User docs are a static [Vocs](https://vocs.dev) site in the `apps/site` workspace.
The marketing landing is `/`: `apps/site/src/pages/index.mdx` (`layout: blank`)
renders `apps/site/src/landing/Landing.tsx`. Its styles are scoped under `.landing`
in `landing.css`; animated SVG scenes live in `apps/site/src/landing/svg/` and show
their final frame under reduced motion. Docs pages live in
`apps/site/src/pages/docs` (served at `/docs`); navigation lives in
`apps/site/vocs.config.ts`. The landing theme toggle shares Vocs' `vocs-theme`
storage, so the landing and docs keep one theme.

| Purpose | Command |
| --- | --- |
| Run the docs dev server | `pnpm dev:site` |
| Build the static site | `pnpm build:site` |
| Preview the built site | `pnpm --filter @servef/site preview` |

`pnpm build:site` fails on dead internal links. Static HTML is written to
`apps/site/dist/public`, which Git ignores.

### Cloudflare Pages

The site deploys as plain static files through Cloudflare Pages Git
integration (decision 0074). Project settings:

| Setting | Value |
| --- | --- |
| Root directory | repository root (CF reads `.node-version`, Node 26) |
| Build command | `pnpm install --frozen-lockfile --filter @servef/site... && pnpm build:site` |
| Build output directory | `apps/site/dist/public` |
| Environment variables | `SKIP_DEPENDENCY_INSTALL=1`; `PNPM_VERSION=11.24.0` if the build image ignores `packageManager` |

The filtered install skips the web and desktop workspaces. Cloudflare serves
`404.html` for unknown paths. There is no `wrangler.toml`: Pages build
settings live in the dashboard.

## Release

See the [release guide](release.md).
