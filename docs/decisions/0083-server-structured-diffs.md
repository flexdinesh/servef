# 0083: Server-side structured diffs and the source control domain

- Status: Accepted
- Date: 2026-11-03
- Revises: [0024](0024-source-control-view-and-working-tree-diffs.md) (diffs
  parse in a browser worker), [0080](0080-application-layer-contracts-and-transports.md)
  (stages 2 and 3 for source control)

## Context

Stage 3a of 0080 moved projects, documents, and events into
`internal/app`. Source control still lived in `internal/web` beside HTTP,
with hand-written TypeScript guards. The browser parsed every diff with
jsdiff in a worker, and derived hunks, collapsed gaps, hunk steps, line
targets, and stats from Pierre's parse. Every client would have to repeat
that work, and match its layout exactly, for selections and anchors to agree.

## Decision

### Source control in the application layer

- `app.SourceControl` owns changes, diffs, commits, commit files, branch
  files, range files, refs, branch stacks, and compare stats as typed
  operations that return `pkg/contract` types or `*app.Error`. Messages,
  codes (`unchanged`, `unknownBase`, `unknownHead`), and kinds are unchanged.
  `internal/web` only maps query parameters and results.
- The shapes live in `pkg/contract/sourcecontrol.go` and generate into
  `@servef/contracts`; request parameters are Go-only (`requests.go`), as are
  the error codes (`codes.go`). The web app's hand-written source control
  guards are gone.
- The generator accepts pointers to scalars with `omitempty` as optional
  fields (`FileStats.additions`, `DiffSide.content`). Rules may not name them.
- `pkg/client` offers every operation on all three transports, covered by the
  parity suite.

### Structured diffs: `internal/diff`

- `/api/diff` adds `lineDiff` when both present sides carry text (null for a
  binary or too-large side): the hunks, each with its blocks (a run of
  unchanged lines, or one change of deleted then added lines), the unchanged
  gaps a hunk view collapses, and the addition and deletion counts.
  `ignoreWhitespace=1` compares lines without leading and trailing
  whitespace.
- Line numbers are 1-based on each side; a side with no lines in a hunk or
  block starts at the line that follows it. Clients need no hunk-header
  conventions.
- Hunks match what jsdiff and Pierre lay out for the same sides: jsdiff's
  Myers search with its tie-breaks among equal-cost paths, four lines of
  context, hunks joined across at most eight unchanged lines, and Pierre's
  realignment of uneven changes by line similarity. A reference set of 815
  cases (unicode, whitespace, CRLF, missing final newlines, 1200-hunk files)
  matched exactly when this was decided.
- Bounds: past a 250 ms search the diff falls back to a time-bounded bisection
  (1 s); past 16384 change runs it coarsens to one change. Either way the
  view stays correct, only less minimal. Response size follows: the 0004
  limits bound each side at 4 MiB, and the change-run bound caps the hunks.
- `diff.Count` (numstat counts for compare stats) replaces `git.LineChanges`;
  the 64 MiB compare-stats budget is unchanged.

### The web app renders server diffs

- A pure adapter (`diff-metadata.ts`) lays `lineDiff` and the two sides out
  as Pierre's `FileDiffMetadata`; a test requires it to equal Pierre's own
  parse for every fixture. The diff worker and the browser's jsdiff run are
  gone.
- Hunk selection and steps, gap expansion, and line targets read the server's
  hunks, blocks, and gaps. Presentation (layout, highlighting, selection,
  annotations) stays in the client.
- When text sides arrive without a line diff, the file says "Couldn't compute
  this diff."

## Consequences

- Each text diff costs server CPU and memory. On the dev fixture, five reviews
  of a branch with uncommitted changes, including a 1.6 MB file with 1200
  hunks, moved servef from about 38-41 MB RSS (43-45 MB peak) to 43-51 MB
  (52 MB peak); the large diff takes about 0.1 s on the server and its
  response grows from 3.3 MB to 3.8 MB. The browser no longer parses.
- The search allocates only when a path changes run kind, and its diagonal
  array grows with the edit distance, so memory follows the change, not the
  file size.
- Generated guards check structure, enums, and rules; value checks the
  hand-written guards made (non-empty paths, hash formats, non-negative
  integers, budget-only `budget` stats) are now the server's contract alone.
- A compare-stats query with several invalid parameters may report a
  different one first; empty parameters count as absent.
