# 0084: In-process client transport in its own package

- Status: Accepted
- Date: 2026-11-03
- Revises: [0080](0080-application-layer-contracts-and-transports.md)
  (Client: the in-process transport's package)

## Context

0080 put all three transports in `apps/srcz/pkg/client`. The in-process
transport imports `internal/app` and `internal/daemon`, which link the
server and its embedded web app (`internal/web/dist`). Every importer of
`pkg/client` paid for that, including HTTP-only clients such as the native
app (0082), which had to build against a placeholder `dist` and shipped a web
app it never serves.

## Decision

- `apps/srcz/pkg/client` holds the `Client` interface, `Error`,
  `ErrNoDaemon`, `Stream`, and the HTTP transports (`NewTCP`, `NewUnix`).
  It imports only `pkg/contract`.
- The in-process transport is `apps/srcz/pkg/client/inprocess`
  (`inprocess.New(application, control)`, type `inprocess.Client`), with the
  same behavior as 0080's `NewInProcess`.
- `client.NewStream` builds a `Stream` for transports outside the package.
- The parity suite stays in `pkg/client` as an external test package and
  still runs every operation on all three transports.

## Consequences

- HTTP-only clients link neither the server nor the web app.
- In-process users import `pkg/client/inprocess` as well as `pkg/client`.
