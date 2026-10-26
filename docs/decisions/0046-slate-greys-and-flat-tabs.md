# 0046: Slate greys for Servef Light and flat tabs

- Status: Accepted
- Date: 2026-10-26
- Supersedes: [0043](0043-grey-document-surface-servef-light.md)'s grey document surface
- Amends: [0044](0044-servef-light-state-contrast.md) Servef Light values

## Context

After 0043 and 0044, Servef Light read washed out: the canvas, document,
explorer, and chrome greys sat within a few levels of each other
(`#ffffff`, `#f9f9f9`, `#f8f8f8`, `#f0f0f0`), so structure rested on
hairlines. The neutral greys also clashed with the cool slate text
(`#515866`, `#5d6676`). The active tab's rounded corners and concave
shoulders added a curve the rest of the flat, square-row workspace does not
use.

## Decision

Servef Light values (named themes and Servef Dark are unchanged):

- `bg`, `canvas`, and `surface` are `#ffffff`: the document is white again.
- Greys are cool slate, a clear step darker: `surface-secondary` (explorer)
  `#f1f2f4`, `workspace-chrome` `#e5e7eb`, `surface-recessed` `#eff1f4`,
  `border` `#d8dbe0`, `border-strong` `#c4c9d1`, `island-border` `#d3d7dd`.
- `field-border` `#7a808a`, so it holds 3:1 on the darker chrome (3.21:1).

Every theme:

- Tabs are flat: no rounded top corners and no concave shoulders. The active
  tab is a square block of the document surface, 4px below the strip top,
  with the 2px `selected-border` line on its top edge (0045).

## Consequences

- The document island reads white on the white canvas, held apart by the
  darker `island-border`; the explorer and chrome carry the grey.
- Muted text holds 4.5:1 on chrome (4.68:1) and on every surface the theme
  test checks.
- Dark and named themes lose their tab shoulders too.
