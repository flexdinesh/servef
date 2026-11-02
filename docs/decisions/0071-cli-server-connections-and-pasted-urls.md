# 0071: CLI-server connections and pasted URLs

- Status: Accepted; scheme form of server URLs and non-loopback servers added by [0076](0076-remote-servers-over-http.md)
- Date: 2026-11-02
- Implements: [0062](0062-desktop-app.md) CLI-server connections;
  [0063](0063-cli-commands-and-url-scheme.md) pasted localhost URLs
- Builds on: [0070](0070-desktop-ssh-connections.md) (connection kinds,
  per-origin windows, the Connect page)

## Context

Users still run CLI servers in a terminal (`servef browser`, or `servef`
with `--host` or `--port`). 0062 planned to show them as app connections and
0063 to resolve pasted `http://localhost:<port>/…` URLs to them. Each CLI
server writes a private control record (`internal/control`) whose token
guards `/_servef/control` (status and stop). The app must find running
servers without holding those tokens, and pasted text is untrusted.

## Decision

### Discovery

- Hidden plumbing command `servef control list [--json]`. It lists the
  registry and keeps records whose authenticated status check passes
  (concurrent, 2 s total), in registry order. JSON:
  `{"servers":[{"host":"localhost","port":7971,"origin":"http://localhost:7971"}]}`;
  plain output prints one origin per line.
- The registry and its tokens stay inside `internal/control`; the app runs
  the command (argv only, 5 s timeout) and never reads records or sends
  tokens. `Remote` gains `Host()` and `Port()`.

### Connections

- Kind `cli`, id `cli-<port>` (canonical decimal, 1 to 65535), origin
  `servef-app://cli-<port>`, upstream TCP to the listed origin's host. The
  app keeps only loopback origins (`localhost`, `127.0.0.0/8`, `::1`), one
  per port; a server bound to another address is not offered.
- Registered on demand: a lookup of an unregistered `cli-<port>` lists once
  (single flight) and registers exactly the running servers. The Connect page
  and a pasted URL list again. A failed list keeps the registrations.
- `ForwardRequest.socket` became `target: {socket} | {host, port}`, with one
  keep-alive agent (16 sockets) per target. TCP connects use
  `autoSelectFamily`, so `localhost` reaches either family.
- Upstream requests keep the proxy's header allowlist with
  `Host: localhost:<port>` (the CLI server's Host check accepts `localhost`
  and IP literals).
- No subscriptions, holds, or notices: a CLI server has no control stream,
  and the user owns its lifetime in a terminal. Titles read
  `"<page> - localhost:<port>"`.

### Proxy

- `/_servef` (the CLI control endpoint) is refused with 403 for every
  connection, as `/_daemon` is, before any lookup.
- Unknown port: documents get "No servef server is running on port N"
  (404, Retry); the API gets `unknownConnection` (404).
- Server gone (refused, reset, or broken pipe before a response head): the
  registration is dropped, never re-ensured. A read lists again and retries
  once when the server is still listed (a pooled socket outlived a restart);
  otherwise documents get "The servef server on port N stopped" (503, a
  `servef browser --port N` hint, Retry) and the API `serverStopped` (503).

### Pasted links

- Connect page form "Open a link": GET `servef-app://app/open?url=…`. On
  success main opens the link and answers 303 to `/connect`; otherwise the
  Connect page renders with the reason and the escaped text refilled.
- Accepted, after trimming and at most 8 KiB: `servef://` links (opened as
  the OS handler would, including ssh first-contact confirmation), and
  `http://localhost|127.0.0.1|[::1][:<port>]` on `/` or `/view` with any
  query and fragment and no credentials (port 80 when omitted), mapped to
  `servef-app://cli-<port><path><query><fragment>` only when `control list`
  reports a server on that port. Anything else: "Not a servef link." or "No
  servef server is running on port N."
- In-app clicks on `http://localhost:<port>` links still open the system
  browser; only the form resolves them.
- The Connect page gains "Servers on this computer": links to
  `servef-app://cli-<port>/`, routed through the `connection` navigation
  kind. Navigation to a `cli-*` origin opens only when a lookup finds the
  server running.
- `shareableOrigin` maps `servef-app://cli-<port>` to
  `http://localhost:<port>`, the address the server prints.

## Security

- The app never handles registry tokens and cannot stop a CLI server.
  Renderers cannot reach `/_servef` through the proxy.
- Pasted text never reaches a shell or the network until it parses as a
  loopback URL whose port the authenticated list reports as servef.
- `/open` is a GET because it only opens windows, as a clicked `servef://`
  link does; any page can already fire such links (0063). Cross-origin
  fetches to `servef-app://app` are refused by the proxy's Origin guard.
- TOCTOU: another local process could bind a freed port between the list and
  a request. Its content stays in its own isolated origin with no bridge, and
  a refused connection drops the registration.

## Consequences

- A CLI server window shows the server's own UI (browser mode) with
  app-isolated storage per port; there are no desktop notifications for it.
- Restarting a CLI server on the same port reuses the window and its origin.
- Desktop tests: `cli-servers`, `location`, proxy and connections `cli`
  cases, forward over TCP, the Connect form; smoke `cli.spec.ts` starts
  `servef browser --port 0`, pastes its URL, and stops it.
