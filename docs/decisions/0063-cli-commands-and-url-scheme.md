# 0063: CLI commands and the `srcz://` URL scheme

- Status: Accepted (via [0065](0065-walkthrough-publish-cli.md), [0068](0068-cli-opens-desktop-app-and-desktop-releases.md), [0069](0069-srcz-remote-and-ssh-links.md), [0071](0071-cli-server-connections-and-pasted-urls.md)); `srcz remote` and ssh links removed and scheme grammar revised by [0076](0076-remote-servers-over-http.md)
- Corrected by [0068](0068-cli-opens-desktop-app-and-desktop-releases.md): the open URL is `srcz://local/?project=<abs>`; inside SSH, `srcz [path]` serves in browser mode with a notice until `srcz remote` exists (that interim behavior revised by [0069](0069-srcz-remote-and-ssh-links.md)).
- Implemented in part by [0065](0065-walkthrough-publish-cli.md): srcz walkthrough publish (no daemon signal; running servers poll).
- Date: 2026-11-01

## Context

With a daemon (0061) and a desktop app (0062), `srcz [path]` should open
the path in the app instead of starting another server. Agents publishing
walkthroughs (0047) probe ports 7971 to 7980 for a running server
(`skills/srcz-walkthrough/SKILL.md`) and fail when none runs. Links shared
in a terminal or pasted from a browser need one form that reaches the right
server.

## Decision

### Commands

- `srcz [path]`: validates the path in the terminal, ensures the daemon,
  adds the root, and opens `srcz://local/view?project=<abs>` (macOS `open`;
  Linux `xdg-open` with an `x-scheme-handler/srcz` desktop entry). With no
  desktop app installed it falls back to browser mode with a notice and
  install instructions. Inside an SSH session (`SSH_CONNECTION` set) it
  behaves as `srcz remote`.
- `srcz browser [path]`: standalone (0060); never attaches to the daemon.
- `srcz remote [path]`: ensures an ephemeral remote daemon, adds the root,
  and prints a `srcz://ssh/...` link as an OSC 8 hyperlink and as a plain
  URL (0064).
- `srcz walkthrough publish <file>`: validates in-process with
  `internal/walkthrough`, writes the file-based store, and signals a running
  app or daemon. It replaces agents probing ports 7971 to 7980.
- `srcz daemon run|ensure|stop|status`: internal plumbing for clients;
  `ensure --json` reports the socket path, protocol version, and roots.
  Implemented (0061 Details): `ensure` and `status` print a line by default
  and JSON with `--json`.

### Scheme

- `srcz://<local|ssh/host>/<web route and query>`. The path and query
  mirror web routes exactly (`/view?project=...&wt=...`), so every web link
  has a scheme form and back.
- Pasted `http://localhost:797x/...` URLs resolve through the control
  registry to the CLI server on that port.

### Security

- Any page can fire a scheme link. Local view links are harmless: they open a
  path read-only in an already trusted app.
- The app never contacts a new ssh host from a link without confirmation.

### Revisions when accepted

- Revises 0047: walkthroughs publish without a running server.
- Revises the skill's discovery steps: agents run `srcz walkthrough
  publish` instead of probing ports.

## Consequences

- `srcz [path]` changes meaning once the app ships; scripts that need a
  server use `srcz browser`.
- Linux needs a desktop entry registered by the app package for the scheme.
- Walkthrough targets checked in-process need the same index and git access
  the server uses, without HTTP.
