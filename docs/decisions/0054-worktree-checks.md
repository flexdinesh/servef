# 0054: Queued worktree checks and critical browser coverage

- Status: Accepted
- Date: 2026-10-27
- Supersedes: 0032 and 0038 check execution and browser port policy

## Context

Parallel agents in linked worktrees contend for CPU when every push runs race
tests, builds, and an uncapped browser suite. Fixed browser ports also collide
when Linux network namespaces are unavailable. The CLI already accepts an
explicit port, including zero for an OS-assigned listener.

## Decision

- Keep the existing CLI port policy. Document `--port 0 --no-open` for agents
  and test that simultaneous servers serve separate roots and stop independently.
- Ordinary checks run in pre-push, not CI. Keep edge publishing and release
  builds, archive smoke testing, and formula validation.
- `check:push` runs cheap semantic/static checks, then critical browser paths.
  `check:full` explicitly adds race tests and the docs production build. Go
  tests compile all packages; do not duplicate that with a separate build.
- Both commands acquire one advisory OS lock in the shared Git directory on
  Linux/macOS. Keep the descriptor across exec and let process exit release it.
  Do not delete the lock file, which would allow another lock inode to bypass it.
  Separate repositories and development servers remain independent.
- Default mise and Go concurrency to two for queued checks, and cap Node test
  files and Chromium workers at two. Chromium runs after other checks.
- The browser runner owns three API servers, Vite, fixtures, and cleanup. Build
  the Go binary once. Bind port zero and pass the actual listener ports to the
  tests; retain explicit port overrides and Linux namespace protection.
- Keep browser-specific navigation, rendering, workers, focus, scroll, and live
  updates. Remove cosmetic checks and duplicate matrices covered by Go/Node.
  Record retained, redundant, and deliberately omitted cases in the browser
  test audit. Preserve useful cheap semantic tests.

## Consequences

- A queued push may wait longer, but linked worktrees cannot multiply the full
  check workload. The lock is not FIFO and does not coordinate separate clones
  or manually invoked individual tests.
- Race detection and broken docs links require an explicit full check. Bypassing
  hooks can publish unchecked changes; CI no longer provides a second check.
- Browser counts fall without claiming unit tests prove omitted interaction
  variants. Visual review remains necessary for cosmetic changes.
- The locking helper is development tooling for Linux/macOS, not part of the
  distributed servef binary. No runtime dependency is added to servef.
