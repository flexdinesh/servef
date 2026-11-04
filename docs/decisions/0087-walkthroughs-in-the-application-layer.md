# 0087: Walkthroughs in the application layer

- Status: Accepted
- Date: 2026-11-04
- Revises: [0080](0080-application-layer-contracts-and-transports.md)
  (stage 2 for walkthroughs, the last domain in `internal/web`),
  [0047](0047-walkthroughs.md) and [0065](0065-walkthrough-publish-cli.md)
  (where the target checks live), [0066](0066-lease-levels-and-desktop-events.md)
  (where announcements fan out)

## Context

0085 left walkthroughs in `internal/web`: its handlers rendered narration,
mapped errors, announced publishes, and owned the 0047 target checks
(`web.NewWalkthroughTargets`), which `servef walkthrough publish` imported.
The native app (0082) may use only `pkg/contract` and `pkg/client`, which
had no walkthrough operations.

## Decision

- `app.Walkthroughs` composes `walkthrough.Service` with the target checks
  (`app.NewWalkthroughTargets`, shared by the CLI) and owns `List`, `Get`
  (Markdown rendered), `Delete`, `Publish`, `Schema`, and announcements
  (`Announce`, `Notices`, `CloseNotices`; the hub moved from `internal/web`).
  `internal/walkthrough` keeps the model, strict decoding, validation, the
  schema, and the store. `app.DecodeWalkthroughDraft` wraps the strict
  decoder for transports that receive JSON.
- `Publish` returns the view as a path and query (`/view?project=&wt=`);
  HTTP makes it absolute at the request's host and alone honors `open`
  (the server's browser opener). The Origin and JSON content-type guards and
  the 1 MiB body limit stay in `internal/web` as transport concerns.
- Shapes: `pkg/contract/walkthroughs.go` is generated; the draft is Go-only
  (`requests.go`); walkthrough codes join `codes.go`. The daemon's control
  stream and `daemon.WalkthroughNotices` use `contract.WalkthroughEvent`.
  The generator accepts `omitempty` on enums (absent when empty) and on
  pointers to structs (optional objects); rules may not name the latter.
- `pkg/client` adds `Walkthroughs`, `Walkthrough`, `PublishWalkthrough`,
  `DeleteWalkthrough`, and `WalkthroughSchema` on all three transports,
  covered by the parity suite.
- The web app uses the generated guards; `walkthrough-contract.ts` is gone.

## Consequences

- Wire format is unchanged. Every domain is now in `internal/app`;
  `internal/web` only adapts it to HTTP.
- Generated guards check structure and enums only: the hand-written checks
  (non-empty IDs and steps, non-negative counts, `[start, end]` lines, file
  or diff but not both) are gone, as in 0085. The server still validates
  every draft; the UI ignores lines that are not a pair.
