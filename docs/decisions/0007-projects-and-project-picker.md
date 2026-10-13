# 0007: Projects and project picker

- Status: Accepted
- Date: 2026-10-13

## Context

`servef` scopes one directory. Users run it over a folder of repos and
worktrees and want to switch between them without restarting, always seeing
the file system as it is at switch time. The server already rescans on every
request and holds no file state.

## Decision

### Model

- **Root**: the CLI path. Confinement boundary; nothing outside it is served.
- **Project**: a directory served as the workspace. Every file API is scoped to
  one project; `--depth`, `--filename`, and `--exclude` apply from its root.
- **Project ID**: root-relative slash path (`.` is root). Stable across
  restarts.
- The server keeps no current project and no project cache. The project lives
  in the URL; each request resolves it against the file system.

### Discovery

- `--scan-depth` (default 10) bounds the repo walk; root is depth 0. `0`
  checks root only.
- A repo is a directory with a `.git` entry (directory or file). Discovery
  stops descending at a repo; symlinks are not followed; `--exclude` names,
  `.git`, and hidden directories are skipped.
- Worktrees are found wherever they are: each discovered repo's common git dir
  lists its linked worktrees (`worktrees/*/gitdir`), verified by the worktree's
  `.git` file linking back. Members of one repo group under the main worktree.
- Worktrees outside root are listed as unavailable with their absolute path;
  they cannot be opened. Widening the boundary is a future opt-in, not a
  default.
- A root that is not a repo is always listed first as a folder project, so
  loose files beside repos stay reachable; with no repos it is the only one.
- Startup prints the project count instead of a file count.
- `/api/projects` rediscovers on every call.

### Resolution

- Any non-excluded directory under root, symlink-evaluated and confined.
  Discovery decides listing, not access. Absolute IDs are rejected.

### Contracts

- `GET /api/projects` → `{ servedRoot, rootName, projects: [{ id, name, path, kind, group, available }], warnings }`;
  unavailable entries have an empty `id` and an absolute `path`.
- `GET /api/directories?path=` → child directories of a root-relative path with
  `isProject`; confined to root; excluded names hidden, hidden directories
  shown.
- `/api/page`, `/api/format`, `/api/raw`, `/api/image`, `/api/search-documents`
  require `project`. Missing → 400; unresolvable → 404 "Project unavailable".
- `pageData` gains `servedRoot`, `project`, and `projectUnavailable`; it does
  not carry the project list. Rewritten Markdown links carry the project.

### UI

- One quick pick on the content-palette frame, opened from the explorer header
  name or Ctrl/Cmd+O. File search shortcuts are unchanged.
- **Projects mode**: fuzzy filter over name and path; worktrees nested under
  their repo; out-of-root worktrees shown disabled with an "Outside <root>"
  note; remembered projects that discovery does not list appear as opened
  folders with a Forget action; current project marked. `/` or
  **Open folder…** enters Browse.
- **Browse mode**: path input with the root as a fixed prefix; Tab completes,
  Enter descends, first row opens the typed folder, Backspace on an empty
  segment goes up.
- Switching closes the picker and keeps the current workspace until the new
  project's page loads, then remounts the workspace: the tree filter, search
  index, and format cache reset, and the project's tabs are restored after
  checking them against the fresh tree.
- Browser storage (`servef-workspaces`), keyed by served root and project ID,
  keeps each project's tabs and active document and the last project. Opened
  folders are the remembered projects discovery does not list; Forget drops
  one. Landing on `/` without `project` reopens the last project (an unlisted
  folder only while it still exists), else the root, opening the picker when
  there is a choice. `/view` without `project` stays root-relative.
- No native OS folder dialog: browsers cannot return server paths.

## Consequences

- Freshness needs no cache invalidation; switching is new requests.
- Out-of-root worktrees are visible but not browsable; run `servef` from a
  common parent to browse them.
- Markdown links and images cannot cross projects.
