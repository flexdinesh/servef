# 0028: Diff line links and copy

- Status: Accepted; no-scroll consequence superseded by [0034](0034-diff-anchor-reveal-and-hunk-navigation.md)
- Date: 2026-10-22
- Extends: [0008](0008-line-links-and-copy-path.md), [0024](0024-source-control-view-and-working-tree-diffs.md)

## Context

Source selects lines and copies `path:12-20` (0008). Diff tabs copied only the
path, and 0024 left them without line selection. Reviewing a change and
handing lines to an editor, review, or agent meant opening the file first.

## Decision

- Diff line numbers select lines as in Source: click, drag, or Shift-click;
  Escape clears while focus is in the document pane. Pierre's line selection
  draws the range; its gutter utility holds the copy button at the range's
  last line.
- The URL fragment owns the selection, GitHub diff style: `L` is the old side
  (HEAD, or the commit's parent), `R` the new side. `#R12`, `#R12-R20`
  (`#R12-20` too), `#L5-L7`, and across sides `#L5-R8`. One side writes top
  down; across sides the drag order is kept. Changes replace history. Diff
  routes read only these anchors.
- Copied references stay editor-style and use the side's own numbers: new
  side `path:12-20`, old side `oldPath:5-7` (the HEAD path of a rename). A
  range across sides copies the new lines between its ends, in the rows as
  shown: Unified lists a change's deletions before its additions, Split pairs
  them. An end on an old line moves inward to the nearest new line.
- The footer shows `Ln 12–20` beside `+N −M`, or `Old Ln 5–7` for the old
  side. The footer still copies the path.
- Resolution lives in the pure `web/src/diff-line-selection.ts`, from the
  parsed hunks; unchanged lines between hunks keep their numbers on both
  sides.

## Consequences

- Lines past either file's end select nothing; anchors do not clamp.
- A diff anchor highlights its range but does not scroll to it on load:
  Pierre's React diff exposes no scroll-to-line API.
- The same selection can copy different new-side ranges in Split and
  Unified when it crosses sides, because the rows between its ends differ.
- The copy button sits at the range's last line, where Pierre places its
  utility; Source shows it at the first line.
