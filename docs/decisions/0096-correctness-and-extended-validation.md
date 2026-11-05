# 0096: Correctness checks and optional extended validation

- Status: Accepted
- Date: 2026-11-05
- Supersedes: [0059](0059-validation-execution-boundaries.md) aggregate aliases and input selection; [0095](0095-desktop-app-is-mygo-electron-removed.md) resource measurements in the desktop gate

## Context

Routine validation repeats ordinary and race Go tests, parses a large browser
diff fixture during ordinary review workflows, and waits for desktop RSS to
settle. Dagger caches complete preparation steps but loses Go compilation work
when source changes. Broad source subsets invalidate unrelated checks.

## Decision

`check:pr`, `check`, and `validate` require the correctness suite: static checks,
contract drift, Go vet and one race-enabled test pass, frontend and tooling
tests, site validation, Chromium feature integration, and desktop correctness.
Keep targeted ordinary Go tests available while editing.

`check:extended` runs browser stress, desktop resource measurements, and
Playwright WebKit compatibility. Agents run it when changes affect diff
virtualization, responsiveness, memory, process lifecycle, browser engines, or
desktop rendering. `check:full` explicitly runs correctness and extended. An
extended failure is reported; it does not retroactively change correctness
coverage or turn an incomplete run into success.

Each extended group has its own certificate identity. `check:status` requires
correctness by default; `--suite extended` and `--suite full` inspect those
explicit coverages. Targeted or partial proof cannot qualify a complete group.

Keep tests of user workflows, cross-package contracts, subtle algorithms,
confinement, persistence, cancellation, and recovery. Delete tests that only
repeat static copy, incidental pixel values, constants, or trivial arithmetic.
Use a compact browser diff fixture for correctness and the full fixture for
stress. Preserve real hunk navigation and collapsed-gap reveal in correctness.

Persist Go compilation caches in Dagger across compatible builds and tests.
Names include the toolchain, platform, image, and system-library preparation;
desktop cgo builds have a separate identity. Keep frozen dependencies as
immutable preparation; cache pnpm downloads independently of lockfile changes.
Only caches designed for concurrent reuse are shared. Source, fixtures, writable
dependency trees, and application state remain private.

Certificates reuse completed checks against matching inputs. Compilation cache
hits alone never certify a check. Checks continue to execute with `-count=1`
when required; `--force` bypasses certificates and check/build execution layers
while allowing compiler and download caches. Attempt IDs stay after reusable
preparation. No engine provisioning, pruning, resource-limit increases, or new
remote validation gate.

Exclude unit tests from consumers that only build production code. Validator
tests invalidate their test consumer; validator implementation still invalidates
its consumers. Batch Git object reads preserve committed snapshot fidelity.

## Consequences

Cosmetic changes need visual review. Performance and compatibility regressions
need an explicit extended run. Input selection is a correctness boundary and
must retain fixtures, runtime configuration, generated contracts, and shared
tooling dependencies. Cache identity must change when cgo libraries change.
Existing cancellation, certificate revocation, artifact retention, engine
ownership, and concurrency rules remain.
