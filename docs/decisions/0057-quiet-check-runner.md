# 0057: Quiet check runner and targeted agent checks

- Status: Accepted
- Date: 2026-10-31
- Supersedes: [0054](0054-worktree-checks.md)'s check output; its lock,
  concurrency, and browser coverage stand

## Context

Agents read every byte a check prints, and that output is re-sent on every
later turn. `check:push` printed about 55 KB, mostly one line per passing Node
and Playwright test; the docs build added about 77 KB of chunk tables. Agent
instructions also pointed at full checks for every change, so a push could
run the suite twice: once by hand and once in the hook.

## Decision

- `check`, `check:push`, and `check:full` run `scripts/check.ts` with
  `:::`-separated groups of mise tasks; `mise.toml` still names every task.
  Groups run in order; each group runs its tasks in a `MISE_JOBS` pool.
- Each task writes stdout and stderr to `.dev/check-logs/NN-<task>.log`,
  cleared per run. The terminal gets `ok <task> 4.2s` per task. A failure
  prints its last 100 lines, its log path, and `mise run <task>`; its group
  finishes, later groups are skipped, and the first failing code is the exit
  code. `SERVEF_CHECK_VERBOSE=1` streams output instead of writing logs.
- Builds and reporters print only what needs attention: Vite builds the app at
  `--logLevel warn`, Node tests use the dot reporter, and Playwright uses dots
  when stdout is not a terminal. Vocs has no quiet flag; its log keeps it.
- `build:site` declares its sources and outputs and skips when unchanged.
- The pre-push hook is the full-check gate. Agents run the narrowest command
  for what they changed, rerun only a failed task, and wait for background
  checks to finish instead of polling.

## Consequences

- A successful full check prints well under 1 KB; failure detail stays whole
  in the log.
- mise's dependency graph no longer schedules checks. Leaf tasks run with
  `--skip-deps`, so a task gaining a dependency must be grouped after it.
- Direct `node --test` runs print dots in terminals too.
