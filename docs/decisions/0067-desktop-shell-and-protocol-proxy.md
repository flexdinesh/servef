# 0067: Desktop shell and protocol proxy

- Status: Accepted; revised by [0070](0070-desktop-ssh-connections.md) (connection kinds, per-origin windows, local-only bridge), [0072](0072-os-integration.md) (OS folder entry points), [0081](0081-desktop-bridge-v2-native-features.md) (bridge v2)
- Date: 2026-11-01
- Implements: [0062](0062-desktop-app.md) Shell, local Connections, Lifetime
  and notifications, Nothing installed
- Revises: [0059](0059-validation-execution-boundaries.md) (eleven groups)
- Consumes: [0066](0066-lease-levels-and-desktop-events.md) (lease levels,
  `head`, `/_daemon/events`, `/_daemon/open`)

## Context

0062 proposes an Electron thin client over the embedded UI, one stable origin
per connection, and notifications while the app runs. 0066 gave the daemon
what the app needs. This record fixes how the app is built, how renderer
requests reach the daemon socket, and what the first version exposes. The
CLI flip (`servef [path]` opens the app) and packaging are a later layer.

## Decision

### Package

- `desktop/` is the pnpm workspace package `@servef/desktop`: Electron
  45.1.0, esbuild, TypeScript, oxlint, Playwright; no runtime dependencies.
  esbuild bundles `src/main/main.ts` to `build/main.mjs` (ESM) and
  `src/preload/preload.ts` to `build/preload.cjs` (sandboxed preloads are
  CJS). pnpm never runs Electron's install script (`allowBuilds: electron:
  false`); the binary downloads, checksum-verified, only where needed
  (`node desktop/node_modules/electron/install.js`: dev tasks and the
  `desktop` validation group).
- Only `main.ts`, `electron-*.ts`, and `preload.ts` import `electron`; every
  other module is pure or Node-only and unit-tested under plain Node.
  oxlint `no-restricted-imports` names the fix.
- Desktop imports `web/src/api/events-contract.ts` (and `http.ts`'s
  `isRecord`) by relative path. It is the second consumer; a shared package
  waits for a third.

### Daemon connection

- Binary: packaged apps use `resources/bin/servef` only; unpackaged runs use
  `SERVEF_DESKTOP_BINARY`, else `servef` on PATH. The app never installs or
  copies it. Missing → an instructions page with Retry.
- `servef daemon ensure --json` (timeout 15 s; dev and test flags `--socket`,
  `--roots-file` only from `SERVEF_DESKTOP_SOCKET`,
  `SERVEF_DESKTOP_ROOTS_FILE`). A protocol other than 1 → a mismatch page
  with `servef daemon stop`, Retry, and "Stop the running daemon and retry".
  A release mismatch shows only in About. The app never stops the daemon on
  quit.
- `servef daemon ensure` marks inherited descriptors above stderr
  close-on-exec before starting the daemon: Electron leaves Chromium's
  descriptors inheritable, and the daemon otherwise held its user-data files
  and stdio after the app quit.
- One connection, `local` (`servef-app://local`). Pasted localhost URLs, CLI
  servers, and ssh remotes are later connection kinds.

### Protocol proxy

- `servef-app` is registered privileged (`standard`, `secure`,
  `supportFetchAPI`, `stream`, `codeCache`) and handled in the main process
  over the socket with `node:http`.
- Refused locally: unknown connection hosts (404); a decoded path starting
  `/_daemon` (403, so `/%5Fdaemon/...` too); a present `Origin` other than
  the connection's (403). The renderer sends `Origin` on script and CSS loads
  but not on same-origin GET fetches, so the guard applies only when present.
- Forwarded request headers are an allowlist (`accept`, `accept-language`,
  `content-type`, `content-length`, `range`, `if-none-match`,
  `if-modified-since`, `last-event-id`, `cache-control`,
  `x-servef-language-servers`) plus `Host: localhost` (the daemon's Host
  check) and `Sec-Fetch-Site: same-origin`, which keeps the daemon refusing
  control requests that arrive through the proxy.
- Responses stream through unchanged minus hop-by-hop headers; the daemon's
  CSP `'self'` resolves to the connection origin. Cancelling the response
  body destroys the upstream request, so closing an EventSource ends the
  daemon's request context. Event streams use their own socket; other
  requests share a keep-alive pool (16).
