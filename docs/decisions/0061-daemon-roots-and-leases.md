# 0061: Daemon, runtime roots, and project leases

- Status: Accepted (leases via [0066](0066-lease-levels-and-desktop-events.md); optional TCP listener via [0098](0098-remote-connections.md))
- Date: 2026-11-01

## Context

Each `srcz` run is a separate HTTP server on a port from 7971 to 7980,
with roots fixed at startup (0009). A desktop app (0062), terminal commands
that open a path in it (0063), and agents publishing walkthroughs (0047)
want one long-lived local process that accepts new roots without a restart
and keeps per-project cost proportional to what the user is looking at.
Live updates (0050) follow only projects a visible tab subscribes to; a
desktop app also wants cheap background signals (a new commit, a branch
switch) for projects with no open tab.

## Decision

### Process

- `srcz daemon` is composed from the same server code as the CLI: the same
  handlers, index, git, document, watch, and walkthrough packages. Only
  transport and root management differ.
- Transport is a Unix domain socket only, mode 0600, in a private per-user
  directory (0700). macOS and Linux first. No TCP listener: browsers cannot
  reach the daemon, so mutating calls need no CSRF guard, and other local
  users cannot connect.
- Single instance per user: a second start finds the live socket and exits.
- Started on demand by its clients (the CLI, the app); it is not installed as
  an OS service.
- Exits after 30 minutes with no clients; `--idle-exit` configures it.
- Reports a protocol version separate from the release version. Clients
  compare protocol versions only; a release mismatch with the same protocol
  works, a protocol mismatch blocks with instructions.

### Roots

- The daemon owns a persisted `RootSet` (0060) registry, persisted per user. Roots are added at runtime under 0009's rules (canonical,
  deduplicated, no nesting in either direction).
- `--depth` and `--scan-depth` are daemon-wide settings, not per root.

### Project leases

A project holds the highest level any lease grants:

- **Known**: listed in the registry only. No watches, no processes, no cost.
- **Watched**: git state only (HEAD, the index, refs; as
  `projects.ReadGitState`), for notifications. No directory watches.
- **Open**: full directory watch (0050) and language servers allowed (0018).
  Held by a visible tab's event stream or a CLI session.

When the last lease at a level ends, the project keeps that level for a grace
period, then drops to the next held level.

### Revisions

- Revises 0009 (applied with Process and Roots): roots are no longer fixed at
  startup for the daemon; the CLI keeps fixed roots.
- Revises 0050 when project leases are accepted: lease levels replace
  subscription-only following; the 8-project cap applies to open projects,
  not watched ones.

## Details (accepted)

Process and Roots are implemented in `internal/runtime/` (web app, HTTP
server, and shutdown order shared with the CLI), `internal/daemon/`, and
`srcz daemon run|ensure|stop|status` (`cmd/daemon.go`).

### Socket

- Linux: `$XDG_RUNTIME_DIR/srcz/daemon.sock` when that variable is set and
  absolute; otherwise `${TMPDIR:-/tmp}/srcz-<uid>/daemon.sock`. macOS:
  `$TMPDIR/srcz/daemon.sock` (`$TMPDIR` is per-user). A path over the
  `sun_path` limit (Linux 107 bytes, macOS 103) falls back to
  `/tmp/srcz-<uid>/daemon.sock`. `--socket` overrides it on every
  subcommand.
- The directory is created 0700 or verified: a symlink or a directory owned
  by another user is refused; an owned directory open to group or others is
  tightened to 0700. The socket is 0600.

### Single instance and stale sockets

- The daemon holds `flock(LOCK_EX|LOCK_NB)` on `<socket>.lock` for its
  lifetime. A held lock means a daemon is running (`ErrRunning`). With the
  lock taken, a leftover socket file is a crashed daemon's and is replaced;
  a non-socket file at the path is refused. The kernel releases the lock on
  a crash. On stop the socket is unlinked before the lock is released.

### Control API

Requests under `/_daemon/` never reach the web app. Any with `Origin` or
`Sec-Fetch-Site` gets 403 (`browserRequest`), so a proxy that forwards
browser requests cannot reach them. JSON errors are `{error, code?}`.

| Route | Response |
| --- | --- |
| `GET /_daemon/status` | 200 `{protocol, version, pid, socket, roots, depth, scanDepth, idleExit, startedAt}` |
| `GET /_daemon/roots` | 200 `{roots, skipped}` |
| `POST /_daemon/roots` `{"path":"/abs"}` (JSON, 4 KiB) | 201 `{root, added: true}`; 200 `added: false` for an existing root; 400 `invalidRoot` (relative, missing, not a directory, overlapping); 415 non-JSON; 500 `rootsNotSaved` (served until the daemon stops) |
| `POST /_daemon/stop` | 202, then the daemon stops |

There is no root removal.

### Protocol

- `ProtocolVersion` is 1. Clients block only on a protocol mismatch, with
  instructions to run `srcz daemon stop` or use a matching build; a release
  mismatch is reported, never blocking.
- `/_daemon/status` and `/_daemon/stop` keep their shape across protocol
  versions, so any client can report and stop any daemon.

### Roots file

- `os.UserConfigDir()/srcz/roots.json` (`$XDG_CONFIG_HOME` on Linux,
  `~/Library/Application Support` on macOS): `{"version":1,"roots":[...]}`,
  file 0600 in a 0700 directory, written atomically (temporary file, fsync,
  rename).
- At start, a saved root that is missing or overlaps another is skipped,
  logged, listed as `skipped`, and kept on the next save, so an unmounted
  drive is not forgotten. A malformed file or unknown version stops the
  daemon before it listens; it is never overwritten.
- A root removed while serving is a discovery warning in `/api/projects`,
  not a failure for every root. The CLI still refuses a missing or
  unreadable root at startup.

### Idle exit

- Idle means no request in flight, event streams included, for
  `--idle-exit` (default 30 minutes; 0 disables). The timer is armed at
  start.

### Host and Origin

- Web routes keep the local `Host` check over the socket; clients send
  `Host: localhost`. `internal/web` stays transport-agnostic.
- Publishing a walkthrough through the daemon never opens a browser.

### Process

- `srcz daemon ensure` starts `srcz daemon run` in its own session
  (`setsid`), working directory `/`, stdin `/dev/null`, output to
  `<socket>.log` (truncated per start), and waits up to 10 seconds for it to
  answer. Depth flags apply only to a daemon it starts.
- Other operating systems compile; daemon commands return an error.

## Consequences

- Discovery over a growing root registry needs per-root caching; rewalking
  every root on each `/api/projects` call does not scale with runtime roots.
- The frontend's last-project memory is keyed by the set of roots (`rootsKey`
  in `web/src/workspace/workspace-memory.ts`). With mutable roots that key
  changes on every `Add` and loses memory; it must be rekeyed (for example by
  connection).
- Watched leases cost one watch set per checkout's git state; their count
  needs its own bound, measured separately from open projects.
- A stale socket (crashed daemon) must be detected and replaced, not trusted.
