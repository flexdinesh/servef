# 0059: Test isolation and safe host checks before PR work

- Status: Accepted
- Date: 2026-11-01
- Supersedes: 0058 command boundary, grouping, certification lifecycle, and PR wrappers

## Context

Validation needs to survive concurrent worktrees and repositories. Mandatory
pre-push suites repeat work after rebases and small follow-up changes. The first
Dagger implementation isolated all checks, required commits for editing checks,
and coupled validation with PR publishing. It also left an older successful
receipt reusable after a forced rerun failed.

## Decision

Run all tests in Dagger, including unit, integration, browser, and tooling tests.
Run prerequisite builds there too. Keep browser clients and fixture servers in
the same private execution container. Retain separate fixtures for tests that
mutate them. No host ports, networking, tunnels, or Docker socket enter runners.

Keep development and formatting on the host. Host static checks use private
worktree-local snapshots and writable state: repository formatting/tidiness/mise,
web types/lint, root tooling types, and site types. Go vet remains with Go tests
because it consumes the embedded frontend build. Dagger groups are Go,
frontend tests, tooling tests, docs build, and browser tests.

Use `mise run check:pr` for all nine groups against clean committed HEAD, and
`check:status` immediately before separate PR creation or updates. Publishing is
not part of validation. Targeted mise tasks capture immutable current edits and
relevant new files, but cannot certify PR readiness. `check`, `check:full`, and
`validate` remain aggregate aliases. Hidden `_host:*` and `_container:*` commands
never call public wrappers. Pre-push remains free of mandatory suite execution.

Reuse success only when complete relevant inputs match, including toolchains,
platform, configuration, implementation, and selected environment. Commit IDs
record provenance. History-only rebases can reuse evidence; relevant upstream
changes invalidate it. Dagger owns immutable preparation caching; local
certificates support cheap mixed host/container coverage checks.

Serialize certification per input identity, recheck reuse under its lock, then
acquire machine capacity. Before actual rerun, revoke old success and mark the
input pending. Failed, actively cancelled, and incomplete executions leave it
uncertified until a successful rerun. Cancellation while waiting does not revoke
prior proof. Publish success only after checks, drift detection, and artifact
export, with a final HEAD/cleanliness/cancellation check for aggregate validation.
Force reruns bypass certificates and check/build result reuse, including Go's
test-result cache. Dependency preparation may remain reusable.

Retain the explicitly owned 0.21.10 engine, dedicated named volume, direct
connection, and `DAGGER_LEAVE_OLD_ENGINE=true`. Never provision automatically or
change another task's engine. Coordinate upgrades and cleanup explicitly. Two
cooperative machine-wide slots are acquired per group and released afterward;
each invocation schedules one expensive group at a time with bounded workers.

Keep unique logs, failure fixtures, traces, and screenshots. Pruning defaults to
30 days and 10 GiB per worktree, oldest completed first. Active-run locks protect
live artifacts; certificates remain independent. Cancellation affects only owned
processes/session, never the engine.

## Consequences

Safe static checks avoid container startup but need separate pinned host
dependency preparation and platform-specific evidence. Linux tests qualify;
native macOS filesystem/process/browser checks remain optional supplements.
Resource admission controls cooperating commands, not every process on the
machine. Local conventions remain bypassable, with no standard Git pre-PR hook
or new remote gate. Existing CI, edge publication, and releases remain unchanged.
