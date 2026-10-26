# 0044: Servef Light state and boundary contrast

- Status: Accepted
- Date: 2026-10-26
- Amends: [0041](0041-white-canvas-servef-light.md) and [0043](0043-grey-document-surface-servef-light.md) Servef Light values

## Context

A full review of Servef Light (design review plus a rendered contrast scan)
found that its greys sat 4-5 levels apart, so the current tab, active
activity tile, active source line, review file islands, and selected rows read
mostly by text color. Hover (8% ink) on the chrome bands read darker, and so
more current, than the active tab or tile. Muted and success text fell under
4.5:1 on hovered and selected rows (4.14:1 at worst), and added diff lines,
mixed from the dark success green, read grey.

## Decision

Servef Light values (named themes and Servef Dark are unchanged):

- `workspace-chrome` and `surface-recessed` `#f0f0f0`, a 1.08:1 step under the
  `#f9f9f9` surface instead of 1.045.
- `border` `#e3e3e3`; `field-border` `#c5cbd4`, a step above `border-strong`.
- `accent-subtle` (the selection tint) `#e0e9fd`: 1.15:1 on the explorer.
- `text-muted` `#5d6676` and `success` `#13703f`: 4.5:1 or more on hover,
  selection, chrome, and recessed fills.
- `warning-subtle` `#ffeab3`, so search matches read on the explorer.
- New `--color-chrome-hover`: halfway between chrome and `surface`, so hover
  on tabs and activity tiles moves toward the active tone. Other themes keep
  `--color-hover`.
- Added diff lines mix a vivid green (`#00c853`) at Pierre's 12% strength.
- Review file islands take the `island-border` hairline.

Shared, since each was a state or boundary that did not render in any theme:

- Accent text on the selection tint uses `selected-text`; a pressed
  IconButton takes the selection tint; selected commit and branch rows take
  selection ink.
- The Source Control header ends on a `border` line; pane dividers draw after
  a sash too.
- Settings items hover with `surface-recessed`; the review Files rail's tree
  sits on `surface`.

## Consequences

- Selection, active tab, and hover remain tint-level cues (about 1.1-1.2:1);
  text color, weight, shadow, or the tab's connected shape carries the rest,
  as in VS Code.
- Field borders stay below WCAG 1.4.11's 3:1, as in VS Code's light themes;
  labels and placeholders identify fields.
