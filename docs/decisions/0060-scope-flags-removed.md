# 0060: Scope flags removed; `srcz browser` alias

- Status: Accepted; the bare `srcz [path]` clause is superseded by [0068](0068-cli-opens-desktop-app-and-desktop-releases.md)
- Date: 2026-11-01
- Supersedes: [0001](0001-filename-glob-scope.md); the exclusion parts
  (`--exclude`) of [0007](0007-projects-and-project-picker.md) and
  [0009](0009-multiple-roots-and-worktree-picker.md)

## Context

`--filename` (0001) and `--exclude` (0007, 0009) narrow what a server shows
per process. A long-lived daemon serving many roots (0061) and a desktop app
(0062) cannot carry per-invocation scope: one server answers every caller.
In practice the defaults (`*`; `.git`, `node_modules`, `vendor`) cover nearly
every run, gitignore marking (0023) already keeps generated and vendored files
out of search and navigation, and the Explorer filename filter (0006) narrows
the tree in the UI. The CLI also needs a name for today's standalone behavior
before bare `srcz` changes meaning (0063).

## Decision

### Scope

- Remove `--filename` and `--exclude`. Every file under a project is in scope.
- `.git`, `node_modules`, and `vendor` are always skipped. The set is fixed
  and owned by `internal/files`; `cmd` passes none.
- Gitignore marking (0023) is unchanged: ignored files stay browsable in the
  tree and are excluded from content search and returned navigation
  locations.
- Markdown links rewrite to any relative file (the former default `*`).
- Page data drops `filenamePatterns`.
- The Explorer filename filter (0006) stays; it only changes visibility.

### `srcz browser`

- Add `srcz browser [path]`, an alias of today's behavior with the same
  flags: `--path` (repeatable), `--depth`, `--scan-depth`, `--host`,
  `--port`, `--no-open`.
- Bare `srcz [path]` keeps browser behavior until the desktop app ships
  (0063).
- Subcommand names shadow same-named directories: `srcz browser` runs the
  subcommand. Serve a directory named `browser` as `srcz ./browser`.

### Roots

- `projects.RootSet` holds the ordered canonical roots behind one
  concurrency-safe type: `List` returns them in order; `Add` enforces 0009's
  rules (symlink-evaluated canonical path, deduplicated, no root inside
  another in either direction) and rejects violations.
- The CLI builds a `RootSet` at startup and never changes it. No HTTP route
  adds roots; mutation exists for the daemon (0061).

## Consequences

- Breaking CLI change: invocations passing `--filename` or `--exclude` fail
  as unknown flags. Users narrow the tree with the Explorer filter and keep
  generated files out of search with `.gitignore`.
- Live updates (0050) watch every directory a scan reads below `--depth`,
  minus the fixed set; there are no user exclusions to apply.
- Discovery (0007) skips the fixed set and hidden directories only.
- Source Control was already repository-scoped and is unaffected.
