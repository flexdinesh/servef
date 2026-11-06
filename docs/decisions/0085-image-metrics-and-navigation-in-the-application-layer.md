# 0085: Image reads, metrics, and code navigation in the application layer

- Status: Accepted; walkthroughs followed in [0087](0087-walkthroughs-in-the-application-layer.md)
- Date: 2026-11-04
- Revises: [0080](0080-application-layer-contracts-and-transports.md)
  (stage 2 for navigation and metrics; image reads in the client),
  [0082](0082-native-mygo-experiment.md) (native's image side request)

## Context

0080 left code navigation (definitions, references, status) and process
metrics in `internal/web`, with hand-written TypeScript guards, and gave
`pkg/client` no image read. The native app (0082) read `/api/image` beside
`pkg/client`, the one request it made outside the client.

## Decision

- `app.Navigation` owns `Status`, `Definition`, and `References`: project
  resolution, indexed and ignored-path checks, size and binary limits,
  snapshots, location mapping, and failure kinds and codes. `internal/lsp`
  still owns the servers; `app.Config.LanguageServers` hands the manager in,
  and its owner closes it. `X-Srcz-Language-Servers: on` becomes the
  request field `LanguageServers` (`contract.LanguageServersHeader` names
  the header); status never starts a server.
- `app.Metrics` samples the process (`Sample`); `app.Documents.ReadImage`
  reads what `OpenImage` opens, up to `contract.MaxImageBytes` (32 MiB).
- Shapes: `pkg/contract/navigation.go` and `metrics.go` are generated;
  `media.go` (the image read, its limit and messages) and the navigation
  requests and codes are Go-only. The generator accepts `omitempty` slices
  (optional arrays; rules may not name them) and pointers to scalars without
  `omitempty` (`T | null`).
- `pkg/client` adds `Image`, `NavigationStatus`, `Definition`, `References`,
  and `Metrics` on all three transports, covered by the parity suite (a fake
  language server answers navigation). Over HTTP an image failure stays
  plain text: a 404 is `contract.ImageUnavailableMessage`, and the client
  enforces the size limit.
- The web app uses the generated guards; `navigation-contract.ts` and the
  metrics guard are gone. Native reads images with `Client.Image`.

## Consequences

- Wire format is unchanged; `/api/image` and `/api/raw` keep streaming any
  size to browsers.
- Generated guards check structure and enums only: the hand-written range
  checks (positive lines, non-negative counts, ordered ranges) are gone, as
  in 0080 and 0083. The server still drops locations with invalid positions.
- Walkthroughs remain in `internal/web`.
