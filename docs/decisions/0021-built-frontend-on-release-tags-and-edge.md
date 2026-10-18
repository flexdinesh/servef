# 0021: Built frontend on release tags and edge

- Status: Accepted
- Date: 2026-10-18

## Context

`go install` builds module source exactly as it is at a Git ref; it runs no
build steps. The Go binary embeds `internal/web/dist/`, so every installable
ref must contain the built frontend. `main` committed about 24 MiB of generated
assets, rebuilt and recommitted on every `web/` change, so that
`go install ...@main` worked.

## Decision

- `main` ignores `internal/web/dist/`. Building from source runs
  `pnpm build:web` before any Go build, test, or vet.
- The manual Release workflow commits the built frontend on top of the
  released `main` commit and tags that commit. Only the tag is pushed; `main`
  never contains it. A tag belongs to the `main` commit that is its first
  parent; reruns reuse it.
- After CI passes on a push to `main`, CI force-pushes `edge`: one commit with
  `main` as its first parent plus the built frontend. When the latest release
  tag is not an ancestor of `main`, the release commit is a second parent, so
  Go pseudo-versions on `edge` sort after the latest release.
- Development installs use `go install ...@edge`; stable installs keep
  `@latest` and `@vX.Y.Z`.

## Consequences

- `go install ...@main`, `@<main sha>`, and feature branches no longer build.
- Fresh clones and new worktrees fail Go builds until `pnpm build:web` runs.
- CI no longer checks for a stale committed build.
- Each release adds one `release: vX.Y.Z` commit to generated release notes.
- `edge` history is rewritten on every update; only the latest build is kept.
- Earlier tags (through v0.1.5) point at `main` commits and remain valid.
