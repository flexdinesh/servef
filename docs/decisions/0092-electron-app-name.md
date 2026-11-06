# 0092: The Electron app is named srcz-electron

- Status: Accepted (experimental)
- Superseded by: [0095](0095-desktop-app-is-mygo-electron-removed.md): the mygo app is named `srcz-desktop`; Electron is removed
- Date: 2026-11-05
- Supersedes: the package and executable names in
  [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) Packages only
- Revises: the names in [0073](0073-install-scripts.md) and
  [0079](0079-releases-edge-prerelease-and-install-scripts.md); "only a click
  registers" in [0072](0072-os-integration.md), for the login entry move

## Context

Two desktop apps exist during an experiment. Each is named for its shell, so
the name says which one a package, path, or task means. One will be picked
later and renamed `srcz-desktop`. Earlier records keep the names they were
written with.

## Decision

- `apps/desktop` is `apps/electron`, package `@srcz/electron`. The executable
  and deb are `srcz-electron`, artifacts
  `srcz-electron_<version>_<os>_<arch>.<ext>`, the install prefix
  `/opt/srcz/srcz-electron`, the Linux desktop id `srcz-electron.desktop`,
  and runtime directories `srcz-electron-<h8>`.
- Tasks and the validation groups use `electron`: `dev:electron`,
  `test:electron`, `package:electron`, `package:electron:dir`, groups `electron`
  and `electron-static`.
- The install scripts are `install-electron.sh` and
  `install-electron-edge.sh`. "Desktop app" stays the category word in prose.
- Unchanged: `srcz://`, the CLI's `desktop:` line, `srcz-app://`, the app id
  `io.github.flexdinesh.srcz`, productName `srcz`, the daemon protocol,
  `window.srczDesktop`, `SRCZ_DESKTOP_*`, and the userData directory
  `<appData>/srcz-desktop`, which holds recent folders and connections.

### Existing installs

- Deb: `srcz-electron` declares `Replaces`, `Conflicts`, and `Provides:
  srcz-desktop`, so `apt install ./srcz-electron_*.deb` removes the old
  package. Both own paths under `/opt/srcz`. There is no apt repository: an
  upgrade happens only when a user installs the new deb. dpkg keeps the old
  package's `rc` record until `sudo dpkg -P srcz-desktop`.
- Install scripts: `install-electron*.sh` delete the old script-installed
  AppImage or app directory, desktop entry, icon, and file manager actions, and
  set the new entry as the `srcz://` handler. They stop while either executable
  runs, and when either deb is installed.
- `install-desktop.sh` and `install-desktop-edge.sh` stay as forwarders that
  fetch and run the new scripts: earlier CLIs and apps print their URLs.
- `uninstall.sh` removes both names and reports both debs.
- Open at Login: on Linux, at startup, an old
  `autostart/srcz-desktop.desktop` that matches exactly what the app wrote
  (only Exec varied) becomes `srcz-electron.desktop` with the current
  executable, and the old file is removed. This revises 0072's "only a click
  registers" for this one move: the user already opted in. An old entry anything
  else wrote or edited is never touched. macOS login items follow the bundle
  id, which is unchanged.

## Consequences

- Not handled, documented in the user docs: hand-written Nautilus and Thunar
  actions and AppImage desktop entries that name `srcz-desktop`, and launcher
  favorites.
- A per-user `mimeapps.list` default naming `srcz-desktop.desktop` stays after
  a deb upgrade: a package script cannot edit it, and the app never sets itself
  as the handler. Generic `xdg-mime` skips the missing entry (checked on Ubuntu
  24.04); desktop-specific lookups are unverified, so the docs give the
  `xdg-mime default` command.
- After an upgrade removes the old executable, the old login entry runs nothing
  until the user opens the app once, which moves it. Scripts and debs cannot
  move it: Open at Login stays the app's own write.
- An edited old login entry stays and fails silently at login until removed.
- A runtime directory from an old run (`srcz-desktop-<h8>`) is not swept. It is
  private to the user and goes away with `$XDG_RUNTIME_DIR` or tmp cleaning.
- The later rename to `srcz-desktop` must drop these relations, since this deb
  conflicts with that name, and repeat this migration in the other direction.
