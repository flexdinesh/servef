# 0066: Lease levels, head events, and daemon control events

- Status: Accepted; announcements fan out from `internal/app` by [0087](0087-walkthroughs-in-the-application-layer.md); `head` gains `parent` and project streams carry walkthroughs by [0099](0099-desktop-notifications.md)
- Date: 2026-11-01
- Implements: [0061](0061-daemon-roots-and-leases.md) Project leases
- Revises: [0050](0050-live-updates.md) (lease parameter, open-only project
  cap, `head` event), [0061](0061-daemon-roots-and-leases.md) (covering
  open, session memory key)

## Context

A desktop app (0062) keeps one daemon connection for every window. It needs
cheap git signals (a branch switch, a new commit) for projects with no
visible tab, announcements of walkthroughs published by agents, a way to open
a path that may lie inside or around served roots (`servef ~/code/foo` with
`~/code` served fails today), and browser memory that survives roots
changing at runtime (0061 Consequences).

## Decision

### Lease levels

- `GET /api/events?project=<id>&lease=open|watched`; absent means `open`.
  Other values get 400 `{error, code: "invalidLease"}`, checked before the
  project. Browsers send nothing and keep 0050's behavior.
- `open` follows the directories a scan reads and git state (0050).
  `watched` follows git state only (`projects.ReadGitState`) and never lists
  the tree. A project runs at the highest level its streams hold.
- Watched streams receive `change` only for git changes, as
  `{paths: [], tree: false, git: true, overflow: false}`.
- Caps: 8 projects at open level (including those in their open grace) and,
  separately, 16 projects held only by watched streams. Past a cap the new
  stream gets `status {mode: "off"}`. Status is per level: watched streams of
  a project whose open level is off still see `watching` or `polling`.
- 0050's directory limits apply to a watched project's git-state
  directories; past them, or on a watch failure, git state polls.
- When the last open stream ends, the project keeps open level for 30 s,
  then releases its directory watches and stays watched while watched
  streams remain. When the last stream ends, it stops after 30 s. A new open
  project may evict an idle open project or, failing that, drop an open
  project held only by watched streams to watched level at once. Dropping to
  watched past the watched cap first stops an idle watched project; if none,
  that project's watched streams get `off`.
- A lost watcher event marks the batch as a git change for checkouts, since
  it may have been one.

### Head event

Every stream for a checkout, at either level, sends `head`:

```json
{"branch": "main", "commit": "<40 hex>", "subject": "First line", "previous": {"branch": "main", "commit": "<40 hex>"}}
```

- Sent once after the first `status` with `previous: null` (baseline), then
  after a `change` with `git: true` when branch or commit differs from the
  stream's last `head`.
- `branch` "" is detached; `commit` "" is unborn. Clients derive switch
  versus new commit. Folders, and checkouts whose HEAD cannot be read, send
  none. Read with go-git (`git.ReadHead`), never the `git` binary.
- "Edits settled" is not a server event: clients derive it from `change`
  batches.

### Daemon control events

| Route | Response |
| --- | --- |
| `GET /_daemon/events` (SSE) | `: servef`, then `status {protocol, version}`, then `walkthrough {project, id, title, created}` per announcement; `: heartbeat` every 25 s |
| `POST /_daemon/walkthroughs` `{project, id, created}` (JSON, 4 KiB) | 202 `{announced: true}`; 404 `projectUnavailable` (no served root resolves it), 404 `walkthroughUnavailable` (not in the daemon's store); 400 `invalidRequest` |
| `POST /_daemon/open` `{path}` (JSON, 4 KiB) | `{root, added, replaced, project}`: 201 when added, 200 otherwise; 400 `invalidRoot`; 404 `projectUnavailable`; 500 `rootsNotSaved` |

- Control routes keep 0061's rules: browser requests (`Origin`,
  `Sec-Fetch-Site`) get 403.
- The control stream is a request in flight, so a running client keeps the
  daemon from idling. It ends with the request, the daemon, or shutdown.
- Announcements fan out per stream with a buffer of 16; the oldest is
  dropped when full. Sources: a successful `POST /api/walkthroughs`
  (in process) and `POST /_daemon/walkthroughs` from `servef walkthrough
  publish`, which reads the walkthrough from the daemon's own store. Publish
  announces with a 1 s timeout; a daemon that is not running or does not
  serve the project is ignored, and `walkthroughUnavailable` is warned about
  as 0065's other-store servers are.
- Walkthrough announcements are daemon-level, not per project: an app shows
  them for projects it has no stream for.

### Covering open

- `POST /_daemon/open` serves an absolute existing directory. Within a
  served root nothing changes. Outside every root it becomes a root; served
  roots inside it are replaced (`replaced`, taking the first replaced root's
  place in order) and saved atomically. Skipped roots inside it are dropped.
- `project` is the nearest repository or worktree from the path up to its
  root, else the path itself: an ID `projects.Resolve` accepts.
- 0061's `POST /_daemon/roots` keeps refusing overlap; inner roots are
  replaced only by an explicit open.

### Session memory key

- `/api/projects` and `/api/page` carry `sessionKey`. The daemon sends
  `"daemon"`; the CLI sends its roots joined by `"\n"`, which equals the
  previous browser key, so existing memory keeps working. The browser keys
  its last project by it.

### Development isolation

- `servef daemon run|ensure --roots-file <path>` (hidden) overrides the roots
  file; `ensure` forwards it to a daemon it starts.

### Protocol

- `ProtocolVersion` stays 1: no release contains protocol 1 yet, and these
  routes extend it.

## Consequences

- Watched leases cost one git-state watch set per checkout (a few
  directories), bounded by 16 separately from open projects.
- Clients holding open streams for hidden windows keep directory watches
  within the open cap.
- Replacing inner roots keeps their repositories reachable; folders inside
  an outer repository stop being projects (repositories open whole).
- `/api/page` grows by one string; no extra request is made for the key.
