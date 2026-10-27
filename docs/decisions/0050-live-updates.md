# 0050: Live updates for the projects browsers show

- Status: Accepted
- Date: 2026-10-27
- Supersedes: the "No polling" refresh clauses of [0024](0024-source-control-view-and-working-tree-diffs.md) and [0027](0027-commit-history-in-source-control.md)

## Context

servef rescans on every request (0007) but the browser only asks when the
user navigates, refreshes, or returns to the window. With an editor or an
agent changing files beside the browser, the tree, the open document, and
Source Control go stale while the user watches. Focus-based rereads (0024,
0027) miss exactly that case: the browser never gets focus.

One server can serve many repositories, and each browser tab shows one
project. Several tabs can show several projects, or the same one.

## Decision

### Which projects are followed

- The server keeps no current project (0007 stands). A tab's interest is its
  subscription: while visible, a tab holds one server-sent event stream,
  `GET /api/events?project=<id>`, for the project in its URL. Switching
  project resubscribes; hiding the tab closes the stream.
- The hub (`internal/watch`) follows the union of subscribed projects. Tabs
  on one project share it; a project stops 30 s after its last subscriber
  leaves, so reloads and tab switches do not rewalk it.
- At most 8 projects are followed at once. A project idle in its grace period
  gives up its slot; past the cap a stream reports `off` and the tab rereads
  only on return.

### What is watched

- The directories a scan reads (`files.Directories`): `--depth` and
  `--exclude` apply; gitignored directories are watched, since the tree shows
  them. Directories created later are watched once their batch is sent.
- For a checkout, its git state (`projects.ReadGitState`): HEAD and the index
  in its own git directory, packed-refs and every ref in the common one. Lock
  files and other git files are ignored.
- One fsnotify watcher serves every project. Directories are
  reference-counted, so overlapping projects (a root folder and the
  repositories inside it, worktrees of one repository) cost one watch.

### Fallback

- A project polls instead when it has more than 4096 directories, or, where
  the watcher holds a descriptor per entry (kqueue: macOS and the BSDs), more
  than 8192 entries; and when adding a watch fails (for example the inotify
  watch limit). A poll compares size and modified time of every entry each
  interval: `max(2 s, 4 × the last walk)`, at most 30 s.
- Native FSEvents on macOS is deferred: fsnotify has no cgo-free backend and
  releases build with `CGO_ENABLED=0`. A purego binding is the planned
  follow-up.

### Events

- `status`: `{mode: watching | polling | off, reason}`, first on every stream
  and whenever the mode changes.
- `change`: `{paths, tree, git, overflow}`: root-relative paths changed,
  whether an entry was created, removed, or renamed, whether git state
  changed, and whether more changed than the 256 paths listed. Batches send
  after 500 ms of quiet, or 2 s after their first change.
- Streams send a heartbeat comment every 25 s; each write extends its own
  deadline past the server's write timeout. Shutdown ends every stream first.

### What follows

- The browser rereads what a change touches: the tree on `tree`, the active
  document when its path changed, Source Control on any change, and the
  search corpus when Search is open (else when it opens). `overflow` rereads
  all of them. A reread keeps scroll, selection, line anchors, and the
  document's mode. Inactive tabs reread on activation, as before.
- Returning to a hidden tab rereads once, then resubscribes.
- The status bar shows Live, Polling, or Off, with the reason.

## Consequences

- Source Control is repository-scoped, but only the watched directories
  report changes: edits under excluded directories or below `--depth` still
  wait for focus or Refresh.
- Tools that rewrite the index often (`git status`, editor git integrations)
  reread Source Control each time, at most once per batch.
- A busy tree at most doubles servef's reads: one batch per 500 ms to 2 s.
- Browsers allow six HTTP/1.1 connections per origin; hidden tabs close
  their streams, so streams number the visible windows. Sharing one stream
  across tabs (a SharedWorker) is deferred until that limit is hit.
- servef adds one dependency, `github.com/fsnotify/fsnotify`; the binary
  stays self-contained.
