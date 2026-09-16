# servef design system

This document is the visual constitution for `servef`. The interface exists to make a local Markdown tree easy to scan and a selected document comfortable to read. Product behavior and document content lead; the interface stays quiet.

Implementation source of truth: `web/src/style.css`. Tailwind theme variables define the shared visual primitives. Semantic classes define recurring product patterns and rendered Markdown, where utility classes are not practical.

## Design direction

The interface should feel:

- calm and content-first
- compact in navigation, generous in reading areas
- modern without chasing trends
- precise, quiet, and dependable
- native to a developer tool without looking unfinished
- equally intentional in light and dark environments

Avoid decorative gradients, ornamental color, heavy shadows, excessive cards, pill-shaped containers, novelty typography, dense toolbars, and visual detail that competes with the document.

## Existing UI audit

The original UI established the right product structure: one compact header, a persistent folder tree, a narrow reading column, a command-style search dialog, and a single mobile collapse point. Preserve that direction.

Before consolidation, styling used:

- system `Canvas`, `CanvasText`, and `LinkText`; neutral mixes at 8%, 10%, 14%, 16%, 18%, 20%, 22%, 58%, 60%, 62%, 65%, 66%, and 72%; standalone error, warning, highlight, and backdrop colors
- `system-ui` for UI/body and `ui-monospace` for code
- explicit text sizes at `.72rem`, `.8rem`, `.84rem`, `.85rem`, `.88rem`, `.9rem`, and `1.05rem`, plus browser-default headings and body text
- weights of 650, 700, and 750 without named roles; global line-height `1.55`
- recurring but irregular spacing from `.1em` through `3rem`, including `.15rem`, `.2rem`, `.35rem`, `.45rem`, `.55rem`, `.6rem`, `.65rem`, `.7rem`, and `1.1rem`
- radii at `.15rem`, `.25rem`, `.3rem`, `.35rem`, `.4rem`, and `.7rem`
- a small control shadow and a large dialog shadow, both derived independently
- a 52rem reading width, 44rem dialog, 14–19rem fluid sidebar, 3.25rem header, 2rem diagram controls, and a 700px breakpoint

The repeated neutral mixes, near-identical small type sizes, fractional spacing, and radii expressed incidental differences rather than useful hierarchy. They are consolidated below. The two-pane layout, constrained reading measure, restrained borders, native disclosure controls, search model, and responsive stacking remain.

## Design principles

1. Content is the product. Navigation helps users reach it and then recedes.
2. Consistency beats novelty. Reuse a token or shared pattern before inventing one.
3. Hierarchy comes from type, spacing, and contrast before borders or decoration.
4. Spacing communicates grouping. Tight spacing binds related items; larger spacing separates concepts.
5. Use semantic color roles, never palette values in component rules.
6. Keep fewer visual styles with clearer jobs. A meaningful difference must explain every variant.
7. Preserve the tree-to-document relationship at every viewport.
8. Responsive behavior preserves hierarchy and access; it does not merely shrink desktop UI.
9. Motion is brief, functional, and optional.

## Color

Use only semantic tokens:

- `--color-bg`: browser and application canvas
- `--color-surface`: primary reading and header surface
- `--color-surface-secondary`: navigation, code, table headers, and subtle interactive fills
- `--color-surface-raised`: menus, dialogs, and controls that sit above another surface
- `--color-text-primary`: headings, body text, and important controls
- `--color-text-secondary`: supporting copy, tree labels, and descriptions
- `--color-text-muted`: metadata, captions, placeholders, and de-emphasized labels
- `--color-border`: ordinary separation
- `--color-border-strong`: emphasized boundaries and elevated control edges
- `--color-accent`, `--color-accent-hover`, `--color-accent-subtle`: links, focus, selection, and the current file
- `--color-on-accent`: content placed directly on an accent fill
- `--color-success`: successful status
- `--color-warning`, `--color-warning-subtle`: recoverable warnings and search highlights
- `--color-destructive`, `--color-destructive-subtle`: failures and destructive actions
- `--color-overlay`: modal backdrop only
- `--color-syntax-*`: fenced-code token roles; comments, keywords, types, functions, variables, strings, numbers, and operators

All themes use the same roles. The control offers Light, Dark, System, and named editor palettes. Each palette declares light/dark polarity; System follows `prefers-color-scheme`. Persist the configured choice locally, default to System, and keep `color-scheme`, Mermaid, tldraw, and Excalidraw synchronized with resolved polarity. Never branch component styling into unrelated dark variants.

Accent is functional, not decorative. Use it for navigation selection, links, focus, and primary action emphasis. Do not use it on large surfaces or routine headings.

Primary text is the default. Secondary text supports it. Muted text is never the sole treatment for essential instructions. Borders separate adjacent regions or define interactive controls; do not outline every section.

Status meaning must pair color with text, iconography, or another non-color cue. Introduce no additional neutral shade when an existing surface, text, or border role works.

## Typography

