# 0092: The Electron app is named servef-electron

- Status: Accepted (experimental)
- Date: 2026-11-05
- Supersedes: the package and executable names in
  [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) Packages only

## Context

Two desktop apps exist during an experiment. Each is named for its shell, so
the name says which one a package, path, or task means. One will be picked
later and renamed `servef-desktop`. Earlier records keep the names they were
written with.

## Decision

- `apps/desktop` is `apps/electron`, package `@servef/electron`. The executable
  and deb are `servef-electron`, artifacts
  `servef-electron_<version>_<os>_<arch>.<ext>`, the install prefix
  `/opt/servef/servef-electron`, the Linux desktop id `servef-electron.desktop`,
  and runtime directories `servef-electron-<h8>`.
- Tasks and the validation groups use `electron`: `dev:electron`,
  `test:electron`, `package:electron`, `package:electron:dir`, groups `electron`
  and `electron-static`.
- The install scripts are `install-electron.sh` and
  `install-electron-edge.sh`. "Desktop app" stays the category word in prose.
- Unchanged: `servef://`, the CLI's `desktop:` line, `servef-app://`, the app id
  `io.github.flexdinesh.servef`, productName `servef`, the daemon protocol,
  `window.servefDesktop`, `SERVEF_DESKTOP_*`, and the userData directory
  `<appData>/servef-desktop`, which holds recent folders and connections.

### Existing installs

- Deb: `servef-electron` declares `Replaces`, `Conflicts`, and `Provides:
  servef-desktop`, so `apt install ./servef-electron_*.deb` removes the old
  package. Both own paths under `/opt/servef`. There is no apt repository: an
  upgrade happens only when a user installs the new deb. dpkg keeps the old
  package's `rc` record until `sudo dpkg -P servef-desktop`.
- Install scripts: `install-electron*.sh` delete the old script-installed
  AppImage or app directory, desktop entry, icon, and file manager actions, and
  set the new entry as the `servef://` handler. They stop while either executable
  runs, and when either deb is installed.
- `install-desktop.sh` and `install-desktop-edge.sh` stay as forwarders that
  fetch and run the new scripts: earlier CLIs and apps print their URLs.
- `uninstall.sh` removes both names and reports both debs.
- Open at Login: on Linux, at startup, an old
  `autostart/servef-desktop.desktop` that matches exactly what the app wrote
  (only Exec varied) becomes `servef-electron.desktop` with the current
  executable, and the old file is removed. This revises 0072's "only a click
  registers" for this one move: the user already opted in. An old entry anything
  else wrote or edited is never touched. macOS login items follow the bundle
  id, which is unchanged.

## Consequences

- Not handled, documented in the user docs: hand-written Nautilus and Thunar
  actions and AppImage desktop entries that name `servef-desktop`, and launcher
  favorites.
- A per-user `mimeapps.list` default naming `servef-desktop.desktop` stays after
  a deb upgrade: a package script cannot edit it, and the app never sets itself
  as the handler. Generic `xdg-mime` skips the missing entry (checked on Ubuntu
  24.04); desktop-specific lookups are unverified, so the docs give the
  `xdg-mime default` command.
- An edited old login entry stays and fails silently at login until removed.
- A runtime directory from an old run (`servef-desktop-<h8>`) is not swept. It is
  private to the user and goes away with `$XDG_RUNTIME_DIR` or tmp cleaning.
- The later rename to `servef-desktop` must drop these relations, since this deb
  conflicts with that name, and repeat this migration in the other direction.
