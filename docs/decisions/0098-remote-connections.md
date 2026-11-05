# 0098: Remote connections: `servef --remote` and `servef connect`

- Status: Accepted
- Date: 2026-11-05
- Supersedes: [0076](0076-remote-servers-over-http.md) (no-UI serving, printed
  `web:` and `desktop:` links, the `servef://<host>:<port>` grammar),
  [0095](0095-desktop-app-is-mygo-electron-removed.md) (the app shows this
  computer only; server connections are not ported)
- Revises: [0061](0061-daemon-roots-and-leases.md) (the daemon can also listen
  on TCP, and may exit before its idle timer)

## Context

The desktop app is the main UI, a browser secondary. Serving on another machine
meant an unauthenticated http port that the app handed to a browser. Bare
`servef` opened that port on any machine with no display, so an ssh session
could expose files by accident.

The same shape has to admit later work without a rewrite: pasting a string in
the app, switching connections in its UI, ssh aliases that start servef, and an
ssh tunnel to a Unix socket.

## Decision

**Commands.** `servef <path> --remote` adds the path to the daemon, opens a TCP
listener in it, prints a string, and returns to the shell. `servef connect
<string>` on another machine saves it and opens the app on it. `servef remote
status|stop|rotate` manage the listener. `--host` and `--port` choose the
address; the default is every interface on the first free of 7971-7980.
`--web` also prints a browser link, and `--web` without `--remote` fails. It
serves all the daemon's roots, which it prints.

**No-UI machines.** Bare `servef` with no display prints advice and exits
non-zero. It never opens a port. `servef browser --host` keeps its
unauthenticated bind and warning.

**Security.** TLS with a self-signed P-256 certificate, pinned by SHA-256 of its
SPKI truncated to 16 bytes, plus a 128-bit token. The string is
`<hosts>:<port>/<credential>`, with hosts editable by hand. The identity
persists in a 0600 file until `servef remote rotate`, so a server restart keeps
strings valid. Over TCP only the app's routes answer: the `/_daemon/*` control
API has no route there. A browser link redirects to an HttpOnly, Secure,
SameSite=Strict cookie, so the token leaves the address bar.

**Lifetime.** The listener and its daemon stop when no UI needs them: 10 minutes
with no first UI, 5 minutes after the last closes. A UI is an open presence
stream, `/api/presence`. `/api/events` cannot be it: a hidden page closes that
stream, and a hidden window still counts. Only the TCP binding counts presence,
reported as `presence` in `/api/projects`.

**Connection identity.** The id is `r-<12 hex>`, derived from the fingerprint:
no secret, stable across address changes, valid in a URL host. The CLI writes
`UserConfigDir/servef/connections.json` (0600), so `servef://<id>/…` carries no
secret. A page load rereads the file, so a new string reaches a running app.

**Desktop.** One window per connection, each its own custom-scheme host:
`servef-app://local` for this machine's daemon, `servef-app://<id>` for a
server. WebKit isolates storage per host; an e2e test confirms it. The proxy
takes an `Upstream` per host through a `Resolver`. mygo trusts every page of a
handled scheme, so the bridge checks the calling origin per method: a remote
page gets `closeWindow`, `showContextMenu`, and `showAppMenu`, never
`openFolder` or `revealPath`. The credential never reaches a page.

**Boundaries.** `pkg/connection` owns the string, transports, and store, and the
desktop app may import it. `internal/remote` owns identity, auth, compression,
listening, and addresses. `internal/daemon` owns when the listener runs.
`internal/runtime` serves per-binding handler chains, so gzip and auth apply on
TCP only.

**Links.** `servef://` hosts are `local` or a connection id. Addresses and
secrets are invalid in a link.

Later work attaches here: pasting a string is a bridge method over the same
store; a switcher lists the store; ssh is another `pkg/connection` transport and
another `Upstream`.

## Consequences

- Walkthroughs published on a `--remote` host print `servef://<id>/view?…`,
  which opens on the connected computer. A browser server prints `web:` only.
- Three runs of the 0080 measure (daemon, one root, five `/api/projects`, 2 s
  settle) gave VmRSS 19752-19804 kB before this work, 17788-19856 kB after with
  no listener, and 21236-26240 kB with the listener serving over TLS and HTTP/2.
  Local use pays nothing; `--remote` pays up to about 6 MiB.
- Reaching a server through an ssh tunnel needs `servef connect --host localhost`
  and a port forward you set up yourself; the pin still applies.

Known limits:

- The check that lets the daemon exit after the listener closes runs once. A
  request in flight at that instant leaves the exit to the 30-minute idle timer.
- With links queued before the first window, the latest wins: a second
  connection's earlier link is dropped.
- A remote page that navigates to another known connection's origin lands in
  that connection's window.
- No revocation short of `servef remote rotate`, which invalidates every string.
