# 0081: Desktop bridge v2: reveal, native context menus, recent folders, folder drop

- Status: Accepted
- Superseded by: [0095](0095-desktop-app-is-mygo-electron-removed.md) for recent folders and folder drop; `openFolder`, `showAppMenu`, `revealPath`, and `showContextMenu` stay
- Date: 2026-11-03
- Revises: [0067](0067-desktop-shell-and-protocol-proxy.md) Bridge v1
  (bridge v2), [0072](0072-os-integration.md) Deferred (reveal in file
  manager, Dock menu and recent folders, launcher actions)
- Implements: [0062](0062-desktop-app.md) native actions that degrade
  outside the app

## Context

The web UI is the source of truth for srcz's interface; the desktop app
adds what only a native shell can do. 0072 deferred reveal in file manager,
a Dock menu with recent folders, and launcher actions. Users also expect
OS context menus in a desktop app and to drop a folder on a window. Open in
editor is dropped: srcz does not guess editors.

## Decision

### Bridge v2

- `window.srczDesktop` is `{version: 2, openFolder(), showAppMenu?(x, y),
  revealPath(project, path), showContextMenu(items, x, y)}`, still exposed
  to `srcz-app://local` frames only (0070). Remote, CLI, http, and app
  page windows get no bridge.
- The UI accepts any integer version from 1 and feature-detects each
  method; a version 1 bridge has only `openFolder` and `showAppMenu`.
  Browsers have no bridge and are unchanged.
- Main validates every message as v1 does: the sender frame must be
  `srcz-app://local`, and every payload is checked for types and bounds
  before use. Invalid requests are dropped without a dialog.

### Reveal in file manager

- `revealPath(project, path)` takes the project ID (absolute, normalized)
  and a project-relative path (`/` separators; no leading `/`, backslash,
  empty, `.`, or `..` segment; at most 4096 bytes). Main asks the local
  daemon for its roots (`/_daemon/status`), requires the project's real path
  inside a root's real path and the target's real path inside the project,
  then calls `shell.showItemInFolder` on it. A symlink leaving the project, a
  missing file, or a project outside the roots is refused with a
  non-blocking report.
- A linked worktree outside every root is refused too: the daemon's status
  lists roots only.
- Offered from Explorer rows and working-tree document tabs. Review,
  walkthrough, Welcome, and commit snapshot tabs have no file to reveal.

### Native context menus

- `showContextMenu(items, x, y)` sends descriptors: `{id, label, enabled?,
  accelerator?}` or `{separator: true}`. At most 32 items; ids match
  `[A-Za-z0-9._:-]{1,64}` and are unique; labels are 1 to 128 characters
  without control characters; accelerators are display hints in Electron's
  grammar and are never registered (the page owns its keys). Leading,
  trailing, and repeated separators are dropped. The point must be finite
  and inside the sender's content size. Any invalid item refuses the menu.
- Main pops the menu at the point (`Menu.popup`), one at a time per window,
  and resolves the chosen id or `null`; the UI runs the action.
- Tabs: with the bridge the tab menu is native (Close, Close Others, Close to
  the Right, Close All, Reveal in File Manager); without it the page keeps
  its own menu. Explorer rows get a native menu (Reveal in File Manager, Copy
  Path, Copy Relative Path) only in the app; browsers keep rows without one.
- Diff file headers and diff lines keep no menu for now: the diff views have
  no context menu to mirror, and adding one is a separate source-control
  change.

### Recent folders

- When a local window starts showing a project inside the daemon's roots,
  its real path moves to the front of a list of at most 10, persisted as
  `userData/recent-folders.json` (`{version: 1, folders}`, mode 0600,
  atomic). Invalid entries are dropped on load.
- File > Open Recent lists them (home shown as `~`) with Clear Recently
  Opened. Choosing one goes through the folder opener (0072), as Open
  Folder does; a folder that no longer exists leaves the list.
- macOS and Windows: `app.addRecentDocument` feeds the system's recent
  items; clearing calls `app.clearRecentDocuments`. macOS: the Dock menu
  lists the recent folders and New Window.
- Linux launcher actions stay deferred: a `.desktop` file's actions are
  static, so a dynamic list means writing files into the user's home, and
  the app installs nothing (0062).

### Folder drop

- The preload accepts OS file drops on local windows when the page did not
  handle the drop, resolves each `File` with `webUtils.getPathForFile`, and
  sends the paths to main. Main accepts at most 64 absolute paths without
  NUL bytes and passes them to the folder opener, which keeps 0072's rules:
  8 folders per request, files and unopenable folders reported, a covering
  open adds a root.

## Consequences

- Dropping a folder adds a daemon root, like the other OS entry points
  (0072); drops come only from the local user's file manager.
- Smoke `bridge.spec.ts` covers bridge v2 on Linux with recorded menus and
  reveals: tab and Explorer menus, refusals, recent folders, and a dropped
  file. Unit tests cover descriptor validation, reveal paths, the recent
  list, and drop payloads.
- Unverified in CI: macOS menu popups (click order against the close
  callback), the Dock menu, system recent items, Finder reveal, and drops on
  macOS; real file manager reveal on GNOME and KDE. These are manual checks
  before release.
