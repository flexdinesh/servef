# 0068: CLI opens the desktop app; desktop releases

- Status: Accepted; revised by [0076](0076-remote-servers-over-http.md) (step 2: no-UI machines serve over http), [0069](0069-servef-remote-and-ssh-links.md) (SSH sessions), [0072](0072-os-integration.md) (folder document type, deb file manager actions), [0073](0073-install-scripts.md) (install scripts, edge desktop builds), [0079](0079-releases-edge-prerelease-and-install-scripts.md) (Releases: tag-triggered, edge prerelease with desktop packages, no Homebrew); package and executable names superseded by [0092](0092-electron-app-name.md)
- Date: 2026-11-01
- Implements: [0063](0063-cli-commands-and-url-scheme.md) `servef [path]` and
  the local scheme; [0062](0062-desktop-app.md) Revisions when accepted
  (Distribution scope, release packages)
- Revises: [0021](0021-built-frontend-on-release-tags-and-edge.md) (release
  tags add desktop packages), [0063](0063-cli-commands-and-url-scheme.md)
  (open URL form; SSH sessions), [0065](0065-walkthrough-publish-cli.md)
  (publish URL)
- Supersedes: [0060](0060-scope-flags-removed.md) clause "Bare `servef [path]`
  keeps browser behavior until the desktop app ships"

## Context

0067 shipped the Electron shell over the daemon, but nothing opened it and no
release built it. `servef [path]` still started a browser server. The app
must reach users as installable packages that bundle the CLI, and the CLI
must hand paths to the app without installing anything.

## Decision

### `servef [path]`

Bare `servef` (same flags as `servef browser`, which is unchanged) decides in
this order:

1. `--host` or `--port` set: browser mode, silently.
2. SSH session (`SSH_CONNECTION`, `SSH_CLIENT`, or `SSH_TTY`): notice
   "servef remote is not available yet", browser mode. `servef remote` (0064)
   is later.
3. OS other than macOS or Linux: browser mode.
4. Paths and discovery are validated as in browser mode; errors exit.
5. Linux: no `x-scheme-handler/servef` default (`xdg-mime query default`):
   notice and install hints, browser mode.
6. Ensures the daemon in process, as `servef daemon ensure` (`--depth` and
   `--scan-depth` apply to a started daemon; a running daemon with other
   values gets a warning). Failure or a protocol mismatch: notice, browser
   mode.
7. `/_daemon/open` per path; the first result's project is the target.
8. `--no-open` prints the URL and exits. Otherwise the OS opens it (macOS
   `open`, which fails without a handler; Linux `xdg-open`, started in its
   own session and treated as launched after 3 s, since without a recognized
   desktop environment it runs the handler in the foreground). Failure:
   notice and install hints, browser mode.
9. Prints `opened <project> in servef` and the URL; exits 0.

The open URL is `servef://local/?project=<abs>` (project landing). 0063's
`servef://local/view?project=` is a required-file page and errors without
`path`.

`internal/desktopapp` owns detection, opening, and per-OS install hints; it
knows no daemon. The CLI never installs, copies, or registers anything.

### Walkthrough publish

When the daemon announces the walkthrough (it serves the project and reads
the same store) and the app handles `servef://`, the URL is
`servef://local/view?project=…&wt=…` and `"open": true` opens it in the app.
An app that fails to open falls back to the running browser servers (0065).

### Packages

- electron-builder 26.18.1 (`desktop/electron-builder.yml`), driven by
  `desktop/scripts/package.ts --arch amd64|arm64 --servef <binary>
  [--version <x.y.z>] [--dir]` on the host OS, one arch per run. Artifact
  names: `servef-desktop_<version>_<darwin|linux>_<amd64|arm64>.<ext>`.
- Identity `io.github.flexdinesh.servef`, name "servef"; the Linux executable
  is `servef-desktop` so it never shadows the CLI on PATH.
- The release Go binary for that OS and arch is `resources/bin/servef`
  (0067's packaged lookup); not on PATH.
- Packages register `servef://`: Info.plist `CFBundleURLTypes`; Linux desktop
  entry `servef-desktop.desktop` with `MimeType=x-scheme-handler/servef` and
  `Exec=… %U`. The app never calls `setAsDefaultProtocolClient`.
- Targets: macOS dmg and zip per arch (no universal build); Linux AppImage
  and deb per arch. An AppImage registers the scheme only with desktop
  integration (for example AppImageLauncher); otherwise `servef .` falls back.
- macOS builds are ad-hoc signed with the hardened runtime off. Users clear
  Gatekeeper once (right-click Open, or remove the quarantine attribute).
  Signing and notarization need an Apple Developer ID Application
  certificate, an App Store Connect API key, the hardened runtime with
  `allow-jit`, the bundled `servef` signed, and a stapled dmg; deferred.
- No auto-update and no Homebrew cask until builds are signed.
- The app's userData is `<appData>/servef-desktop`: Electron's default,
  `<appData>/servef`, is the CLI's config directory holding `roots.json`.

### Releases

- The Release workflow runs `prepare` (version, release commit, Go archives,
  archive smoke), then `desktop-linux` (ubuntu) and `desktop-macos` (macOS)
  packaging the archived binaries with smoke checks, then `publish`
  (`checksums.txt` over every asset, tag, release, Homebrew tap). The release
  commit reaches `publish` as a git bundle, so the tag names the commit the
  binaries stamp. Reruns never rebuild published artifacts.
- `edge` builds no desktop packages. Locally: `mise run package:desktop`
  (installers) and `mise run package:desktop:dir` (unpacked), host OS and
  arch.
- Distribution: the CLI stays one self-contained Go binary. The desktop app
  is a separate artifact that bundles that binary plus Electron's Chromium
  and Node.

## Consequences

- `servef .` exits once the app opens; scripts that need a server use
  `servef browser` or pass `--port`.
- This change merges only with the first release that publishes desktop
  packages, so `servef .` never points at a download that does not exist.
- macOS packaging, Gatekeeper, `open-url`, and the release workflow's macOS
  job are verified only on a macOS runner or machine.
- On macOS without the app, `servef .` has already added its roots to the
  daemon before `open` fails; they persist as other daemon roots do.
- Deferred to later layers: OS context menus, login item, reveal and
  open-in-editor bridge actions, root removal, signing, auto-update.
