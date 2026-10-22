# 0031: Hide .git entries

- Status: Accepted
- Date: 2026-10-22

## Context

0023 kept a linked worktree's `.git` file browsable and only skipped it in
search. The default `.git` exclusion matches directories only, so the explorer
listed a `.git` file that editors and file managers hide. It is git metadata,
never project content.

## Decision

- The index never includes a `.git` entry of any type at any level. It is not
  in the tree, direct document access, search, or navigation locations.
- Supersedes the "A `.git` file is always ignored" clause of 0023; the rest of
  0023 stands.

## Consequences

- Linked worktrees and repositories browse alike.
- `--exclude` stays directory-only; `.git` is the one name hidden for files.
