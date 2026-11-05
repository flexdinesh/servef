# 0094: mygo app uses web views

- Status: Accepted (experimental)
- Date: 2026-11-05
- Supersedes: the native UI of [0082](0082-native-mygo-experiment.md);
  [0086](0086-native-agent-tooling-and-ports.md), [0088](0088-native-highlights-and-ondine-via-wazero.md),
  [0089](0089-native-diffs-viewer.md), [0090](0090-native-trees-and-pierre-icons.md),
  [0091](0091-native-workspace-shell-and-memory.md)
- Relates to: [0067](0067-desktop-shell-and-protocol-proxy.md) (the proxy it
  mirrors), [0077](0077-desktop-shell-stays-on-electron.md) (the WebKit findings
  it answers; Electron stays), [0081](0081-desktop-bridge-v2-native-features.md)
  (the bridge), [0079](0079-releases-edge-prerelease-and-install-scripts.md)
  (release assets)

## Context

0082 drew servef with mygo widgets beside the Electron app, and 0086 to 0091
hand-ported the web UI's views, highlighting, diagrams, diffs, trees, and shell
to reach parity. Each port is a second implementation that has to follow every
web change. After six milestones commit history, compares, search, navigation,
and walkthroughs were still missing.

## Decision

The whole window is `apps/web`, in the OS web view. mygo supplies the window,
the native menus and dialogs, and nothing drawn. There is one UI codebase. That
is the point of the change; parity by hand-porting cost too much.

- `apps/mygo` is a thin Go host: ensure the daemon with the bundled `servef`
  (`internal/connect`), one instance per user, `servef-mygo://` links, one
  window. `mygo.json` sets `linux.webview`. It embeds no frontend.
- `internal/proxy` serves the custom scheme `servef-mygo-app://local` from the
  daemon socket with the rules of `electron-protocol.ts`, `proxy.ts`,
  `forward.ts`, and `headers.ts`: `/_daemon` and `/_servef` refused, the same
  request header allowlist, the same Host and `Sec-Fetch-Site` for the daemon's
  checks, the daemon's CSP kept.
- `window.servefDesktop` (bridge v2) goes through mygo IPC: `openFolder`,
  `showContextMenu`, `revealPath`, `showAppMenu`. Each call checks that the
  caller's page is the app's own, and `internal/bridge` validates each payload
  as `context-menu.ts`, `reveal.ts`, and `titlebar.ts` do. Reveal resolves real
  paths inside a daemon root; folders open through `/_daemon/open`.
- ssh stays in the Electron app. So do http and CLI server connections: a
  `servef-mygo://<host>:<port>` link opens in the browser.

The server's `img-src` gains `'self'`: `http:` and `https:` scheme sources do
not match a custom scheme, so an `<img>` of the page's own origin was blocked.
On http origins it permits nothing new. This also fixes the Electron app, where
`servef-app://` has no CSP bypass: with the earlier policy a Markdown image in a
project never loaded (`naturalWidth` 0), and with `'self'` it does. Electron's
`images.spec.ts` covers it.

### 0077's WebKitGTK findings

0077 kept Electron partly because of two scheme handler limits. Both are handled
in the proxy, and both are covered by the e2e:

- A request's context cancels only on a failed pipe write, so a closed
  EventSource would hold its daemon connection forever. Event streams write a
  comment every 5 s; closing four streams dropped their daemon connections
  within 5 s.
- WebKit sends request bodies without Content-Length and no Origin on
  same-origin requests. mygo hands the whole body to the handler, so the proxy
  reads it under a 2 MiB limit and forwards a known length.

A silent non-stream response still cancels only at its next write.

### Where the page keeps its state

On Linux, WebKitGTK's default store is `~/.local/share/servef-mygo/`
(`localstorage/`, observed in the e2e), named by the program, not by mygo. The app
installs in `servef-mygo/app/` beside it, so updates replace only `app/` and
uninstall leaves the store and reports it. A named mygo session would have moved
the store to `~/.config/servef-mygo/Sessions/`, but it starts a second network
process, because mygo's default context still starts one: 577 MiB RSS, 359 MiB PSS
in one run against about 490 and 315 here. On macOS the default store is in
`~/Library/WebKit/<app id>`, outside the bundle, by mygo's documentation; not
observed.

