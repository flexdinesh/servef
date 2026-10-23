# 0034: Diff anchor reveal and hunk navigation

- Status: Accepted
- Date: 2026-10-23
- Extends: [0028](0028-diff-line-links-and-copy.md)

## Context

0028 highlighted a diff anchor but did not scroll to it: Pierre's React diff
exposed no instance and no scroll-to-line API. An anchor on an unchanged line
between hunks selected rows that stayed collapsed. Reviewing a long diff meant
scrolling by hand from change to change.

## Decision

- A diff anchor reveals its first line on load and whenever it changes from a
  link, history, or a hunk step, near the pane's top third as in Source (0008),
  unless the line is already visible. A gutter selection never scrolls.
- An anchor end inside a collapsed gap expands that whole gap.
- F7 and Shift+F7 (VS Code's diff editor bindings), while focus is in the
  document pane, and the toolbar's Previous change and Next change buttons
  select the next or previous Pierre hunk, context included, and reveal it.
  Steps start from the selection, else from the pane's top, and wrap within
  the file. The selection is written as a 0028 anchor, so a hunk link is a
  range link: `#R17-R25`, or `#L1-R5` when the hunk starts with deletions in
  Unified. There is no hunk-index anchor; hunk indexes shift when the file
  changes or whitespace is ignored.
- A local `pnpm patch` of `@pierre/diffs` adds `onInstance` to React
  `FileDiff`. servef reads row positions (`getLinePosition`) and expands gaps
  (`expandHunk`) through the instance and scrolls the document pane itself.
- Document navigation leaves scroll alone for fragment-only changes when no
  document is selected: a diff tab owns its scroll.
- Hunk rows, cursors, steps, and gaps live in the pure
  `web/src/features/source-control/diff-hunks.ts`.

## Consequences

- Supersedes 0028's "does not scroll to it on load".
- The patch must be reapplied or dropped on each `@pierre/diffs` upgrade; drop
  it when Pierre exposes the instance or a scroll-to-line API. Pierre's
  `CodeView` has one but owns a nested scroller, which would move pane focus,
  Escape, keyboard scrolling, and notices.
- Positions of rows not yet rendered are Pierre's estimates; with Wrap on, a
  reveal can land slightly off.
- Expanded gaps last until the diff remounts (a Font Size change or reparse).
