# Releases

servef ships only as GitHub release assets, installed with the scripts in
`scripts/` (decisions 0073, 0079). No branch or tag carries built files; tags
point at plain `main` commits. Homebrew and `go install` are not supported.

`.github/workflows/build.yml` builds every asset for one commit and version.
Two workflows call it:

- `release.yml`: pushing a `vX.Y.Z` tag publishes that stable release.
- `ci.yml` (**Edge**): every push to `main` replaces the `edge` prerelease.

## Stable releases

No repository secrets are needed; publishing uses the workflow's
`GITHUB_TOKEN`.

1. Merge release-ready code to `main`. Ordinary checks run locally before
   push; run `mise run check:full` before releasing for race tests and docs
   validation too.
2. Pick the tag: `go run ./tools/release-version` prints the next patch in
   the `.release-version` series (or the tag already on `HEAD`).
3. Tag a `main` commit and push the tag:

   ```sh
   git fetch origin
   git tag v0.1.7 origin/main
   git push origin v0.1.7
   ```

4. `check` rejects a tag that is not `vX.Y.Z` or whose commit is not on
   `main`, and skips the build when the release already exists.
5. `build` (`build.yml`):
   - `cli` builds the embedded frontend and the four Go archives stamped with
     the version, commit, and commit date, and smoke tests the linux/amd64
     binary.
   - `electron-linux` (Ubuntu) and `electron-macos` (macOS) package the desktop
     app per arch with `apps/electron/scripts/package.ts`, bundling the
     matching archive's `servef`. Each smoke tests one package: Linux
     extracts the amd64 AppImage and deb and checks the bundled binary's
     version, `app.asar`, the deb's `servef://` handler, and its file manager
     actions; macOS checks the arm64 app's binary, `CFBundleURLSchemes`, and
     folder document type.
   - `assets` checks the exact asset set and writes `checksums.txt`.
6. `publish` creates the GitHub Release for the tag with generated notes and
   marks it latest.

Published releases are never rebuilt or replaced: `check` and `publish` both
skip an existing release, so re-running failed jobs is safe. To redo a
release whose publish failed before the release existed, re-run the
workflow; to replace a published one, publish a new patch version.

## Edge prerelease

After every push to `main`, the **Edge** workflow runs `build.yml` with
version `0.0.0-edge` and `macos-arches: arm64` (no macOS Intel, decision 0093).
When every build job passes, `publish` moves the `edge`
tag to the pushed commit, creates or edits the `edge` release (prerelease,
never latest, notes name the commit), uploads the assets with `--clobber`
(same names every time, so each is replaced in place), and deletes assets
no longer built. A newer push cancels an older build; publishing is never
cancelled, and an older run skips publishing when `edge` already has a newer
commit. The first green push creates the prerelease.

Edge binaries report `servef 0.0.0-edge <commit> <date>`; the desktop app
reports `0.0.0-edge` and links the latest stable release.

## Assets

Each release and `edge` publishes `checksums.txt`, four archives, and eight
desktop packages, where `<version>` is the tag without its leading `v`, or
`0.0.0-edge`:

- `servef_<version>_<darwin|linux>_<amd64|arm64>.tar.gz`
- `servef-electron_<version>_darwin_<amd64|arm64>.dmg` and `.zip`
- `servef-electron_<version>_linux_<amd64|arm64>.AppImage` and `.deb`

`edge` omits every `darwin_amd64` asset (decision 0093); the edge install
scripts stop on macOS Intel and point at the stable ones.

The native app (decision 0079, phase 5) adds its packages to this set, a job
to `build.yml`, and `install-native.sh` and `install-native-edge.sh`.

The install scripts download these names:

| Script | Installs | From |
| --- | --- | --- |
| `install.sh` | CLI | the release `releases/latest` redirects to |
| `install-electron.sh` | CLI and desktop app | the same release |
| `install-edge.sh` | CLI | `releases/download/edge/` |
| `install-electron-edge.sh` | CLI and desktop app | `releases/download/edge/` |

`install-desktop.sh` and `install-desktop-edge.sh` hand over to the
`install-electron` scripts, because earlier builds print their URLs (decision
0092).

The desktop scripts fetch the Linux icon from the tag
(`apps/electron/resources/icon.png`, `apps/desktop/resources/icon.png` for tags
before decision 0092, or `desktop/resources/icon.png` for tags before decision
0078). Renaming an asset or moving the icon breaks them for
every user at once.

Each archive contains the native `servef` binary, README, and license. The
frontend is built into `apps/servef/internal/web/dist/` before Go
compilation and embedded in the binary. Node.js and pnpm are build-time
dependencies only; installed releases need no JavaScript runtime, external
assets, or sidecar server.

Desktop packages bundle the same Go binary as the archive for their OS and
arch, at `resources/bin/servef` (not on `PATH`), plus Electron. macOS builds
are ad-hoc signed, not notarized, so Gatekeeper blocks the first launch of a
browser-downloaded package until the user allows it. There is no
auto-update; upgrade by running the install script again. To package locally
for the host OS and arch, run `mise run package:electron` (installers) or
`mise run package:electron:dir` (unpacked app); output lands in
`apps/electron/dist/`.

For development installs from `edge`, see the
[development guide](development.md#install-from-main).

To inspect the Linux packages, run `mise run package:electron`, then list the
deb (`dpkg-deb -c apps/electron/dist/servef-electron_0.0.0-dev_linux_$(go env GOARCH).deb`,
or `bsdtar -tf` on its `data.tar.xz`). It must contain
`usr/share/kio/servicemenus/servef-electron.desktop`,
`usr/share/nemo/actions/servef-electron.nemo_action`, and one entry under
`usr/share/applications/` without `inode/directory`; `desktop-file-validate`
checks that entry (not the KDE service menu). electron-builder's bundled fpm
needs `libcrypt.so.1` (Arch: `libxcrypt-compat`), `xz`, and `ar`.

The install scripts share code between `# shared:<name>:begin/end` markers;
edit every copy. `mise run test:release` runs shellcheck and
`tests/install-scripts.test.ts`, which checks the copies match and runs each
script against stub tools.

## Docs site deploy

The site (`apps/site`) deploys as plain static files through Cloudflare Pages
Git integration (decision 0074). Project settings:

| Setting | Value |
| --- | --- |
| Root directory | repository root (CF reads `.node-version`, Node 26) |
| Build command | `pnpm install --frozen-lockfile --filter @servef/site... && pnpm build:site` |
| Build output directory | `apps/site/dist/public` |
| Environment variables | `SKIP_DEPENDENCY_INSTALL=1`; `PNPM_VERSION=11.24.0` if the build image ignores `packageManager` |

The filtered install skips the web and desktop workspaces. `pnpm build:site`
fails on dead internal links. Cloudflare serves `404.html` for unknown paths.
There is no `wrangler.toml`: Pages build settings live in the dashboard.

## Version series

`.release-version` contains the active `major.minor` release series, which
`go run ./tools/release-version` reads. For example, `0.1` selects `v0.1.0`
when the series has no releases, then `v0.1.1`, `v0.1.2`, and so on; when a
release tag already names `HEAD`, it prints that tag.

To begin a new minor or major series, change `.release-version` in the repo.
Changing it to `0.2` makes the next release `v0.2.0`; changing it to `1.0`
makes the next release `v1.0.0`. Later releases continue incrementing that
series' patch number. The workflow accepts any `vX.Y.Z` tag on `main`; the
series only suggests the next one.
