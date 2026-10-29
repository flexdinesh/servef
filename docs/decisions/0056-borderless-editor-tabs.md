# 0056: Borderless editor tabs

- Status: Accepted
- Date: 2026-10-29
- Amends: [0046](0046-slate-greys-and-flat-tabs.md) and
  [0045](0045-selection-marks-and-field-edges.md)'s active-tab selection mark

## Context

The requested VS Code-style refinement replaces the active tab's accent rule
with a quieter surface treatment. Gaps and inactive separators add competing
edges to the compact strip.

## Decision

- Tabs remain square and fill the strip, with no gaps, separators, or active
  accent rule. The active tab joins the document surface; inactive tabs remain
  on workspace chrome and alone receive the chrome hover layer.
- The tab owns hover, including over its label. Keyboard focus retains its
  inset accent outline, and selection retains `aria-selected`.
- With a precise hovering pointer, inactive close actions appear on hover or
  focus within the tab. Their space stays reserved. Other devices keep all
  close actions visible; the active close action is always visible.
- When the strip narrows, reveal the whole active tab, including its close
  action, rather than only the label.

## Consequences

- Tab selection relies on surface continuity and primary label ink rather
  than 0045's explicit 3:1 accent mark. This intentionally reduces the visual
  contrast of selection; keyboard focus remains explicit.
- Tab selection, sizing, themes, and context menus retain their behavior.
