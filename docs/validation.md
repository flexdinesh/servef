# Local validation

Use this machine for validation before PR creation and updates. All tests and
their prerequisite builds run in Dagger. Safe static checks run on the host
against private snapshots. Development commands and formatting stay on the host.
Linux test coverage qualifies; native macOS checks are optional supplements.
CI edge publication and release workflows remain independent.

## Commands and PR workflow

While editing, targeted tasks capture immutable current edits and relevant
unignored new files. They exclude personal environment files, dependencies, and
generated outputs. Targeted results never qualify a PR, even when they pass.

```sh
mise run test:go
mise run test:race
mise run test:web
mise run test:release
mise run test:browser -- tests/browser/live-updates.spec.ts
mise run test:desktop
mise run test:desktop -- test/smoke/app.spec.ts
mise run lint
mise run lint:desktop
mise run typecheck
mise run fmt:check
mise run tidy:check
mise run mise:check
```

Before creating or updating a PR, commit changes and validate clean committed
HEAD across all eleven groups. Verify status immediately before the separate
push/PR operation:

```sh
mise run check:pr
mise run check:status
```

Confirm HEAD still matches the reported tested commit. Tracked changes or a
changed HEAD require reassessing validation before publication. `check:status`
compares relevant inputs with successful certificates; it runs no tests, builds,
or engine startup. Validation never creates or updates a PR. Local conventions
are bypassable; Git has no standard pre-PR hook and no remote enforcement is
introduced. There is no mandatory pre-push suite.

`check`, `check:full`, and `validate` alias `check:pr`. Select a committed group
with `mise run check:pr -- --group docs`. Use `mise run check:pr -- --force` to
execute checks and prerequisite builds again. `mise run check:prune` manages
completed artifacts.

Public mise tasks invoke the coordinator. Hidden `_host:*` and `_container:*`
tasks contain leaf commands and never invoke public wrappers. This prevents
container execution from recursively launching the host coordinator.

| Group | Location | Coverage |
| --- | --- | --- |
| repo | Host | Go formatting, module tidy diff, mise validation |
| web-static | Host | Web typecheck, oxlint, stylelint |
| tooling-static | Host | Root TypeScript check for scripts and tests |
| site-static | Host | Site TypeScript check |
| desktop-static | Host | Desktop TypeScript check and oxlint |
| go | Dagger | Vet, ordinary Go tests, race tests |
| frontend | Dagger | Frontend, mock-server, and fixture-contract tests |
| tooling | Dagger | Release, runner, and browser harness tests |
| docs | Dagger | Production site build and internal-link validation |
| browser | Dagger | Chromium integration tests and fixture servers |
| desktop | Dagger | Desktop unit tests, bundle, and Electron smoke under Xvfb |

Go vet stays in Dagger because it needs the embedded frontend build. Tests of
architecture or validation tooling remain tests and run there too. Git, Ruby,
and repository-local TypeScript are prerequisites: missing tooling must fail
preparation rather than silently skip formula or navigation coverage.

### Desktop

`desktop-static` reads `desktop/` and the shared `web/src/api/` contracts.
`desktop` also reads the server sources (`cmd/`, `internal/`, root Go files),
`web/`, and `testdata/`: its smoke drives the real daemon. pnpm never downloads
Electron (`allowBuilds: electron: false`). Desktop preparation alone runs
`desktop/node_modules/electron/install.js`, which verifies the archive against
the package's checksums, installs Xvfb and Electron's shared libraries from the
pinned Debian snapshot, and fails if `ldd` reports a missing library. It builds
the embedded UI and the servef binary as the browser group does.

`_container:test:desktop` runs the unit tests, bundles the app, then runs the
Playwright `_electron` smoke under `xvfb-run -a` with `--no-sandbox` (the
container runs as root). Each run gets its own short temporary socket, roots
file, user data, walkthrough cache, notification log, and git fixtures; nothing
reaches the user's daemon. Traces, window screenshots, and daemon, app, and
notification logs survive failures under the group's `playwright/` output.

The smoke covers Linux only. Headless Chromium reports hidden pages and closes
the UI's live stream, so the smoke always needs a display: Xvfb in Dagger, or a
real or virtual compositor on a host (`SERVEF_DESKTOP_BINARY`, plus
`SERVEF_DESKTOP_ELECTRON_ARGS=--ozone-platform=wayland` under Wayland). macOS
shell behavior stays a manual check (`mise run dev:desktop`), as does
`mise run measure:desktop`, which prints Electron, daemon, and language-server
memory for decision 0067 and gates nothing.

## Engine ownership

Install pinned tools through mise. Reuse `servef-validation-engine-v0.21.10`
across this repository's worktrees, with the dedicated named volume
`servef-validation-engine-v0.21.10-state`. CLI, SDK, and engine use 0.21.10.
Initialization and execution connect directly with:

```sh
export _EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container://servef-validation-engine-v0.21.10
export DAGGER_LEAVE_OLD_ENGINE=true
```

