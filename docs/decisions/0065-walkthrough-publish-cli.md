# 0065: `servef walkthrough publish` without a server

- Status: Accepted; revised by [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) (`servef://` URL when the app shows the walkthrough) and [0069](0069-servef-remote-and-ssh-links.md) (`servef://ssh` URL inside SSH)
- Date: 2026-11-01
- Extends: [0047](0047-walkthroughs.md), [0049](0049-copyable-agent-instructions.md)
- Implements in part: [0063](0063-cli-commands-and-url-scheme.md) (the
  publish command, ahead of the daemon)

## Context

Agents publish walkthroughs with `POST /api/walkthroughs` (0047) and find a
server by probing ports 7971 to 7980. With no server running, or one on
another port, publishing fails. The store is already a per-user directory of
files (`walkthrough.DefaultDirectory()`), and the browser already polls the
list, so a server is needed only to check targets, not to store or show them.

## Decision

### Commands

- `servef walkthrough publish <file|->` reads one draft from a file, or
  standard input for `-`, within the 1 MiB request limit. It decodes strictly,
  normalizes, checks targets, and writes the same store. Flags: `--depth`
  (default 5, as `servef browser`), `--no-open`, `--json`.
- `servef walkthrough schema` prints the JSON Schema, so agents need no server
  to read the format.
- Text output by default: `published <id> (created|replaced) for <project>`,
  then `url: <url>` or how to view it. `--json` prints
  `{"id", "project", "created", "url"}` on one line, `url` only when found.
- Any rejection exits 1 and lists every problem as `field: message` on
  stderr. Nothing is written.
- Subcommand names take precedence over directories (as 0060's `browser`):
  serve a directory named `walkthrough` as `servef ./walkthrough`.

### Targets

- The CLI treats the draft's `project` as its own root: absolute, clean, an
  existing directory, symlinks evaluated (`projects.ResolveStandalone`).
- A folder inside a repository that is not itself a repository or worktree is
  rejected with a hint to use the repository root
  (`git rev-parse --show-toplevel`). Otherwise it would be stored under a key
  no server lists, since servers open repositories whole.
- Targets are checked by the exact checker `POST /api/walkthroughs` uses. It
  stays in `internal/web` (the 0047 boundary: web implements
  `walkthrough.Targets`), decoupled from the server and exported as
  `web.NewWalkthroughTargets(roots, depth, types)`. `internal/walkthrough`
  stays free of git and HTTP.

### Running servers

- No signal. Browsers poll the list every 5 s while visible and on focus; the
  store rereads changed files. A CLI publish appears within seconds.
- After writing, the CLI asks each running server in the control registry
  (`control.Registry.List`) for the walkthrough. The first that shows it gives
  the URL (`<origin>/view?project=…&wt=…`). A server that serves the project
  but answers `walkthroughUnavailable` reads another store (for example
  another `XDG_CACHE_HOME`); the CLI warns and names its POST endpoint.
- `"open": true` opens the found URL unless `--no-open` or an SSH session.
  With no server showing it, the CLI prints how to view it
  (`servef <project>`, then Walkthroughs) and exits 0. It never starts a
  server.

### Agents

- The skill and the copyable instructions (0049) publish with the CLI first.
  Curl to a known server stays as the fallback for older binaries and servers
  with a separate store. `POST /api/walkthroughs` is unchanged.

## Consequences

- Publishing works with no server running and without port probing.
- A server run with `--depth` above 5 accepts deeper working-tree files than
  the CLI default; `--depth` matches it.
- A server rooted inside a repository (`servef ~/repo/docs`) accepts
  `~/repo/docs` as a project; the CLI rejects it. A repository at `~` makes
  every non-git folder under `~` inside a repository for the CLI.
- Processes with different cache directories write different stores. The
  viewer probe warns when a running server serves the project; otherwise the
  walkthrough is invisible to that server.
- The daemon or app (0061, 0062) can later watch the store directory instead
  of polling.