- Daemon gone (`ECONNREFUSED`, `ENOENT`): one single-flight re-ensure; GET
  and HEAD retry once, other methods get 503 `daemonRestarted`. Failure
  pages carry their own CSP (`default-src 'none'; style-src
  'unsafe-inline'`); `/api/*` gets JSON `daemonUnavailable`.

### Windows and links

- Windows use `contextIsolation`, `sandbox`, no `nodeIntegration`, and
  `webSecurity`; permissions are denied except `clipboard-sanitized-write`.
  Navigation stays on the connection origin; `servef://` links route
  internally; `http(s):` and `mailto:` open externally; the rest is denied.
- `servef://local/` and `servef://local/view` map to the same path, query,
  and fragment on `servef-app://local`. `servef://ssh/...` shows "Remote
  links arrive in a later servef". Scheme links never add roots.
- One instance: a second launch hands its link over and exits. A link for
  project P loads in the window showing P (focused first, then most recently
  focused), else a new window; a link without a project focuses the latest
  window.
- Closing the last window quits, except on macOS. The last closed window's
  bounds persist in `userData/window-state.json`.

### Bridge v1

- `window.servefDesktop = {version: 1, openFolder()}`, honored only from
  `servef-app://` frames. Open Folder (also File menu, Cmd/Ctrl-O) picks a
  directory, posts `/_daemon/open`, and opens its project. The UI shows
  "Open folder…" in the Welcome tab and the empty project picker only when
  the bridge exists.
- In the app, copied agent instructions name `servef://local` and omit the
  curl fallback; browsers are unchanged.

### Subscriptions and notifications

- The main process holds `/_daemon/events` while it runs (walkthrough
  notices; keeps the daemon from idling), an `open` lease per project any
  window shows, and a `watched` lease for up to 16 projects shown earlier
  this run. Streams reconnect with backoff (1 s to 30 s).
- Notices: a branch switch, a new commit, a walkthrough published, and edits
  settled (10 s without a change after at least one changed path). Clicks
  open the project, commit, walkthrough, or working review. A notice for the
  focused window's project is suppressed; one per project and kind per 30 s.
  OS settings control enablement.

### Validation

- Host group `desktop-static` (typecheck, lint) and Dagger group `desktop`
  (unit tests, then a Playwright `_electron` smoke under `xvfb-run` with
  `--no-sandbox`) bring `check:pr` to eleven groups. The smoke covers Linux
  only.

### Spike outcome

- Electron 45.1.0 on Linux with the primary `servef-app://` scheme passed:
  incremental EventSource with `stream: true`, module workers and
  `'wasm-unsafe-eval'` under the daemon CSP, localStorage per origin across
  restarts, and EventSource close cancelling the upstream request. The
  `*.localhost` fallback was not needed. macOS is unverified.
- Headless ozone reports `visibilityState=hidden` (the UI closes its live
  stream), so smoke tests run under a real display server.

### Memory baseline

Linux, Electron 45.1.0, `mise run measure:desktop` under a virtual Wayland
compositor; PSS (RSS in parentheses):

| Checkpoint | Electron total | Daemon |
| --- | --- | --- |
| One window, idle 30 s | 313 MiB (611 MiB) | 21 MiB (28 MiB) |
| Two windows, idle 30 s | 354 MiB (772 MiB) | 21 MiB (28 MiB) |
| After one navigation | 376 MiB (795 MiB) | 23 MiB (30 MiB) |

Per window the renderer adds about 70 to 95 MiB PSS; the browser process
(about 110 MiB), GPU (about 70 MiB), and utility (about 30 MiB) processes
are shared. No language servers ran. Defaults stay light (one renderer per
window, no hidden windows, no tray, background throttling on); no Chromium
flags until measured.

## Consequences

- The desktop app costs about 15 times the daemon's memory at idle; servef's
  own memory priority is unchanged and measured separately.
- Hidden windows keep open leases (needed for settled notices), bounded by
  the daemon's 8 open projects.
- macOS behavior (`open-url`, notifications, LaunchServices) and the macOS
  spike remain manual checks.
- Whole-workspace installs fetch Electron's npm package but not its binary.
