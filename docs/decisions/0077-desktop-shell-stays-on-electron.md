# 0077: Desktop shell stays on Electron

- Status: Accepted
- Date: 2026-11-03
- Evaluates: [0062](0062-desktop-app.md) Shell,
  [0067](0067-desktop-shell-and-protocol-proxy.md) Package, Protocol proxy,
  Memory baseline

## Context

The desktop app should be light (memory, CPU, bundle size) and robust.
Electron bundles Chromium and Node: a 102 MiB deb and about 320 MiB PSS for
one idle window (0067). System-webview shells in Go promise less. Two were
evaluated against what the app relies on: one `servef-app://<connection>`
origin per connection with its own storage (0067, 0070, 0071, 0076),
incremental event streams whose close cancels the upstream request, module
workers and `compileStreaming` under the daemon CSP, a bridge only on
`servef-app://local`, navigation and window-open policy, single instance
with links and folder arguments.

## Decision

Keep Electron. Neither candidate is lighter where it matters or matches the
origin model without workarounds.

### Wails v3 (rejected on design)

Wails v3.1.0 (stable) serves every page from one asset-server origin
(`wails://localhost`), routes by path only, registers no further in-webview
schemes, and does not scope bindings per window or origin. Per-connection
origins, and with them storage isolation and the bridge boundary for remote
servers (0070), cannot be reproduced. Not built.

### mygo 0.7.4 (spiked on Linux, not adopted)

A minimal mygo shell proxied `servef-app://local` and a second connection to
two real daemons with 0067's proxy rules, and ran ten gates and the 0067
memory scenario beside Electron 45.1.0 in one Debian bookworm image under
Xvfb (WebKitGTK 2.50.6). Spike branch `spike/desktop-shell` (unmerged).

| Gate | Result |
| --- | --- |
| Incremental event streams, daemon `head` baseline | Pass |
| Close or navigation cancels upstream | Fails: cancelled only on the next failed write (24.5 s for `/api/events`, never for a silent stream); passes with a 1 s keepalive comment from the shell |
| Distinct origins, separate localStorage | Pass |
| localStorage across restarts | Pass |
| Real UI: module workers, Ondine `compileStreaming`, CSP enforced | Pass |
| Request headers | Accept, Last-Event-ID, Range arrive; WebKitGTK sends no `Origin` on same-origin POST and no `Sec-Fetch-*`; cross-origin requests carry `Origin` and the guard refuses them |
| POST bodies through the proxy | Fails: mygo sets the body length only from a `Content-Length` header WebKitGTK omits, so bodies arrive empty; passes with a shell fix |
| Bridge only on local | Pass (`Untrusted` pages rejected) |
| Navigation and window-open policy | Pass |
| Single instance, links, `--open-path` | Pass |

Medians of three runs, PSS (RSS) of the app process tree:

| Checkpoint | Electron 45.1.0 | mygo 0.7.4 |
| --- | --- | --- |
| One window, idle | 323 (638) MiB | 330 (503) MiB |
| Two windows, idle | 371 (807) MiB | 436 (760) MiB |
| After one navigation | 396 (837) MiB | 449 (772) MiB |
| Startup to first page | 440 ms | 489 ms |
| Idle CPU, one window, 30 s | 0.37 s | 0.59 s |
| Live-update burst CPU, 12 s | 0.41 s | 1.11 s |
| deb / Installed-Size | 102.1 / 292.2 MiB | 12.0 / 29.7 MiB |

- A WebKit web process costs 155 to 180 MiB PSS per window against 70 to 95
  MiB for an Electron renderer; GTK and WebKit add about 100 MiB to the Go
  process. Electron throttles unfocused windows; both WebKit windows worked
  during the burst.
- The bundle win moves into a system dependency: WebKitGTK and its closure
  add about 407 MiB (114 packages) to a minimal Debian, mostly already
  present on GNOME desktops. Its version is the distribution's, and the
  AppImage target goes away.
- mygo also grants CORS to every app-scheme origin for trusted pages, so the
  0067 `Origin` guard becomes the only cross-connection boundary, and it
  reads request bodies whole on the GTK main thread.

### Revisit when

- mygo cancels scheme requests when WebKit stops them and passes bodies
  without `Content-Length`, and a WebKit shell measures below Electron on
  memory and CPU; or
- a macOS-only need appears (WKWebView was not measured).

## Consequences

- 0062 and 0067 stand: Chromium everywhere, one engine to test, the
  measured memory cost.
- `mise run test:browser:webkit` (opt-in) keeps reporting WebKit
  compatibility: 83 of 91 pass; six failures are the harness's clipboard
  permission, two are UI differences (`scrollbar-gutter` and a review anchor
  in a collapsed gap).
- Not measured: macOS, a GPU or Wayland session, notifications and tray, and
  language-server memory.
