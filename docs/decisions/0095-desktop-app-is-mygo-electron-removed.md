# 0095: Desktop app is mygo in web views; Electron removed

- Status: Accepted
- Revised by: [0096](0096-correctness-and-extended-validation.md) (desktop resource checks move to extended), [0098](0098-remote-connections.md) (server connections return, one window each), [0099](0099-desktop-notifications.md) (notifications return)
- Date: 2026-11-05
- Supersedes: [0062](0062-desktop-app.md) (the Electron shell),
  [0064](0064-ssh-remotes.md), [0067](0067-desktop-shell-and-protocol-proxy.md),
  [0070](0070-desktop-ssh-connections.md),
  [0071](0071-cli-server-connections-and-pasted-urls.md),
  [0072](0072-os-integration.md), [0075](0075-desktop-title-bar.md),
  [0077](0077-desktop-shell-stays-on-electron.md),
  [0092](0092-electron-app-name.md); in part
  [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) (packages),
  [0076](0076-remote-servers-over-http.md) (ssh and server windows),
  [0081](0081-desktop-bridge-v2-native-features.md) (recent folders, folder drop),
  [0094](0094-mygo-app-uses-web-views.md) (names, ssh stays in Electron, opt-in)
- Revises: [0073](0073-install-scripts.md),
  [0079](0079-releases-edge-prerelease-and-install-scripts.md) (scripts and
  assets), [0059](0059-validation-execution-boundaries.md) (groups)

## Context

0092 and 0094 ran two desktop apps side by side and left the pick to the user.
The pick is mygo: `apps/web` in the OS web view, with Electron gone. "mygo" is the
toolkit's name, so it leaves the product's.

## Decision

### Names

| Thing | Name |
| --- | --- |
| Directory | `apps/desktop` |
| Binary, slug, command | `srcz-desktop` |
| macOS bundle | `srcz-desktop.app` |
| Identifier | `io.github.flexdinesh.srcz.desktop` |
| Link scheme | `srcz://`, which the CLI prints and `internal/desktopapp` detects |
| Page origin | `srcz-app://local` |
| Page bridge | `window.srczDesktop` |
| Scripts | `install-desktop.sh`, `install-desktop-edge.sh` |
| Assets | `srcz-desktop_<version>_linux_<arch>.tar.gz`, `srcz-desktop_<version>_darwin_<arch>.zip` |
| Tasks | `dev:desktop`, `test:desktop`, `test:desktop:e2e` (alias `desktop:e2e`) |
| Validation group | `desktop` |
| Linux data | `$XDG_DATA_HOME/srcz-desktop/`: `app/` and WebKit's store beside it |

"mygo" stays for the framework: the dependency and its imports, `mygo.json`, its
CLI, and prose naming it. `srcz-mygo` was never in a stable release, so its page
state does not migrate.

### Removed

`apps/electron` and everything it alone used:

- `srcz control list` (`Registry.List` stays: walkthrough publish uses it).
- `/_daemon/directories`, `projects.Browse`, and their contract shapes and client
  methods.
- The web app's `cli-`, `ssh-`, and `http-` shareable origins, and the title band
  (0075). The band's height was `env(titlebar-area-height, 0px)`: a Chromium
  window-controls value WebKit lacks, so here it was always 0, and the window keeps
  its native title bar. `showAppMenu` stays on the bridge, with no caller in the
  web app.
- Deb and AppImage packaging, and the `electron` and `electron-static` validation
  groups.

Not ported, on purpose: ssh connections, the Connect page, CLI and http server
connections, Open at Login, notifications, recent folders, the Dock menu, folder
drop, file manager actions. Each is the cost of one shell doing less. A
`srcz://<host>:<port>` link opens in the browser. Nothing replaces them here; a
later decision can add one back.

Left on the server, without a client: `/_daemon/events` and `DaemonEvents`,
walkthrough announcements, `lease=watched`, `/_daemon/roots`. They are shared with
the daemon's design (0061, 0066), not Electron's alone, and removing them is its
own change.

### Validation

The `desktop` group enters `check:pr`, replacing `electron` and `electron-static`:
ten groups. It runs unit tests, then the built app under Xvfb. A failure in the
WebKit app now blocks PRs. Playwright's WebKit project stays opt-in: it compares
against Chromium and has known gaps (0094). The e2e also runs `srcz <dir>`
through a `srcz://` handler registered with real `xdg-utils`, in place of what
Electron's `cli.spec.ts` covered.

### Existing Electron installs

Stable never shipped Electron: v0.1.6 has CLI archives only. Electron installs
came from the edge scripts or a local build: `srcz-desktop` or `srcz-electron`
AppImages, debs, and on macOS `srcz.app`. `install-desktop*.sh`:

- stop while either deb is installed, naming `sudo apt remove`;
- on Linux, after installing, remove the AppImages, autostart entries, Dolphin,
  Nemo, and Nautilus actions, and report a Thunar action, which lives in Thunar's
  own settings file;
- on macOS, remove `srcz.app`. Its name differs, so it would stay and also claim
  `srcz://`;
- never touch Electron's user data, and replace only `srcz-desktop/app`. With
  `XDG_DATA_HOME` equal to `XDG_CONFIG_HOME` that directory holds Electron's data
  and WebKit's store too.

`uninstall.sh` also removes Electron and `srcz-mygo` leftovers, and keeps and
reports data. Its `srcz-native` handling and the `install-electron*.sh` scripts
are gone, so the URL an edge CLI's install hint printed no longer resolves.

## Consequences

- WebKitGTK 4.1 is a Linux runtime dependency, which the scripts warn about but do
  not install. 0094 lists WebKit's known gaps.
- `install-desktop.sh` installs the app, not the CLI (`install-electron.sh` did
  both). The app bundles its own `srcz`; `srcz .` needs `install.sh` too.
- Until a stable release carries the assets, `install-desktop.sh` stops at the
  download with a message naming `install-desktop-edge.sh`, and changes nothing.
- macOS is built and smoke-tested by `build.yml` only; nobody has run the app on a
  Mac. Linux's e2e is the only automated coverage of the window.
- The Electron app's data, including saved ssh connections, stays on disk unused.
