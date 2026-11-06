# 0101: Install script installs the CLI and the desktop app

- Status: Accepted
- Date: 2026-11-06
- Revises: [0073](0073-install-scripts.md) (one script per purpose, no flags),
  [0079](0079-releases-edge-prerelease-and-install-scripts.md) (one script per
  part), [0095](0095-desktop-app-is-mygo-electron-removed.md) (`install-desktop*.sh`;
  the desktop script installs the app only)

## Context

Two scripts per channel made the common case two commands. A user who ran only
the desktop script had an app but no `srcz` on `PATH`, so `srcz .` and
`srcz walkthrough` did not work. `srcz` opens the desktop app by default, so the
two belong together.

## Decision

- One install script per channel: `install.sh` (stable) and `install-edge.sh`
  (edge). By default each installs the CLI and the desktop app.
- `--cli-only` installs the CLI alone: `curl ... | sh -s -- --cli-only`. It is the
  only option; any other argument stops the script. The docs show it right next
  to the default command.
- No UI detection. A machine with no UI gets both unless `--cli-only` is passed.
  The remote docs use `--cli-only` for hosts.
- `install-desktop.sh` and `install-desktop-edge.sh` are deleted, with no
  forwarders.
- The default install stops, changing nothing, while the app runs. Both
  downloads finish before anything is installed. `--cli-only` does neither
  check nor download for the app, and leaves an installed app alone.
- PATH behavior stays as in 0073: use `~/.local/bin` or `~/bin` when on `PATH`,
  else `~/.local/bin` and print the line to add. Scripts never edit rc files.

## Consequences

- Edge CLIs already installed print a hint naming `install-desktop.sh`, which
  now 404s. New builds name `install.sh`.
- A default install on a server downloads an app it cannot use, and may warn
  about WebKitGTK.
- An update of the CLI through the default command waits until the app is quit;
  `--cli-only` does not.