## Names

Both desktop apps are named for their shells during this experiment:
`servef-mygo` here, `servef-electron` in [0092](0092-electron-app-name.md). The
user will pick one and rename it `servef-desktop` later. Nothing migrates from `servef-native`; it
was experimental and had no stable users.

## Tradeoffs

- WebKit instead of Chromium: the UI runs on Safari's engine on macOS and
  WebKitGTK on Linux, whose versions the distribution picks, and only the
  Playwright WebKit suite and this app's e2e watch it. Failures do not block PRs.
- WebKitGTK becomes a Linux runtime dependency. The install scripts warn
  without it; they do not install it.
- The low-memory native window is gone: 0082 measured 124 MiB RSS. With one
  Markdown document (highlighted fence, Mermaid) open in a fresh profile,
  an image, 10 s settled, summing `smaps_rollup` over the app's process tree, in
  the same Debian image under Xvfb (WebKitGTK 2.50.6 on Mesa; Electron with
  `--disable-gpu`), with the image drawn in both: this app 484, 493, and 486 MiB
  RSS, 303, 321, and 317 MiB PSS over three runs; Electron 597 MiB RSS twice, 305
  and 308 MiB PSS; servef about 26 MiB RSS beside either. PSS, which counts shared
  pages once, is about equal, and WebKitGTK's libraries are shared with little
  else on most desktops.
- macOS is built and smoke-tested in CI only, by `build.yml`, which has no PR
  trigger: it first runs after merge. Nobody has run the app on a Mac.
- Dropped against the Electron app: Open at Login, notifications, recent
  folders, the Dock menu, folder drop, ssh, and server connections.

### Known gaps on WebKit

The 0077 failures, by name. Playwright WebKit on `servef browser`, 85 passed and 6
failed; skipping the clipboard grant WebKit does not support let six test bodies
run, and two of them now pass.

Copying, four tests. Each runs up to the test's own `navigator.clipboard.readText()`,
which Playwright WebKit refuses (`NotAllowedError`), so the copied text is unchecked
there; the app's e2e checks that a clipboard write succeeds.

- `app.spec.ts` labels and copies fenced code
- `code-navigation.spec.ts` Control hover preserves selected source text for copying
- `line-links.spec.ts` line numbers select lines and ranges, update the URL, and copy path:line
- `source-control.spec.ts` diff line numbers select lines, update the URL, and copy path:line

UI differences, two tests:

- `scrollbars.spec.ts` document content keeps its width as overflow comes and goes:
  924 px against 934, which 0077 puts down to `scrollbar-gutter`. Text shifts when a
  document gains a scrollbar.
- `source-control.spec.ts` review anchors scroll their lines into view and open the
  unchanged lines they name: the anchor to changed line 12001 of `large.txt` never
  renders its line within 20 s. 0077 calls this an anchor in a collapsed gap, but the
  line is a change.

With no folder served, `/` shows the workspace with status Error and "The server
returned an invalid page response."; File, Open Folder works from there. That path is
the shared web app, not this host; not checked in Electron.

Those suites run in Playwright's WebKit, not in this app. The app's own e2e covers
Explorer, documents, highlighting, diagrams, images, live updates, event streams,
the bridge, context menus, navigation policy, restart, crashes, and a failed daemon.
Search, Source Control, code navigation, and walkthroughs have not been run in it.

Removed with the native UI: `packages/mygo`, the agent tooling and parity loop,
the wazero ports and their pins, and `THIRD_PARTY_NOTICES.txt` (nothing third
party is embedded now). `internal/app`, `pkg/contract`, and `pkg/client`
(decisions 0080, 0083, 0084, 0085, 0087) stay: every transport uses them.
