# 0073: Install scripts

- Status: Accepted; paths revised by [0078](0078-apps-and-packages-layout.md); edge scripts (prebuilt `edge` prerelease assets) and Homebrew and `go install` (dropped) revised by [0079](0079-releases-edge-prerelease-and-install-scripts.md); script, path, and asset names revised by [0092](0092-electron-app-name.md)
- Revised by: [0095](0095-desktop-app-is-mygo-electron-removed.md) (scripts, asset names; the desktop script installs the app only)
- Date: 2026-11-02
- Revises: [0068](0068-cli-opens-desktop-app-and-desktop-releases.md)
  Packages (an AppImage registers `srcz://` through `install-desktop.sh`)
  and Releases (edge desktop builds on the target machine)

## Context

The CLI installs through Homebrew or `go install`; the desktop app through a
deb, a dmg, or an AppImage plus a hand-written desktop entry. Machines
without Homebrew or Go (SSH hosts for `srcz remote`), Linux systems without
apt, and a maintainer testing unreleased code on another machine all need
several manual steps. `edge` carries no desktop builds, and publishing a
moving prerelease would mean CI packaging on every push.

## Decision

Five POSIX `sh` scripts in `scripts/`, each run as
`curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/<name> | sh`:

| Script | Installs | From |
| --- | --- | --- |
| `install.sh` | CLI | the latest release archive |
| `install-desktop.sh` | CLI and desktop app | the latest release (archive, AppImage or zip) |
| `install-edge.sh` | CLI | a shallow clone of `edge`, `go build` |
| `install-desktop-edge.sh` | CLI and desktop app | a shallow clone of `edge`, `mise run package:desktop:dir` |
| `uninstall.sh` | removes srcz | n/a |

- One script per purpose: no mode flags and no settings. The edge scripts
  are development tools, documented only in `docs/development.md`.
- No sudo. The CLI goes to the first writable directory already on `PATH`
  among `~/.local/bin` and `~/bin`, else `~/.local/bin`. The scripts never
  edit shell rc files; they print the line to add. They report updates
  (`vX -> vY`), a different `srcz` earlier on `PATH`, and a running daemon
  (`srcz daemon stop`).
- Stable scripts read the latest tag from the redirect of
  `github.com/flexdinesh/srcz/releases/latest` (no API rate limit). No
  checksum verification: integrity rests on HTTPS to GitHub.
- Edge scripts need git, plus Go (`install-edge.sh`) or mise
  (`install-desktop-edge.sh`); missing tools stop the script with an install
  hint, and the scripts never install tools. The desktop edge build runs
  `mise trust` on its temporary clone and supports `mise.lock`'s platforms
  (linux/amd64, darwin/arm64) only. Builds are stamped `0.0.0-dev` with the
  edge commit. The clone is deleted afterwards.
- Desktop, Linux: the app lives in `~/.local/share/srcz-desktop/` (an
  AppImage for stable, the unpacked `app/` for edge; each replaces the
  other). A user desktop entry `srcz-desktop.desktop` handles
  `x-scheme-handler/srcz` (`xdg-mime default`), with the icon in the user
  hicolor theme and the deb's Dolphin and Nemo "Open in srcz" actions in
  user directories. Nothing claims `inode/directory` (0072). The scripts stop
  when the deb is installed or the app is running.
- Desktop, macOS: `srcz.app` in `/Applications`, else `~/Applications`,
  registered with Launch Services. curl sets no quarantine attribute, so
  Gatekeeper does not block the ad-hoc signed app.
- `uninstall.sh` removes the CLI from `~/.local/bin`, `~/bin`, and Go's
  install directory; the app, desktop entry, icon, file manager actions, the
  documented Nautilus script and macOS Quick Action, the login entry, and a
  srcz-desktop AppImage a desktop or login entry runs. It prints removal
  commands for package-managed copies (deb, Homebrew, mise) and stops while
  the app or the daemon runs. It never deletes app data, `roots.json`, or
  caches; it lists them.
- Shared code is duplicated between `# shared:<name>:begin/end` markers;
  `tests/install-scripts.test.ts` fails when copies differ, and runs each
  script against stub tools. `shellcheck` checks every script in the tooling
  group.

## Consequences

- A broken script on `main` reaches users immediately; scripts change only
  with their tests and shellcheck green.
- `install-desktop.sh` fails clearly until a release carries desktop
  packages.
- Edge desktop installs take minutes and download the full toolchain; they
  trail `main` by one CI run, because `edge` moves only after CI passes.
- Versions cannot be pinned through the scripts; Homebrew, `go install`, and
  release downloads still can.
