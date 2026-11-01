# 0061: Daemon, runtime roots, and project leases

- Status: Proposed
- Date: 2026-11-01

## Context

Each `servef` run is a separate HTTP server on a port from 7971 to 7980,
with roots fixed at startup (0009). A desktop app (0062), terminal commands
that open a path in it (0063), and agents publishing walkthroughs (0047)
want one long-lived local process that accepts new roots without a restart
and keeps per-project cost proportional to what the user is looking at.
Live updates (0050) follow only projects a visible tab subscribes to; a
desktop app also wants cheap background signals (a new commit, a branch
switch) for projects with no open tab.

## Decision

### Process

- `servef daemon` is composed from the same server code as the CLI: the same
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

### Revisions when accepted

- Revises 0009: roots are no longer fixed at startup for the daemon; the CLI
  keeps fixed roots.
- Revises 0050: lease levels replace subscription-only following; the
  8-project cap applies to open projects, not watched ones.

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