Inter Variable is the UI and reading face. The system monospace stack is reserved for code. Load fonts locally so the viewer remains useful without internet access.

The type scale uses 16px body copy and 14px interface text while retaining compact metadata and clear document headings:

| Role | Token | Use |
| --- | --- | --- |
| Page/document title | `--text-3xl` | Markdown `h1`; one primary title per document |
| Section heading | `--text-2xl` | Markdown `h2` |
| Subsection heading | `--text-xl` | Markdown `h3` |
| Minor heading | `--text-lg` | Markdown `h4`, prominent empty-state title, large search input |
| Body | `--text-base` (16px/24px) | Markdown prose and default UI copy |
| Small body/label | `--text-sm` (14px/20px) | navigation, result titles, alerts, supporting UI |
| Metadata/caption | `--text-xs` (12px/16px) | paths, statuses, overlines, secondary search text |

Use weight 700 for document hierarchy, 600 for selected items and labels, and 400–500 for copy. Headings use tight letter spacing and compact line height. Prose uses a 1.75 line height. Labels may use uppercase only at `--text-xs`, with deliberate tracking.

Do not create typography styles to compensate for poor spacing or color hierarchy. Add a new type role only when content has a distinct, recurring semantic level that the table cannot express.

## Spacing

Use the 4px-based scale:

| Token | Size |
| --- | --- |
| `--space-1` | 4px |
| `--space-2` | 8px |
| `--space-3` | 12px |
| `--space-4` | 16px |
| `--space-5` | 24px |
| `--space-6` | 32px |
| `--space-7` | 48px |
| `--space-8` | 64px |

Use 4–12px inside compact controls, 12–24px inside larger components, 24–32px between related content groups, and 48–64px between document sections. Default desktop page padding scales from `--space-5` to `--space-7`; mobile uses `--space-4` horizontally.

Keep label-to-control spacing at 8px, related action gaps at 8–12px, standard control padding at 8–12px, and alert/card-like inset spacing at 12–16px. Document block flow defaults to 16px; headings create the larger section rhythm.

Custom spacing is acceptable only for optical alignment, intrinsic media sizing, or a layout calculation. Comment non-obvious exceptions. Do not approximate a scale value with a nearby one-off value.

## Layout

- App structure: 40px header above a full-height workspace with a resizable 13–30rem sidebar, 28px tab bar, flexible document pane, and 24px status line.
- Reading measure: `--reading-width` (50rem) maximum, centered in the document pane.
- Desktop gutters: fluid 24–48px around the reading column.
- Mobile gutters: 16px.
- Search overlay: maximum 42rem wide and 40rem tall; it never touches viewport edges.
- The sidebar and document own independent scrolling on desktop.
- Align document headings, prose, tables, diagrams, alerts, and empty states to the same reading column.
- Do not independently center elements that belong to the reading grid.

Use full-width application layout for navigation and document browsing. Use the narrow reading measure for prose. Add grids only when content has parallel, comparable items; do not turn document blocks into dashboards.

The single structural breakpoint is 48rem. Add another only when content demonstrably fails, not for a specific device model.

## Borders, radii, and shadows

- `--radius-sm` (6px): compact tree rows, key hints, inline code
- `--radius-md` (8px): controls, code blocks, diagrams, alerts
- `--radius-lg` (12px): overlays and dialogs only
- `--shadow-control`: subtly lifts bordered controls
- `--shadow-overlay`: modal elevation only

Prefer whitespace or a surface change before adding a border. Prefer a border before adding a shadow. Use shadows only when an element is genuinely elevated. Sections of a Markdown document are not cards.

## Component density and sizing

- `--control-height-sm` (32px): compact desktop navigation and grouped diagram controls
- `--control-height-md` (40px): primary controls and mobile navigation rows
- Touch targets should reach 40px in dense navigation and 44px when controls stand alone.
- Icons are 16px by default and inherit the current text color.
- Text should truncate only when the complete value remains available through context, title, or another view. File names may wrap in the tree.

## Components

Shared primitives live in `web/src/components/ui/`. Use shadcn as a local source-code registry, not as a separate visual system or runtime service. Review generated code before use: reduce variants to meaningful roles, map styles to this document's tokens, and enforce repository TypeScript rules.

### Buttons

Use 40px height by default, 8px radius, label weight 500–600, and 8–12px horizontal padding. Neutral buttons use secondary/raised surfaces. Accent buttons are reserved for a true primary action. Icon-only buttons require an accessible name. Active state removes perceived elevation; disabled state reduces contrast and blocks interaction.

### Forms

Inputs use primary text, muted placeholders, a visible boundary, and accent focus treatment. Labels are explicit and persistent unless an accessible hidden label accompanies a self-explanatory search field. Validation messages sit next to the relevant field and use status tokens plus text.

### Cards

Cards are exceptional, not the default container. Use them only for a repeated, independently actionable object. A card uses the standard border, `--radius-md`, and no shadow unless it moves above the page.

