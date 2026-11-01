# 0069: `servef remote` and ssh links

- Status: Accepted
- Date: 2026-11-01
- Implements: [0063](0063-cli-commands-and-url-scheme.md) `servef remote` and
  the ssh link form; [0064](0064-ssh-remotes.md) Host identity (link hints)
- Revises: [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) step 2
  (SSH sessions), [0065](0065-walkthrough-publish-cli.md) (publish URL inside
  SSH)

## Context

0064 has the desktop app reach servef on an SSH host through the system
`ssh` and a forwarded daemon socket. Something on the host must add roots to
that daemon and hand the user a link their computer can resolve to an ssh
destination. 0068 made bare `servef [path]` inside SSH serve in a browser
until `servef remote` existed, and `servef walkthrough publish` inside SSH
printed a remote `http://localhost` URL that is useless on the user's
computer.

## Decision

### `servef remote [path]`

- Flags: `--path` (repeatable; a positional path is shorthand for one),
  `--depth`, `--scan-depth`, `--alias <name>`.
- Validates paths and discovery as bare `servef` does; errors exit before
  any effect. An invalid `--alias` fails first.
- Ensures the daemon in process, as `servef daemon ensure` (depth flags apply
  to a daemon it starts; a running one with other values gets a warning),
  posts `/_daemon/open` per root, prints the link to the first project, and
  exits 0. No daemon or protocol change: remote daemons stay ephemeral
  (default 30-minute idle exit; roots persist in that user's `roots.json`).
- macOS and Linux only; elsewhere it returns `daemon.ErrUnsupported`.
- Failures exit non-zero; the explicit command never falls back to browser
  mode.
- Outside an SSH session it still prints the link, with a warning that the
  hints may not identify the host.
- Output (stdout), one `serving` line per root:

  ```text
  serving /home/dee/code/foo on devbox
  servef://ssh/devbox/?project=%2Fhome%2Fdee%2Fcode%2Ffoo&ssh-hostname=devbox&ssh-user=dee&ssh-addr=10.0.0.5
  open the link on your computer; it needs the servef desktop app
  ```

  The link line is an OSC 8 hyperlink (`ESC ]8;;URL ESC \` URL
  `ESC ]8;; ESC \`) only when stdout is a terminal and `TERM` is not `dumb`.
  The visible text is the plain URL either way, so copying works in
  terminals without OSC 8.
- No `SERVEF_REMOTE_ALIAS` environment variable: `--alias` only; the app
  remembers how it resolved a host (0064).

### Bare `servef [path]` inside SSH

0068 step 2 becomes: an SSH session (`SSH_CONNECTION`, `SSH_CLIENT`, or
`SSH_TTY`) runs the `servef remote` flow with the same paths and depths
(default host name; `--alias` is not a root flag). Invalid paths exit as in
step 4. A later failure (unsupported OS, no host name, daemon start, protocol
mismatch, open) prints a notice and serves in browser mode, as other app-mode
failures do. Step 1 (`--host` or `--port`: browser mode) still wins.

### Link grammar

Extends 0063's `servef://<local|ssh/host>/<route>?<query>`:

```text
servef://ssh/<host>/<route>?<web query>&ssh-hostname=<h>&ssh-user=<u>[&ssh-addr=<ip>][&ssh-port=<n>]
```

- `<host>`: `--alias` when given, else `os.Hostname()`, percent-encoded as
  one path segment. No host name and no alias: the command fails.
- `<route>`: `/` (project landing) or `/view`, then the web query exactly as
  local links (`project`, `wt`, …), in `url.Values.Encode` order. Hint keys
  follow in the fixed order above; each is omitted when empty. Web routes
  never use `ssh-` keys; the app strips them.
- `ssh-hostname`: `os.Hostname()`, also present when `--alias` names the link.
- `ssh-user`: `user.Current().Username`, else `$USER`.
- `ssh-addr` and `ssh-port`: the server address and port of `SSH_CONNECTION`
  (`client_ip client_port server_ip server_port`), only when both parse (IP
  by `net.ParseIP`, written canonically; port 1 to 65535). `ssh-port` is
  omitted when 22.
- `--alias` matches `^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`: an ssh Host token
  with no leading dash, `@`, `%`, `/`, or whitespace.
- `testdata/remote-links.json` is the shared fixture:
  `[{name, hints: {alias, hostname, user, addr, port}, route, query, url,
  parsed: {host, hostname?, user?, addr?, port?}}]`. Go generates each `url`
  from `hints`, `route`, and `query` and parses it back to `parsed`; the
  desktop app parses each `url` to `parsed`, `route`, and `query`. Grammar
  drift fails both sides.

### Walkthrough publish inside SSH

When the daemon announces the walkthrough, `url` is the ssh link to
`/view?project=…&wt=…` (also in `--json`), and `"open": true` opens nothing;
the human output says so. Without an announcing daemon (or a host name),
publish probes registered servers as before (0065).

## Consequences

- `servef remote` links need the desktop app's ssh connections (0064); an
  app without them shows "Remote links arrive in a later servef". Both land
  in the same release.
- The app reaches the daemon `servef remote` started only when its own
  `ssh <host> servef daemon ensure` resolves the same socket, which needs the
  same `XDG_RUNTIME_DIR` in interactive and non-interactive SSH sessions.
- Hints are advisory: a link names a host but never carries credentials or
  commands, and the app confirms any host it does not know (0063).
