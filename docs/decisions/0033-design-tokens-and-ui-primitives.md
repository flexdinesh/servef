# 0033: Design tokens and UI primitives

- Status: Accepted
- Date: 2026-10-23

## Context

The UI grew one feature at a time in a single `style.css`. Each view rebuilt
its own icon buttons, fields, rows, and overlays. A sweep found about 20
icon-button classes (6 sizes, 3 glyph sizes, 2 rest colors, 3 hover fills, 3
radii, 4 focus offsets), 6 field implementations with 3 focus treatments, 22 to
32px rows, and overlay radii from 4 to 8px. Design changes did not propagate,
and nothing stopped the next feature from adding a seventh field.

## Decision

- Styles live beside their owners. `web/src/style.css` is only the import
  manifest and fixes the cascade layers:
  `theme, base, components, utilities, foundation, ui, features`. Tokens and
  themes are unlayered; `styles/base.css` is `foundation`; primitives are `ui`;
  every feature file, `layout.css`, and `print.css` (imported last) are
  `features`.
- Three token tiers in `styles/tokens.css`:
  1. Theme values: `light-dark()` colors in Tailwind's `@theme`, resolved by
     the root `color-scheme`. Named themes override them in `themes.css`.
  2. Semantic roles: state layers (`--color-hover`, `--color-active`,
     translucent primary ink), `--color-selected(-text)`, `--color-focus`,
     field colors, type roles, motion, z-index, island and overlay radii.
  3. Component tokens: control, icon, row, chrome, field, and status sizes.
- Hand-owned React primitives in `web/src/components/ui/`, each with
  co-located CSS and data-attribute variants: Button, IconButton,
  ToggleButton, SegmentedControl, TextField, Select, ListRow/ListButton,
  Badge, Notice/StatusMessage, ProgressBar, CopyButton, ViewHeader,
  SectionToggle/Chevron, Kbd, DialogShell, and Menu class names for Base UI.
  Features compose them; behavior stays in features.
- One design language, VS Code conventions: square 22px list rows (32px on
  narrow layouts), 28px default buttons (44px touch), text-secondary icon and
  row ink, a translucent hover layer, selected rows in selection fill and ink,
  a 1px inset accent focus outline on rows and icon buttons (a 2px outline
  outside standalone buttons), fields with an accent border plus 1px ring,
  full destructive border on invalid fields, and one overlay surface (6px
  radius, strong border, raised fill, overlay shadow) for dialogs, the
  drawer, popovers, and menus.
- Enforcement: oxlint replaces ESLint and errors on raw `button`, `input`,
  `select`, `textarea`, and `dialog` outside `components/ui/`, alongside the
  Hooks rules. Stylelint errors on color literals, raw font sizes, radii,
  z-indexes, duration literals, and px above 2 outside the token files.
- A dev-only gallery at `/__ui` in `pnpm dev:mock` shows every primitive and
  state under a theme switcher. Production builds drop it.
- Tailwind stays for its token theme and utilities. Primitives do not use
  Tailwind classes; `class-variance-authority` is no longer used by them.
- No screenshot tests. Browser tests assert shared geometry and states in
  `tests/browser/design-consistency.spec.ts`.

## Consequences

- A token or primitive change propagates to every view.
- New UI composes primitives; a missing capability is added to the primitive,
  not forked in a feature.
- Activity bar, tab strip, Pierre trees and diffs, prose, and the Mermaid
  canvas remain feature CSS on tokens; they follow the same state tokens.
- Visual regressions are caught by review of the gallery and the consistency
  spec, not pixel diffs.
