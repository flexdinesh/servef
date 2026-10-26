# 0043: Grey document surface for Servef Light

- Status: Accepted
- Date: 2026-10-26
- Amends: [0041](0041-white-canvas-servef-light.md)'s white document island

## Context

0041 set Servef Light's islands on a white canvas but left the document
surface white, so source and diff content read as white on white, held apart
only by a 1px hairline. The hairline was an inset outline that child
backgrounds (the active tab, source rows, review content) painted over.
Source Control pane dividers used `border` (`#ebebeb`), nearly invisible on
the `#f8f8f8` explorer.

## Decision

- Servef Light's `surface` is `#f9f9f9`: the document island floats on the
  canvas as the explorer does. Text, accent, status, and syntax colors keep
  4.5:1 (muted text 4.82:1).
- A new `--color-surface-recessed` fills what is set into the document: the
  active source line, code blocks, inline code, table headers, diff
  separators, and the image checkerboard. It is `surface-secondary` in every
  theme except Servef Light, where it is `#f4f4f4`, since `surface-secondary`
  (`#f8f8f8`) no longer contrasts with `surface`.
- The island edge paints as an overlay above island content instead of an
  inset outline.
- Source Control panes divide with `border-strong` in every theme: they
  resize against each other, a boundary rather than a separator.

## Consequences

- Fields, segmented lifts, and the active activity tile, which use `surface`,
  take the same grey in Servef Light; `surface-raised` overlays stay white.
- Named themes are unchanged except their pane dividers.
