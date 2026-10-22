# 0032: Browser tests run before push, not in CI

- Status: Accepted
- Date: 2026-10-22

## Context

CI dropped browser tests to stay fast; only the manual release workflow still
ran them, and it ran Go tests before building the embedded frontend, which
`main` no longer tracks (0021). CI and release each listed their checks, and
the lists drifted.

## Decision

- `pnpm check` is the one command for every non-browser check: build the
  embedded frontend, Go vet/test/build, web tests, release-tooling tests,
  typecheck, and the docs build. CI and release run it and nothing more.
- Browser tests run locally. `pnpm install` sets `core.hooksPath` to
  `.githooks/`, whose `pre-push` hook runs `pnpm check` and
  `pnpm test:browser`.
- `pnpm test:browser` builds the embedded frontend first, and each run builds
  its git fixture in its own temporary folder.

## Consequences

- CI and release need no browser install and stay fast.
- A push with `--no-verify`, or from a clone that skipped `pnpm install`, can
  reach `main` and `edge` without browser coverage.