### Navigation

The file tree stays compact and visually secondary to the document. Folder disclosure uses native semantics. Hover changes surface and text contrast. The selected file uses a full-width rectangular row with square corners, accent text, subtle accent fill, a 2px leading indicator, weight 600, and `aria-current="page"`.

The sidebar width is keyboard- and pointer-adjustable on desktop and persists locally. Double-clicking its separator restores the default width. On mobile the separator disappears and the tree/document split remains fixed.

### Document tabs

Visited documents remain in a 28px tab bar. Tabs show file names and expose full paths as titles. The active tab shares the document surface and has a 2px accent edge. Closing the active tab selects its nearest neighbour; closing the last returns to the empty state. The URL remains authoritative, preserving deep links and browser history.

### Status line

The 24px monospace status line reports readiness, active path, file size, workspace file count, and supported process metrics. It removes path, size, and metric labels at the mobile breakpoint before values. Metrics polling pauses while the page is hidden.

### Theme control

Use a shadcn-style icon trigger in the header next to Search. Its dropdown presents system, base, and named editor themes as a single-choice group and marks the configured choice. The trigger reflects resolved polarity and has an accessible label naming the configured theme. The menu and trigger use shared control, surface, border, focus, and typography tokens.

### Overlays and modals

Use native dialog behavior, `--color-surface-raised`, `--radius-lg`, `--shadow-overlay`, and `--color-overlay`. Provide Escape and backdrop dismissal when safe. Focus enters the primary field and remains keyboard-operable.

### Status indicators

Success, warning, and error states use the corresponding semantic tokens. Pair color with plain language and appropriate ARIA semantics. Status treatments should not change layout unexpectedly.

### Rendered Markdown

Rendered content shares one prose system for headings, paragraphs, lists, quotes, code, tables, links, images, and diagrams. Do not style individual documents. Code and dense tables may scroll horizontally; prose must wrap. Mermaid controls follow the same control tokens as the application.

Fenced code uses one floating control in the top-right corner. It shows a recognized language by default, replaces it with a copy icon when the block is hovered or the control receives keyboard focus, and uses a check or error icon for brief copy feedback. Unknown and unlabelled fences show the copy icon directly. The control overlays the block without adding a header row. Mermaid blocks keep their diagram-specific controls.

## Interaction states

- Hover: increase contrast or change to the nearest surface role; never move layout.
- Focus: use a 2px `--color-accent` outline with 2px offset. Focus must remain visible in both themes.
- Active/pressed: reduce elevation and retain clear affordance.
- Selected: use accent text plus `--color-accent-subtle`; never rely on color alone.
- Disabled: remove pointer interaction, reduce prominence, and keep labels readable.
- Loading: preserve component dimensions and use concise status text. Avoid indefinite decorative animation.
- Error: use destructive text and subtle surface with `role="alert"` for blocking failures.
- Motion: keep state transitions near 120ms and honor `prefers-reduced-motion`.

## Responsive design

At 48rem and below, replace columns with a fixed viewport split: header, tree at 38%, then the tabbed document workspace at 62%. The sidebar resizer disappears.

- Preserve file-tree order before document content.
- Keep compact tree rows at least 40px.
- Keep 16px mobile gutters and at least 40px controls.
- Collapse secondary button labels and keyboard hints before hiding primary actions.
- Let wide code, tables, and diagrams scroll inside their own bounds.
- Do not scale body text down for mobile.
- Reorder content only when the task sequence remains clear to keyboard and screen-reader users.
- If the file tree later becomes a drawer, preserve its label, focus management, selected-file context, and a persistent way to reopen it.

## Accessibility

- Meet WCAG AA contrast: 4.5:1 for normal text and 3:1 for large text and meaningful control boundaries.
- All navigation, disclosure, search, dialog, and diagram controls must work by keyboard.
- Never remove focus indication without an equal or stronger replacement.
- Keep body copy at least `--text-base` and avoid long prose wider than `--reading-width`.
- Every input has an associated visible or visually hidden label.
- Icon-only controls have accessible names; decorative icons are hidden from assistive technology.
- Target 44px for isolated touch controls; 40px is the minimum for dense repeated navigation.
- Communicate selection and status through text, shape, icon, or position as well as color.
- Preserve reduced-motion preferences and logical DOM order.

## Rules for future UI work

Before introducing a visual style, ask:

1. Does a semantic token already express this meaning?
2. Does a shared component or pattern already solve it?
3. Is the difference recurring and meaningful, or merely incidental?
4. Does it preserve the tree-and-document hierarchy?
5. Does it work in light and dark modes?
6. Does it remain coherent on mobile and by keyboard?
7. Does it follow this document?

> Do not introduce a new color, font size, spacing value, radius, shadow, or component variant unless the existing system cannot express the required design meaning.

When an addition is truly necessary, define its semantic role first, add it at the shared token or component layer, verify both themes and the 48rem breakpoint, and update this document in the same change.
