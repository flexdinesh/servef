# 0102: Sharp slate panes

- Status: Accepted
- Date: 2026-11-07
- Supersedes: [0041](0041-white-canvas-srcz-light.md)'s white canvas and island
  hairline; the Source Zero Light values in
  [0044](0044-srcz-light-state-contrast.md) and
  [0046](0046-slate-greys-and-flat-tabs.md); 0046's activity tile;
  [0056](0056-borderless-editor-tabs.md)'s tabs without separators
- Amends: [0033](0033-design-tokens-and-ui-primitives.md)'s radius tokens

## Context

Source Control's review drew a lighter 10px column beside its files: `main`
reserved its scrollbar gutter on `surface` while the review sat on
`workspace-chrome`, inset from the tab strip and the status bar. The column was
one symptom of the islands: every rounded card needed masks, insets, and a
hairline that only one theme drew. Zed shows the alternative: panes that meet on
hairlines, with one continuous multi-file buffer.

## Decision

**Every corner is square.** Radius tokens are gone, a base rule zeroes browser
defaults, and stylelint allows only `0` for `border-radius`. Pierre's trees and
diffs take the same rule through their injected CSS. Badges and the uncommitted
mark are square too. User-authored Mermaid node shapes (circles, stadiums) keep
their shape, which carries meaning; other diagram corners are square.

**Panes meet on 1px hairlines.** No canvas gaps or insets, on desktop or phones.
The sidebar's resizer is the hairline, with a 5px target. The status bar moves to
`workspace-chrome` with a top hairline. Overlays still float with their shadow.

**The review is one buffer.** Files stack without cards or gaps, divided by
sticky bands on `surface-secondary`, which holds the contrast floors for stats and
status letters. Code and collapsed-line bands run flush to the pane. The Files
rail docks full height on a leading hairline.

**The scrollbar belongs to its pane.** The review and `main` share `surface`, and
file bands end on a 1px hairline at the reserved gutter (0055), so the gutter
reads as the scrollbar's lane. The lane runs from the tab strip's hairline to the
status bar, level with the rail.

**Tabs and the activity bar.** Tabs take hairline dividers, and the active tab
cuts through the strip's line onto the document surface. The active activity row
fills on `surface` with a 2px accent edge.

**Slate Source Zero.** Source Zero Dark and Light's chrome move to cool blue-grey
values, around a near-white `#fafafa` document surface in Light. Contrast floors
take priority over the design's first values: `themes.test.ts` on surfaces, and
sampled diff fills for Dark's keyword, operator, type, and destructive inks.
Named themes keep their palettes and take the new structure.

**Mono chrome labels.** Tabs, view and pane headers, and the rail header use
`--font-chrome` (the system mono stack). Rows, buttons, settings, and prose stay
Inter. No new font ships.

Keyboard conventions and behavior stay VS Code's. Only the visual language moves.

## Consequences

- Stored theme choices keep working: ids stay `light` and `dark`, under the
  labels Source Zero Light and Source Zero Dark. Their colors change.
- Pane separation depends on `--color-border`, which every theme defines.
- Mono labels are wider, so tabs truncate sooner.
- Pierre's radius override is a universal `!important` rule in its shadow roots;
  a browser test walks those roots, so an upgrade that defeats it fails.
