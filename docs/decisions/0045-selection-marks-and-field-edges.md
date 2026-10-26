# 0045: Selection marks and 3:1 field edges

- Status: Accepted
- Date: 2026-10-26
- Amends: [0044](0044-servef-light-state-contrast.md)'s field border and tint-only states

## Context

WCAG 1.4.11 asks 3:1 for the visual information that identifies a control
and its state. Selected rows, the active tab, the active activity tile, menu
highlights, and pressed icon toggles showed state only through the selection
tint and text color (about 1.1-1.2:1 against their ground), and field borders
measured 1.3-2:1. A 3:1 tint would need a mid-blue fill with inverted text.

## Decision

In every theme:

- `--color-selected-border` (= `accent`, 3.3:1 or more on every theme's
  surfaces and on the selection tint) marks each state beside its tint, as VS
  Code does:
  - selected list and tree rows, the highlighted menu item, and a pressed
    IconButton: a 1px inset ring. A commit range takes one ring around the
    run, so no middle commit reads as the cursor;
  - the active tab: a 2px line on its top edge;
  - the active view: a 2px bar on the activity rail's left edge.
- `--color-field-border` is `text-muted` mixed 90% into `surface`; Servef Light
  sets `#858b94`. Both hold 3:1 against the field, the sidebar, chrome, and
  dialogs.
- `themes.test.ts` enforces both for every theme.

## Consequences

- Focus and selection share the accent ring; a selected row adds the tint, so
  a focused unselected row stays distinguishable.
- Segmented controls keep their lifted pressed item (shadow plus primary ink);
  hover layers stay tint-only, which WCAG does not cover.
