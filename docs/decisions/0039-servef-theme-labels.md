# 0039: Servef Light, Servef Dark, and Auto theme labels

- Status: Accepted
- Date: 2026-10-25
- Supersedes: [0036](0036-omarchy-themes.md) and [0037](0037-dark-default-theme.md) labels for the built-in themes

## Context

Beside named palettes, the built-in "Light" and "Dark" read as generic
polarities rather than servef's own themes, and "System" is clearer as "Auto".

## Decision

- Labels: `light` is Servef Light, `dark` is Servef Dark, `system` is Auto.
- Ids are unchanged, so stored `servef-theme` values keep working. Order and
  grouping are unchanged: Auto, Servef Light, Servef Dark, then named themes.

## Consequences

- Settings, the Color Theme quick pick, and palette values show the new labels.
