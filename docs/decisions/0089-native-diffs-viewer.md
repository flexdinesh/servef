# 0089: Native diffs viewer

- Status: Superseded by [0094](0094-mygo-app-uses-web-views.md)
- Date: 2026-11-04
- Relates to: [0082](0082-native-mygo-experiment.md) (revises its review
  rendering: rows through `DiffState.SetPatch`), [0083](0083-server-structured-diffs.md)
  (`lineDiff`), [0086](0086-native-agent-tooling-and-ports.md) (the parity
  loop and `packages/mygo`), [0024](0024-source-control-view-and-working-tree-diffs.md),
  [0028](0028-diff-line-links-and-copy.md), [0034](0034-diff-anchor-reveal-and-hunk-navigation.md)

## Context

0082 showed the server's `lineDiff` in mygo's `editor.DiffView`, handing it
rows through `SetPatch`. DiffView either diffs two texts itself or shows a
patch's lines without the files around them, and its hunks are runs of
changes. It cannot show the server's hunks between collapsed gaps that
expand in place, mark a missing final newline, or select lines on a side as
the web UI does, and the Source Control milestone (M8) needs all of them.

## Decision

- `packages/mygo/diffs` is a generic diffs viewer, a port of what
  `@pierre/diffs` does for servef. It imports nothing from servef (0086) and
  never compares texts into hunks: callers pass both sides' lines with
  hunks, blocks, and gaps in its own model.
  - `diffs` (pure, no mygo; a test enforces it): rows for split and unified
    as Pierre lays them out (gaps of one line show, gaps expand up, down, or
    both by 100 lines, longer gaps offer Expand all, no-newline marks where
    Pierre puts them), word and character emphasis (jsdiff
    `diffWordsWithSpace` with Pierre's word-alt joining, presentation only),
    and line selection and hunk stepping with `diff-line-selection.ts` and
    `diff-hunks.ts` semantics (anchors such as `L5-R8`, `path:12-20`
    references, F7 cursors).
  - `diffs/diffview` draws it with its own virtualized `ui.List` rows, not
    DiffView: line-number gutters select (Shift extends), a host utility
    shows at the selection's end, F7 and Shift+F7 step hunks, Escape clears.
    Syntax colors come from the host through `Spans(side, line)`, none for
    lines of 1000 UTF-16 units or more (Pierre's tokenizeMaxLineLength).
- Bounded work per frame. Emphasis runs on the host's runner (`Run`, the
  app's off-UI-thread runner), cached by the pair's texts, so a reread
  that keeps a pair keeps its emphasis; the search stops past 400 edits or
  20 ms (jsdiff's maxEditLength and timeout), and such a pair, or a line
  over 1000 units (Pierre's maxLineDiffLength), shows no emphasis. Styled
  lines are built in one pass over colors and runs and cached per line
  until the diff, `SpansVersion`, or the look changes. Columns count
  display width: East Asian wide characters and most emoji take two,
  combining marks none.
- `apps/native/internal/review` converts `contract.DiffResponse` into the
  viewer's model. The review view keeps expansion and selection across live
  rereads, copies the selected lines' reference, and adds Previous and Next
  change buttons. Ignore whitespace stays the server's.

## Consequences

- Native matches the web UI's diff rows, gaps, emphasis, selection, and
  hunk steps (scenarios `changes-diff`, `diff-gaps`, `diff-hunk-step`,
  `diff-line-selection`, `diff-dark`).
- RSS (VmRSS from `/proc`, sampled by `test:native:e2e`: the `-tags e2e`
  app under Xvfb with llvmpipe, on the fixtures server, written to
  `native-e2e/rss.txt`, on 0088's Highlights and 0090's trees): 136 MiB
  with a Markdown preview open, 142 MiB after opening code/main.go's diff
  in the full-file view. 0082 sampled 120 MiB on a daemon with the old
  viewer and Chroma; the builds and hosts differ, so the in-run step
  (+5.5 MiB) is the comparable figure.
- Not yet: wrap lines, the inline changes menu, links and anchor reveal
  from a link, stepping to the next file (one file shows at a time).
