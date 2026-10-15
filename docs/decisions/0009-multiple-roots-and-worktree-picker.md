# 0009: Multiple roots, whole repositories, and worktree picker

- Status: Partially superseded by [0012](0012-repository-names-and-worktree-header.md)
- Date: 2026-10-15
- Supersedes parts of: [0007](0007-projects-and-project-picker.md)

## Context

Users keep repositories in more than one parent folder and run `servef` from
inside a repository whose worktrees sit beside it. 0007 served one root,
listed out-of-root worktrees as unopenable, let Open folder open any folder
(including folders inside repositories), and listed every worktree flat under
its repository.

## Decision

### Roots

- `--path` is repeatable; a positional path is shorthand for one and merges
  first. No path serves `.`.
- Roots are symlink-evaluated and deduplicated in order. A root inside another
  fails before serving.
- Discovery walks each root with one repository-grouping map, so a repository
  groups with its worktrees across roots.

### Project IDs

- A project ID is the canonical absolute path. Relative IDs and `.` are gone;
  IDs are stable across restarts and argument order.
- `/` without a project reopens the last project for this set of roots, else
  the first root. `/view` without a project is relative to the first root.

### What opens

- Below a root, a directory opens when it has `.git` (repository, worktree,
  submodule), or when no directory from its root down to it has `.git`. Folders
  inside a repository do not open (404, explained), and a repository root opens
  only whole.
- Outside every root, only a linked worktree opens, and only when its admin
  directory links back to it and its common git directory lies in a root. This
  replaces 0007's "outside root is unavailable"; widening stays tied to a
  served repository.
- `/api/directories` lists the roots for an empty path and refuses repositories
  and folders inside them; repository children are flagged to open whole.

### Contracts

- `GET /api/projects` → `{ roots: [{ path, name, isRepo }], projects: [{ id,
  name, path, kind, group, root, branch, available }], warnings }`. `group` is
  the main worktree's path; `root` the root it or its repository lives in;
  `branch` comes from `HEAD` (short commit when detached), read without git.
- `pageData` replaces `servedRoot` with `roots` and adds `branch`.

### Picker

- One section per root (titled by root name when there are several): the root
  folder, then repositories. A repository with worktrees is one row showing the
  branch it opens and its worktree count; Enter opens its last-used worktree,
  else the main worktree.
- → or Tab, or the row's arrow, lists the repository's worktrees by branch;
  Backspace or ← on an empty query returns. A query flattens every project and
  worktree, matching branch, folder, repository, and root-relative path.
- The explorer header shows the branch; clicking it opens the worktree list of
  the current repository.
- Browse starts at a lone root's children, else the root list; it walks
  absolute paths, does not enter repositories, and opens them whole.

### Memory

- Storage key `servef-workspaces-v2`: tabs per absolute project path, shared
  across sessions; last project per set of roots. Earlier memory is dropped.
- Opened folders are remembered projects in a root that discovery does not
  list.

## Consequences

- Monorepo subfolders no longer open as projects; open the repository.
- URLs carry absolute paths; they stay local to the machine.
- Worktrees of served repositories open anywhere on disk, so running inside a
  repository reaches its sibling worktrees.
