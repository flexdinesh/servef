# Releases

## Stable releases

Stable versions and the latest GitHub Release are published only by manually running the **Release** workflow from `main`. The workflow checks out the latest `main` when its `prepare` job starts, and later jobs use that commit; dispatches from other branches are skipped.

Install the latest stable release or a specific version:

```sh
go install github.com/flexdinesh/servef@latest
go install github.com/flexdinesh/servef@v0.1.3
```

Required repository secret (desktop packaging adds none):

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge release-ready code to `main`.
2. Run the **Release** workflow on `main`. It requires no inputs.
3. `prepare` selects the next patch version, builds the embedded frontend, commits it on top of `main` as `release: vX.Y.Z`, builds the Go archives from that commit, and smoke tests the linux/amd64 binary. Ordinary checks run locally before push; run `mise run check:full` before releasing for race tests and docs validation too. `main` never tracks `internal/web/dist/`.
4. `desktop-linux` (Ubuntu) and `desktop-macos` (macOS) package the desktop app per arch with `desktop/scripts/package.ts`, bundling the matching archive's `servef`. Each smoke tests one package: Linux extracts the amd64 AppImage and deb and checks the bundled binary's version, `app.asar`, and the deb's `servef://` handler; macOS checks the arm64 app's binary and `CFBundleURLSchemes`.
5. `publish` writes `checksums.txt` over every asset, pushes only the stable tag, and publishes the GitHub Release as latest. The release commit reaches `publish` as a git bundle artifact, so the tag is the exact commit the binaries stamp.
6. `publish` generates `Formula/servef.rb` and opens or updates a pull request against `flexdinesh/homebrew-tap`.
7. Merge the tap pull request after its Homebrew checks pass.

Each release publishes `checksums.txt`, four archives, and eight desktop packages, where `<version>` omits the leading `v`:

- `servef_<version>_<darwin|linux>_<amd64|arm64>.tar.gz`
- `servef-desktop_<version>_darwin_<amd64|arm64>.dmg` and `.zip`
- `servef-desktop_<version>_linux_<amd64|arm64>.AppImage` and `.deb`

`scripts/install.sh` and `scripts/install-desktop.sh` (decision 0073) install
the release `github.com/flexdinesh/servef/releases/latest` redirects to, by
these names, and fetch the desktop icon from the tag; renaming an asset or
moving `desktop/resources/icon.png` breaks them for every user at once.

Each archive contains the native `servef` binary, README, and license. The frontend is built into `internal/web/dist/` before Go compilation and embedded in the binary. Node.js and pnpm are build-time dependencies only; installed releases need no JavaScript runtime, external assets, or sidecar server.

Desktop packages bundle the same Go binary as the archive for their OS and arch, at `resources/bin/servef` (not on `PATH`), plus Electron. macOS builds are ad-hoc signed, not notarized, so Gatekeeper blocks the first launch until the user allows it. There is no auto-update; upgrade by installing the newer package. `edge` carries no desktop builds. To package locally for the host OS and arch, run `mise run package:desktop` (installers) or `mise run package:desktop:dir` (unpacked app); output lands in `desktop/dist/`.

The tap branch is deterministic per version, such as `servef-v0.1.0`. Rerunning a release whose tag is still built on current `main` (its commit's first parent) reuses the published artifacts and updates the same branch and pull request; desktop jobs are skipped once the release exists. If the tag was pushed but the release was not, a rerun rebuilds every asset from the tag. Published artifacts are not rebuilt or replaced: `publish` checks for the release again before creating it, so re-running only a failed `publish` job also reuses it. The workflow publishes the GitHub Release before updating the tap, so rerunning it can repair a failed tap update.

The tap repository owns Homebrew style, strict audit, install, and formula test checks before merge.

For development installs from `edge`, see the [development guide](development.md#install-from-edge).

## Version series

`.release-version` contains the active `major.minor` release series. For example, `0.1` selects `v0.1.0` when the series has no releases, then `v0.1.1`, `v0.1.2`, and so on. A rerun from the same commit reuses its existing tag and release.

To begin a new minor or major series, change `.release-version` in the repo. Changing it to `0.2` makes the next release `v0.2.0`; changing it to `1.0` makes the next release `v1.0.0`. Later releases continue incrementing that series' patch number.
