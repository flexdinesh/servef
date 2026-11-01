# Local validation

Validation runs on this machine through Dagger. Linux aggregate validation
qualifies for PR preparation; native macOS checks are optional supplements.
CI edge publication and release workflows remain independent.

## Engine ownership

Install tools with mise. Validation connects explicitly to
`servef-validation-engine-v0.21.10`. It must never fall back to Dagger's automatic
engine bootstrap: that bootstrap can remove another repository's engine.
Keep `DAGGER_LEAVE_OLD_ENGINE=true` as defense in depth.

Provision the engine once with a named volume and the pinned engine image:

```sh
docker volume create servef-validation-engine-v0.21.10-state
docker run -d --name servef-validation-engine-v0.21.10 --privileged \
  --cpus 8 --memory 12g --memory-swap 12g \
  -v servef-validation-engine-v0.21.10-state:/var/lib/dagger \
  registry.dagger.io/engine@sha256:2a7c054e0864e297671c24a0726ebf1c9747d70291b8843b1726daaff200a334
```

Inspect an existing container before provisioning. Never replace another
owner's engine or delete its volume. These limits suit a machine with at least
8 available CPUs and 16 GiB RAM; reduce the engine ceiling on smaller machines
and keep room for other repositories. Checks themselves use two Go/Node/browser
workers and one expensive group per invocation. A two-slot user-level admission
pool bounds concurrent invocations across worktrees and repositories using this
wrapper. This is admission control, not a host-wide CPU scheduler.

The host engine needs its container runtime. Check containers receive no Docker
socket, host networking, published application ports, or host-service tunnels.
Upgrades require an idle engine and an explicit version/digest change. Never
stop the engine to cancel a single validation run or prune it automatically.

## Commands

Commit changes first; validation rejects staged or unstaged tracked changes.
Untracked files, local `.env`, dependencies, and generated build outputs are not
part of the snapshot. Tests run against tracked committed content only.

```sh
mise run validate
mise run validate -- --group frontend
mise run validate -- --group browser -- tests/browser/live-updates.spec.ts
mise run validate -- --force
mise run validate:status
mise run pr:create -- --ready --title "fix: example" --body-file /path/to/body
mise run pr:update
mise run validate:prune
```

`check` and `check:full` mean validate everything. Public targeted check tasks
use the same container isolation. Hidden `_validate:*` tasks are container
implementation details. Direct native commands remain useful during editing,
but do not establish the committed snapshot's aggregate coverage.

| Group | Coverage |
| --- | --- |
| go | Go formatting, tidy, vet, ordinary and race tests |
| frontend | Web tests, typecheck, oxlint, stylelint |
| tooling | Runner/release tests, root TypeScript, mise validation |
| docs | Site typecheck and production build, including internal links |
| browser | Chromium integration tests and private fixture servers |

Go tests include the host validation harness. Root TypeScript dependencies and
Ruby are installed so native TypeScript navigation and formula syntax checks do
not silently skip. Git behavior tests construct independent fixture repositories.

## Inputs and reuse

The wrapper exports committed Git objects, preserving modes and symlinks and
rejecting unsupported submodules. It never copies a worktree's `.git` pointer.
Each group receives a conservative source subset. Go/browser include frontend
inputs because their embedded UI depends on the web build; frontend consumes
shared API and family fixtures. Shared workspace lockfile changes invalidate
all pnpm consumers. Docs content does not invalidate application checks.

Content manifests include source/tests/configuration, dependency locks/patches,
task and validation implementation, toolchain/image/platform identity, selected
arguments, and behavior-affecting environment. HEAD and branch names identify
provenance, not cache validity. A history-only rebase can reuse matching results;
upstream content changes invalidate affected groups.

Dagger reuses immutable preparation/build layers. Local success receipts reuse
complete successful group results. Every executed attempt gets a fresh nonce,
so failed attempts and mutable fixture executions cannot be replayed as success.
Only successful checks with exported artifacts publish success receipts.
Selections remain partial coverage. Aggregate success requires every complete
group. `--force` bypasses receipts and build-result reuse; Go tests use
`-count=1`. Download/compiler preparation caches are not test-result evidence.

No mutable home, temporary directory, browser profile, walkthrough store, or
fixture repository is shared between executions. Dependencies and build outputs
are prepared immutably, then consumed through private container filesystems.
Behavior environment is explicit; host service URLs and personal config are
not imported into runners.

## Failures and cancellation

Each invocation records tested HEAD, input identities, platform/environment,
coverage, execution/reuse provenance, task status, and artifact paths. Logs,
browser traces/screenshots, and useful failure fixtures remain outside shared
checkout output paths. Read the reported log before rerunning a failed group.

SIGINT/SIGTERM stops only the invocation's process groups/session. Graceful
cleanup attempts artifact export; hard termination or engine failure may leave
only streamed logs. Such runs never qualify as successful. Keep artifacts until
explicit pruning; completed artifacts older than 30 days are eligible, while
active runs and compact success receipts are retained.

PR wrappers validate, recheck tracked cleanliness and HEAD, push the captured
commit explicitly, and verify the remote SHA before creating a PR. Credentials
remain on the host. There is no standard Git pre-PR hook: wrappers and agent
instructions are bypassable. No remote enforcement is introduced.

Linux does not prove macOS kqueue behavior, native filesystem/process semantics,
or macOS browser behavior. Run the relevant native commands explicitly when
investigating those concerns; their coverage is separate from the Linux result.
