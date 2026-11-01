# 0058: Explicit local Dagger validation before PR work

- Status: Accepted
- Date: 2026-11-01
- Supersedes: 0054 and 0057 check execution, locking, and pre-push policy

## Decision

Use mise as the command interface and Dagger for isolated local Linux execution.
Validate committed HEAD with a clean tracked tree before creating or updating a
PR. Preserve CI, edge publishing, releases, and deployment workflows. Linux
aggregate coverage qualifies; native platform checks remain optional supplements.

Keep five coarse groups: Go, frontend, tooling, docs, browser. Keep leaf commands
in mise. Prepare immutable dependencies and frontend assets once, then provide
private writable state for each consumer. Go and browsers consume the embedded
UI build. Tests, browsers, and fixture servers run inside the isolated environment.
No host application ports, service tunnels, networking, or Docker socket mounts.

Use content manifests for successful-result reuse, including source/tests,
fixtures, locks, configurations, implementation, toolchains/images/platform,
arguments and environment. Commit hashes record provenance only. Failed,
cancelled, partial, and incomplete attempts cannot establish aggregate success.
Force reruns bypass result reuse and underlying Go test-result caching.

Replace the repository-wide Git-directory lock with bounded user-level admission
and private run state. Bound internal workers independently from graph structure.
Retain unique logs, browser traces, and failure diagnostics until explicit pruning.

Use an explicitly managed engine outside the `dagger-engine-*` namespace, a
dedicated named state volume, explicit connection, and disabled automatic cleanup.
During migration, default bootstrap removed another repository's different engine
version. Engine provisioning and lifecycle therefore form part of isolation;
containerized tests alone do not prevent cross-repository engine interference.

Remove mandatory full checks from pre-push once parity and isolation acceptance
pass. `validate`, `check`, and `check:full` mean complete validation. Targeted
commands have identical isolation, but partial coverage. PR wrappers check HEAD
again and push the tested commit. They are bypassable, as are agent instructions;
there is no standard pre-PR hook and no new remote gate.

## Consequences

Local Docker/Dagger is required for qualifying validation. Linux containers omit
native macOS coverage. Source boundaries and artifact retention remain explicit
repository responsibilities; Dagger does not infer them. Shared immutable layers
reduce preparation cost while private attempts avoid state and port collisions.
See [validation](../validation.md) for commands and operation.
