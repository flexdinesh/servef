# 0012: Repository names and worktree header

- Status: Accepted
- Date: 2026-10-15
- Supersedes parts of: [0009](0009-multiple-roots-and-worktree-picker.md)

## Context

The explorer header named the project by its folder, so a linked worktree
showed `wt-feature` and a main checkout kept as `repo/main` showed `main`. The
branch beside it always opened the worktree list, even for a repository with no
worktrees, where the picker fell back to the project list. Branches and linked
worktrees shared one icon.

## Decision

- A repository's name is the last path segment of its `origin` remote URL
  (without `.git`), read from the common git directory's `config`; else its
  main worktree's folder, else its bare git directory's name. `internal/projects`
  now reads that one config key besides `.git`, worktree, and `HEAD` files.
- `/api/projects` projects carry `repository` (empty for folders); the picker
  names repository rows, worktree lists, and `repo ›` labels with it.
- `pageData` adds `repository`, `projectKind`, and `worktrees` (the count of the
  repository's verified checkouts).
- The explorer header names the project by `repository`, falling back to the
  folder name.
- The header branch opens the worktree list only when `worktrees > 1`; otherwise
  it is static text.
- Linked worktrees show a colored fork icon in the header and picker; other
  checkouts keep the branch icon.

## Consequences

- Worktrees of one repository share a header name; the branch tells them apart.
- A repository with no `origin` remote keeps its folder name.
