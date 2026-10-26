# 0046: Slate greys for Servef Light, flat tabs and toolbar

- Status: Accepted
- Date: 2026-10-26
- Supersedes: [0043](0043-grey-document-surface-servef-light.md)'s grey document surface
- Amends: [0044](0044-servef-light-state-contrast.md) Servef Light values and [0045](0045-selection-marks-and-field-edges.md)'s activity rail bar

## Context

After 0043 and 0044, Servef Light read washed out: the canvas, document,
explorer, and chrome greys sat within a few levels of each other
(`#ffffff`, `#f9f9f9`, `#f8f8f8`, `#f0f0f0`), so structure rested on
hairlines. The neutral greys also clashed with the cool slate text
(`#515866`, `#5d6676`). The active tab's rounded corners and concave
shoulders added a curve the rest of the flat, square-row workspace does not
use. Tabs and the Source/Preview/Format toggle sat 4px inside the 32px strip
rather than filling it, the active activity item drew both a tile and a rail
bar, and the source view's focus outline sat under its sticky line-number
gutter and scrolled away with the document.

## Decision

Servef Light values (named themes and Servef Dark are unchanged):

- `bg`, `canvas`, and `surface` are `#ffffff`: the document is white again.
- Greys are cool slate, a clear step darker: `surface-secondary` (explorer)
  `#f1f2f4`, `workspace-chrome` `#e5e7eb`, `surface-recessed` `#eff1f4`,
  `border` `#d8dbe0`, `border-strong` `#c4c9d1`, `island-border` `#d3d7dd`.
- `field-border` `#7a808a`, so it holds 3:1 on the darker chrome (3.21:1).

Every theme:

- Tabs are flat and fill the strip's height: no rounded top corners and no
  concave shoulders. The active tab is a square block of the document
  surface with the 2px `selected-border` line on its top edge (0045).
- The toolbar `SegmentedControl` (`md`) matches: no track, square segments
  that fill the strip, chrome hover, and the pressed one on the document
  surface. `sm` keeps its track.
- The active activity item shows only its tile, as VS Code highlights it; the
  rail bar is gone.
- Keyboard focus on the document pane or source view draws a 2px accent ring
  around the visible pane as an overlay above content. Review tabs with the
  Files rail keep the pane's own outline.

## Consequences

- The document island reads white on the white canvas, held apart by the
  darker `island-border`; the explorer and chrome carry the grey.
- Muted text holds 4.5:1 on chrome (4.68:1) and on every surface the theme
  test checks.
- Dark and named themes lose their tab shoulders too.
- The active activity view reads by its tile alone, a tint-level cue on
  chrome, below 0045's 3:1 mark; icon color and the tile's shadow carry it.