Inspect existing containers and volumes before setup. Leave unclear ownership
untouched. Validation never provisions an engine automatically; a missing,
stopped, or incompatible engine produces an error. Never remove, stop, restart,
or prune another task's engine or volume. Do not use broad prune commands.

Only when explicitly provisioning a new repository-owned engine, use:

```sh
docker volume create servef-validation-engine-v0.21.10-state
docker run -d --name servef-validation-engine-v0.21.10 --privileged \
  --cpus 8 --memory 12g --memory-swap 12g \
  -v servef-validation-engine-v0.21.10-state:/var/lib/dagger \
  registry.dagger.io/engine@sha256:2a7c054e0864e297671c24a0726ebf1c9747d70291b8843b1726daaff200a334
```

The name deliberately avoids `dagger-engine-*`. Explicit connection and
`DAGGER_LEAVE_OLD_ENGINE=true` prevent automatic bootstrap cleanup from
interfering with other repositories. Upgrades and cleanup require coordinated
ownership, an idle engine, and an explicit compatible version/digest change.

The existing 8-CPU/12-GiB ceiling suits this shared machine. Do not increase it
automatically. Two cooperative user-level execution slots bound participating
repositories and worktrees. Each invocation runs one expensive group at a time,
with two Go/Node/browser workers, and releases capacity after each group. Acquire
the input-certificate lock before waiting for machine capacity. These limits do
not control unrelated tools or guarantee freedom from host memory pressure.

## Source and execution isolation

Committed snapshots come from Git objects, preserving paths, modes, and safe
symlinks. Unsupported submodules and escaping symlinks fail. Never copy a
worktree's `.git` pointer. A separate synthetic baseline supplies tracked-file
enumeration and generated-file drift detection; Git behavior tests create their
own independent repositories.

Dagger prepares immutable toolchains, frozen pnpm dependencies, Go modules,
frontend assets, and the application binary. Consumers receive private writable
container state. Browser clients, Vite, application servers, and mutable fixture
repositories run together inside the execution container. Mutating browser
tests retain their own fixtures. No application ports are published, and runners
receive no host networking, host-service tunnels, or host Docker socket. Future
mutable Dagger services need invocation-specific identities and private storage.

Host static checks also use private materialized snapshots under
`.dev/validation/`. Their home, temporary, output, and dependency state is
worktree-local. Frozen dependency preparation is published atomically and copied
without shared writable hardlinks. Formatting remains an explicit operation on
the working tree; do not format concurrently with validation of that checkout.

Both environments allowlist behavior inputs. Personal configuration, host
service URLs, `.env`, and existing build outputs do not enter validation.

## Inputs, reuse, and certificates

Each group receives a conservative source subset. Go/browser consume embedded
UI inputs; frontend tests also consume API/family fixtures and the walkthrough
schema. Shared workspace locks invalidate their consumers. Site content does
not invalidate application tests. Toolchain preparation is separate from task
commands, so unchanged toolchains can survive a task edit.

Input identities include relevant contents, paths, modes, symlinks, manifests,
locks, patches, configuration, implementation, toolchain/image/browser identity,
platform, arguments, and behavior-affecting environment. Host static certificates
are platform-specific. Commit and branch names record provenance, not validity.
A history-only rebase can reuse matching certificates; relevant upstream changes
invalidate affected groups. No elapsed-time or small-diff rule establishes success.

Dagger caches immutable preparation/build layers. Local certificates record
successful complete check coverage. A per-input execution lock coordinates
concurrent certification and rechecks reuse before doing work. Starting a real
rerun invalidates old success and marks the input pending. Only completed checks,
drift detection, and artifact export publish a new certificate. Failure or active
cancellation leaves that input uncertified until a successful rerun. Cancellation
while waiting does not revoke prior proof. Older concurrent work cannot restore
stale success.

`--force` bypasses certificates and cached check/build execution; Go tests use
`-count=1`. Dependency download preparation may remain reusable. Reusable build
outputs are not successful test evidence. Aggregate success requires every
complete group plus a final cancellation, HEAD, and tracked-cleanliness check.

## Failures, artifacts, and debugging

Each invocation records HEAD/tree or current-edit provenance, input identities,
environment, coverage, execution/reuse, result status, and artifact locations.
Logs use unique run directories, not a shared directory cleared by another run.
Read the reported failure log and use the printed targeted rerun command.
Browser traces, screenshots, and useful failure fixtures survive failed checks.

SIGINT/SIGTERM stops only owned process groups and the invocation's Dagger
session, using TERM then bounded KILL. Never stop an engine to cancel validation.
Graceful cleanup attempts artifact export; hard termination or engine failure
may leave only streamed logs and incomplete records, never certified success.

`check:prune` defaults to 30 days and 10 GiB of artifacts per worktree, removing
oldest completed runs first. Active-run locks protect live output; PID guesses
are insufficient. Compact certificates remain independent of artifact retention.

Linux tests do not establish macOS kqueue, filesystem, process, or browser
coverage. Native checks are optional, explicitly chosen supplements for those
concerns. Routine testing stays in Dagger. Container paths and retained traces
replace reliance on host-local fixture servers during debugging.
