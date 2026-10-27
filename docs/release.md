# Releases

## Stable releases

Stable versions and the latest GitHub Release are published only by manually running the **Release** workflow from `main`. The workflow checks out the latest `main` when its job starts; dispatches from other branches are skipped.

Install the latest stable release or a specific version:

```sh
go install github.com/flexdinesh/servef@latest
go install github.com/flexdinesh/servef@v0.1.3
```

Required repository secret:

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge release-ready code to `main`.
2. Run the **Release** workflow on `main`. It requires no inputs.
3. The workflow selects the next patch version, builds the embedded frontend, commits it on top of `main` as `release: vX.Y.Z`, builds release archives from that commit, then pushes only the stable tag and marks the GitHub Release as latest. Ordinary checks run locally before push; run `mise run check:full` before releasing for race tests and docs validation too. Release keeps its packaged-binary smoke test. `main` never tracks `internal/web/dist/`.
4. It generates `Formula/servef.rb` and opens or updates a pull request against `flexdinesh/homebrew-tap`.
5. Merge the tap pull request after its Homebrew checks pass.

Each release publishes `checksums.txt` and four archives, where `<version>` omits the leading `v`:

- `servef_<version>_darwin_amd64.tar.gz`
- `servef_<version>_darwin_arm64.tar.gz`
- `servef_<version>_linux_amd64.tar.gz`
- `servef_<version>_linux_arm64.tar.gz`

Each archive contains the native `servef` binary, README, and license. The frontend is built into `internal/web/dist/` before Go compilation and embedded in the binary. Node.js and pnpm are build-time dependencies only; installed releases need no JavaScript runtime, external assets, or sidecar server.

The tap branch is deterministic per version, such as `servef-v0.1.0`. Rerunning a release whose tag is still built on current `main` (its commit's first parent) reuses the published artifacts and updates the same branch and pull request. Published artifacts are not rebuilt or replaced. The workflow publishes the GitHub Release before updating the tap, so rerunning it can repair a failed tap update.

The tap repository owns Homebrew style, strict audit, install, and formula test checks before merge.

For development installs from `edge`, see the [development guide](development.md#install-from-edge).

## Version series

`.release-version` contains the active `major.minor` release series. For example, `0.1` selects `v0.1.0` when the series has no releases, then `v0.1.1`, `v0.1.2`, and so on. A rerun from the same commit reuses its existing tag and release.

To begin a new minor or major series, change `.release-version` in the repo. Changing it to `0.2` makes the next release `v0.2.0`; changing it to `1.0` makes the next release `v1.0.0`. Later releases continue incrementing that series' patch number.
