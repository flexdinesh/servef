# 0080: Application layer, generated contracts, and transports

- Status: Accepted; revised by [0083](0083-server-structured-diffs.md) (source control stages), [0084](0084-in-process-client-package.md) (in-process transport package), [0085](0085-image-metrics-and-navigation-in-the-application-layer.md) (navigation, metrics, image reads), [0087](0087-walkthroughs-in-the-application-layer.md) (walkthroughs)
- Date: 2026-11-03
- Revises: [0078](0078-apps-and-packages-layout.md) (creates `packages/`)

## Context

Domain behavior lives in `apps/srcz/internal/web` beside HTTP: handlers
resolve projects, read files, and choose `http.Status*` codes in the same
functions (about 180 uses). Only the HTTP server can run it. The desktop
app, the CLI, and a planned native shell need the same operations over a Unix
socket, over TCP, or in process.

Transport shapes are written twice: Go DTOs in `internal/web` and
hand-written TypeScript runtime guards in `apps/web/src/api/*-contract.ts`.
The two drift independently; only fixtures tie them together.

Some domain logic runs in the browser that every client would need: search
matching and scope globs, diff parsing into hunks, gaps, anchors, and stats.

## Decision

Make the server composable across transports in stages. This record covers
the whole design; stage 3a (this change) lays the foundation.

### Application layer: `apps/srcz/internal/app`

- Per-domain services (`Projects`, `Documents`, `Events`; later source
  control, navigation, walkthroughs, metrics) with typed operations. They take
  `pkg/contract` requests and return `pkg/contract` types. They know no
  `net/http`.
- Failures are `*app.Error`: a kind (`contract.ErrorKind`: invalid, not
  found, forbidden, too large, unsupported, unprocessable, unavailable,
  internal), an optional code (`contract.Code*`), and the user-facing message.
  Each transport maps the kind; HTTP uses `contract.HTTPStatus`, a bijection,
  so clients recover the kind from a status (`contract.KindForStatus`).
- A page always returns its data; a failed page also returns the error whose
  message it carries, so the page's status follows its kind.
- Domain packages (`projects`, `document`, `files`, `git`, `watch`) keep
  their own types; services map them to contract types and keep nil slices
  nil so encodings stay byte-compatible.
- The daemon's control operations become typed methods of
  `daemon.Controller` (`Status`, `Roots`, `Open`, `Subscribe`) and
  `daemon.Directories`; its HTTP handler only decodes, maps, and encodes.

### Contracts: `apps/srcz/pkg/contract` and `packages/contracts`

- `pkg/contract` is the single source of truth for transport shapes: JSON
  bodies (field order is wire order), `ErrorResponse` (`{error, code?}`),
  error codes and kinds, request parameters, and stream events.
- `tools/contracts` (standard library only: `go/parser`) generates
  `packages/contracts` (pnpm package `@srcz/contracts`): one TypeScript
  module per listed Go file with interfaces, string unions, and runtime type
  guards that use no `as`, `any`, or `!`. Generation supports exported structs
  with JSON tags, string enums with typed constants, scalars, slices, pointers
  to structs (`T | null`), and `omitempty` strings (optional). Anything else
  fails generation rather than producing a weaker guard.
- Invariants a structural guard cannot express are declared on the Go type as
  `//contract:rule <field> ==|!= <literal> => <field> ==|!= <literal>`; the
  guard enforces each rule.
- The generator lists every contract file as generated or Go-only. Go-only
  files are docs, the kind mapping, request parameters, and shapes whose
  TypeScript guards are still hand-written until they migrate (stream events,
  daemon control). An unlisted file fails generation.
- The output is committed. `mise run contracts:generate` writes it;
  `mise run contracts:check` (repo host group) fails on drift or stray files.
  Never edit `packages/contracts/src`.
- `testdata/api` fixtures of generated shapes are validated by the generated
  guards (`apps/web`), by the generator's model including rules and unknown
  keys (`tools/contracts`), and by strict Go decoding (`internal/web`).

### Client: `apps/srcz/pkg/client`

- One `Client` interface: the data API, the project event stream, and the
  daemon control calls apps need (status, roots, open, directories, events).
- Three transports with identical results: HTTP over TCP (`NewTCP`, a CLI
  server), HTTP over a Unix socket (`NewUnix`, the daemon), and in process
  (`NewInProcess`, the application layer plus an optional
  `daemon.Controller`). Daemon calls on a transport without a daemon return
  `ErrNoDaemon`. Failures are `*client.Error` with the kind, code, and
  message on every transport. Streams yield typed events until `io.EOF`.
- One parity suite runs every operation on all three transports and requires
  equal results and errors.
- `internal/daemon.Client` keeps serving the CLI (ensure, stop, announce,
  protocol checks) until those calls move to the client.

### HTTP adapter: `apps/srcz/internal/web`

`internal/web` becomes the HTTP adapter: routing, the local-Host check, CSP,
JSON encoding, kind-to-status mapping, server-sent events, and raw byte
streaming (`/api/raw`, `/api/image`, whose failures stay plain text). Every
JSON error stays `errorResponse` (now `contract.ErrorResponse`). Responses
stay byte-compatible.

### Domain logic on the server, presentation in clients

Logic every client needs moves to the application layer: search matching and
include/exclude globs, diff parsing into hunks, collapsed gaps, line anchors,
and stats. Clients keep presentation: layout, interaction, highlighting, and
view state. Each move measures srcz RSS (and browser memory where it
changes) before and after on the dev fixture, records the numbers in its PR,
and bounds response sizes, as the memory priority requires.

### Stages

1. 3a (this change): the application layer with projects, documents (page,
   format, search documents, raw and image reads), and events; contracts and
   generated TypeScript for projects, documents, and errors; the client with
   all three transports; the daemon controller.
2. Remaining domains (source control, navigation, walkthroughs, metrics), and
   generated TypeScript for events and daemon control; the desktop app
   imports `@srcz/contracts` instead of web contract files.
3. Shared domain logic moves from the web client to the server.

## Consequences

- `packages/contracts` is the first shared package; it joins
  `pnpm-workspace.yaml`, and `apps/web` depends on it (`workspace:*`). Its
  package exports TypeScript source, so Vite, `tsc`, and Node's type
  stripping read it without a build step.
- Validation selects `packages/` for the groups that build or test the web
  app, `packages/contracts/` for the repo group, and installs the package
  manifest everywhere; Dagger copies it into web builds.
- Changing a transport shape means editing `pkg/contract`, regenerating, and
  updating fixtures; hand-written guards remain only for shapes not yet
  migrated.
- Dependency direction: `web -> app -> domain packages`; `app`, `web`, and
  `client` depend on `pkg/contract`; `client` depends on `app` and `daemon`
  for the in-process transport.
