# 0072: OS integration: open folders from the OS, open at login

- Status: Accepted; revised by [0081](0081-desktop-bridge-v2-native-features.md) (Deferred: reveal, recent folders, Dock menu)
- Date: 2026-11-02
- Implements: [0068](0068-cli-opens-desktop-app-and-desktop-releases.md)
  deferred OS context menus and login item
- Revises: [0062](0062-desktop-app.md) Nothing installed (login
  registration on explicit opt-in), [0067](0067-desktop-shell-and-protocol-proxy.md)
  Windows and links (second-instance data, `open-file`, `--open-path`),
  [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) Packages (macOS
  folder document type, deb file manager actions)

## Context

Users open projects with `servef [path]` or File > Open Folder. File
managers and the Dock are the other natural starting points, and a daily
tool should be able to start at login. Folder paths from the OS are
untrusted input. 0067 says scheme links never add roots, because any web page
can fire one; OS entry points must therefore be argv or `open-file`, which
only local users and processes can trigger (the trust of running
`servef <path>`).

## Decision

### Opening folders

- One opener (`folder-opener.ts`) for every source: the Open Folder dialog,
  macOS `open-file`, `--open-path <paths…>` argv, and second instances. Each
  folder goes to `POST /_daemon/open` on the local daemon in order, so an
  outside folder becomes a persisted root (covering open, 0066), exactly as
  `servef [path]` does. Each distinct project gets a window at
  `servef-app://local/?project=<p>`; a window already showing it is reused.
- At most 8 folders per request (the daemon's open-level cap, 0066); the
  rest are skipped with one report.
- Failures (daemon not ready, a file, a missing folder) are collected into
  one non-blocking message box; successes still open. The app never stays
  running without a window: when nothing opened and no window exists, it
  opens home (its instructions page has Retry).
- Linux argv form is an explicit `--open-path` flag; plain arguments are
  ambiguous beside `electron .` and Chromium switches. Arguments after the
  flag, up to the next `--` switch, are paths; `file://` URLs with an empty
  or `localhost` host convert to paths, other URLs are dropped.
- A second instance parses its own argv and sends `{openPaths}` as
  `requestSingleInstanceLock` `additionalData`; the first instance accepts
  only an array of absolute strings. `second-instance` argv may be reordered,
  so it is never trusted for paths. Paths go only into a JSON request body;
  nothing runs a shell.
- macOS: a `public.folder` document type at rank `Alternate` delivers Dock
  and app-icon drops and `open -a servef <dir>` as `open-file`, without
  claiming folders from Finder. Events from one drop are collected for 100
  ms into one request.
- Linux: the deb installs a Dolphin service menu
  (`/usr/share/kio/servicemenus/servef-desktop.desktop`) and a Nemo action
  (`/usr/share/nemo/actions/servef-desktop.nemo_action`), both running
  `/opt/servef/servef-desktop --open-path %F`. The main entry keeps only
  `x-scheme-handler/servef`: with `inode/directory`, desktops without a
  folder default would pick servef as the folder opener, and every
  `xdg-open <dir>` would add a daemon root.
- Nautilus, Thunar, the AppImage, and Finder's context menu get documented
  recipes the user creates (`site/src/pages/desktop.mdx`); the app installs
  nothing for them.

### Open at Login

- An "Open at Login" checkbox: the app menu on macOS, the File menu on Linux.
  Off by default; only a click registers or unregisters. The checkbox reads
  the OS state and refreshes on window focus when it changed there.
- macOS (packaged only): `app.setLoginItemSettings({openAtLogin})`
  (SMAppService). `requires-approval` shows how to allow it in System
  Settings > General > Login Items, with a button that opens it.
- Linux: writes `$XDG_CONFIG_HOME/autostart/servef-desktop.desktop` (atomic,
  mode 0644; `~/.config` when unset or relative) and removes only that file
  on uncheck. Exec is the installed executable, or `$APPIMAGE` for an
  AppImage, quoted per the Desktop Entry spec. An entry another tool wrote
  under that id counts as enabled.
- Unpackaged builds offer no toggle, except Linux with the test seam
  `SERVEF_DESKTOP_AUTOSTART_DIR`, so development never registers a dev
  Electron. Login launches are normal launches with a window: Linux has no
  tray, and SMAppService ignores hidden starts.
- Failures show the error and where to change it by hand.

### Deferred

- Windows: the daemon is Unix-socket only, bare `servef` uses browser mode,
  and Explorer menus need installer registry writes plus signing.
- Signing, notarization, auto-update, and the Homebrew cask (0068).
- A shipped Finder Quick Action and Nautilus menu: they need native code or
  copying files into the user's home.
- Reveal in file manager, open in editor, root removal (bridge and UI work).
  Reveal shipped in 0081; open in editor was dropped.
- Dock menu and recent folders, launcher actions, tray or background start.
  The Dock menu and recent folders shipped in 0081; launcher actions stay
  deferred.

## Consequences

- OS-opened folders persist as daemon roots, as with the CLI; root removal
  stays deferred.
- Uninstalling the deb leaves the user's autostart entry; an AppImage moved
  after enabling Open at Login leaves a stale Exec until re-toggled. Docs say
  so.
- Release smoke checks the deb's actions and the absence of
  `inode/directory`, and the macOS `public.folder` document type.
- Desktop tests: `open-paths`, `folder-opener`, `autostart`, `linux-files`;
  smoke `os.spec.ts` covers `--open-path` on cold and second launches, a
  refused file, and the autostart entry.
- Unverified in CI: macOS Dock drops, `open -a`, and SMAppService
  registration for an ad-hoc signed app; Dolphin and Nemo actions and login
  on real GNOME and KDE sessions. These are manual checks before release.
