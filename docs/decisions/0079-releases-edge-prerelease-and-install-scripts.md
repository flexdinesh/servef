# 0079: Releases, the edge prerelease, and install scripts only

- Status: Accepted; script and asset names revised by [0092](0092-electron-app-name.md); edge assets by [0093](0093-no-macos-intel-on-edge.md)
- Date: 2026-11-03
- Supersedes: [0021](0021-built-frontend-on-release-tags-and-edge.md)
- Revises: [0068](0068-cli-opens-desktop-app-and-desktop-releases.md)
  (Releases; the Homebrew cask), [0073](0073-install-scripts.md) (edge
  scripts; Homebrew and `go install`),
  [0070](0070-desktop-ssh-connections.md) (`servefMissing` instructions),
  [0074](0074-landing-page-and-cloudflare-pages.md) (install tabs)

## Context

0021 committed the built frontend on release tags and a force-pushed `edge`
branch so `go install` worked. Since the `apps/` layout (0078) the Go module
is no longer at the repository root, so `go install github.com/flexdinesh/servef`
works only for older tags, and `@edge` no longer builds. The Homebrew tap
needed a cross-repository token and a pull request per release. The edge
scripts (0073) cloned `edge` and built on the target machine with Go or
mise, taking minutes and supporting only `mise.lock`'s platforms for the
desktop app. Releases ran from a manual dispatch that created a release
commit and handed it between jobs as a git bundle.

## Decision

- servef ships only as GitHub release assets installed by the scripts in
  `scripts/`. Homebrew and `go install` are not supported install paths; the
  tap job, `scripts/generate-homebrew-formula.ts`, its tests, and
  `HOMEBREW_TAP_TOKEN` are removed. Built files are never committed to any
  branch or tag; `main` ignores `apps/servef/internal/web/dist/`.
- `.github/workflows/build.yml` is a reusable workflow that builds every asset
  for one commit and version: CLI archives (linux and darwin, amd64 and
  arm64), Electron packages (Linux AppImage and deb, macOS dmg and zip, per
  arch), smoke checks, and `checksums.txt` over the exact expected set.
- Stable: pushing a `vX.Y.Z` tag that names a `main` commit runs
  `release.yml`, which builds and publishes that release as latest. Tags
  point at plain `main` commits. A published release is never rebuilt or
  replaced. `tools/release-version` only suggests the next tag from
  `.release-version`.
- Edge: every push to `main` runs `ci.yml`, which builds the same asset set
  stamped `0.0.0-edge` (the commit and date stamped as for releases) and,
  once every build job passes, moves the `edge` tag to the pushed commit and
  replaces the assets of the single `edge` prerelease (never latest). Asset
  names are fixed per version, so uploads replace assets in place and names
  no longer built are deleted. A newer push cancels an older build; an older
  run never publishes over a newer commit. A macOS runner per push is
  accepted.
- Version reporting: stable builds report `X.Y.Z`; edge builds report
  `0.0.0-edge` and the commit; local package builds keep `0.0.0-dev`.
- Install scripts, one per part, no flags, served from GitHub raw `main`:
  `install.sh` (CLI) and `install-desktop.sh` (CLI and desktop app) from the
  release `releases/latest` redirects to; `install-edge.sh` and
  `install-desktop-edge.sh` from `releases/download/edge/`; `uninstall.sh`.
  Edge scripts download prebuilt assets exactly like the stable ones (no git,
  Go, or mise) and support every published platform. Download and install
  code is shared through `# shared:<name>:begin/end` blocks (`release`,
  `desktop-install`); only `latest` (stable) and `edge` differ.
- `uninstall.sh` still removes copies in Go's install directory and prints
  removal commands for Homebrew and mise copies, which earlier installs left.
- The desktop app's `servefMissing`, `servefTooOld`, and `mismatch`
  instructions offer the install script instead of Homebrew and Go. The
  landing page's install tabs are the CLI and desktop scripts.
- The native app (`apps/native`) will ship in stable and edge too. Phase 5
  adds its packaging job and assets at the marked extension point in
  `build.yml`, plus `install-native.sh` and `install-native-edge.sh`.

## Consequences

- Pinning a version means downloading that release's archive by hand; the
  scripts install only the latest release or `edge`.
- Every push to `main` spends Linux and macOS runner time; `edge` lags `main`
  by one build.
- Tags no longer contain the frontend, so building a tag from source needs
  `pnpm build:web` like any other commit.
- The release workflow cannot be run locally; it is checked with actionlint,
  and a real tag and the first green push to `main` after this change prove
  it. That push creates the `edge` prerelease.
- The `edge` branch from 0021 is no longer updated; delete it so `edge` names
  only the tag.
