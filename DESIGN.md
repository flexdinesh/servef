---
name: servef
description: Compact VS Code-style workspace for local Markdown, JSON, and YAML.
colors:
  bg: "#f8f8f8"
  surface: "#ffffff"
  surface-secondary: "#f3f3f3"
  surface-raised: "#ffffff"
  text-primary: "#333333"
  text-secondary: "#515866"
  text-muted: "#6d7685"
  border: "#e5e5e5"
  border-strong: "#c5cbd4"
  accent: "#2563eb"
  accent-hover: "#1d4ed8"
  accent-subtle: "#e8efff"
  on-accent: "#ffffff"
  success: "#16794b"
  warning: "#9a5800"
  warning-subtle: "#fff1cf"
  destructive: "#bd3029"
  destructive-subtle: "#fff0ef"
  overlay: "rgb(15 18 23 / 48%)"
  syntax-comment: "#57606a"
  syntax-keyword: "#cf222e"
  syntax-type: "#0550ae"
  syntax-function: "#6639ba"
  syntax-variable: "#953800"
  syntax-string: "#0a3069"
  syntax-number: "#0550ae"
  syntax-operator: "#0550ae"
typography:
  headline:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  section:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.375rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  subsection:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  minor-heading:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  body:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.6
  body-mobile:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.6
  label:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
  supporting:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    lineHeight: "1.25rem"
  metadata:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.75rem"
    lineHeight: "1rem"
  code:
    fontFamily: "ui-monospace, \"SFMono-Regular\", Consolas, \"Liberation Mono\", monospace"
    fontSize: "13.5px"
  source:
    fontFamily: "ui-monospace, \"SFMono-Regular\", Consolas, \"Liberation Mono\", monospace"
    fontSize: "13.5px"
    lineHeight: "21px"
rounded:
  sm: "0.125rem"
  md: "0.25rem"
  lg: "0.375rem"
spacing:
  space-1: "0.25rem"
  space-2: "0.5rem"
  space-3: "0.75rem"
  space-4: "1rem"
  space-5: "1.5rem"
  space-6: "2rem"
  space-7: "3rem"
  space-8: "4rem"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.md}"
    height: "2.5rem"
    padding: "0 0.75rem"
    typography: "{typography.supporting}"
  button-outline:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    height: "2.5rem"
    padding: "0 0.75rem"
    typography: "{typography.supporting}"
  button-ghost:
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    height: "2.5rem"
    padding: "0 0.75rem"
    typography: "{typography.supporting}"
  button-destructive:
    backgroundColor: "{colors.destructive-subtle}"
    textColor: "{colors.destructive}"
    rounded: "{rounded.md}"
    height: "2.5rem"
    padding: "0 0.75rem"
    typography: "{typography.supporting}"
  button-link:
    textColor: "{colors.accent}"
    rounded: "{rounded.md}"
    height: "2.5rem"
    padding: "0 0.75rem"
    typography: "{typography.supporting}"
  footer-action:
    textColor: "{colors.text-secondary}"
    height: "1.75rem"
    padding: "0 0.5rem"
    typography: "{typography.metadata}"
  search-field:
    textColor: "{colors.text-primary}"
    backgroundColor: "{colors.surface-raised}"
    padding: "0.5rem"
  explorer-row:
    backgroundColor: "{colors.surface-secondary}"
    textColor: "{colors.text-secondary}"
    height: "24px"
    typography: "{typography.label}"
  document-tab:
    backgroundColor: "{colors.surface-secondary}"
    textColor: "{colors.text-secondary}"
    height: "2rem"
    typography: "{typography.label}"
  code-block:
    backgroundColor: "{colors.surface-secondary}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
    typography: "{typography.code}"
---
# Design System: servef

## Overview

**Creative North Star: "VS Code editor"**

servef follows familiar editor conventions: a compact explorer, document tabs, a readable preview, and a quiet status footer. Neutral surfaces, flat rows, subtle boundaries, and restrained controls keep local files central.

Workspace identity sits above the explorer. App identity, content search, process state, and theme selection sit in the footer. There is no top app header or logo. Mobile keeps the document visible and opens files in a drawer.

**Key Characteristics:**

- Compact editor navigation.
- Flat, subtle, neutral surfaces.
- Left-aligned proportional prose and monospace code.
- Shared semantic roles across named editor themes.

Implementation authority: `web/src/style.css`, `web/src/FileTree.tsx`, shared primitives in `web/src/components/ui/`, and `web/src/theme.ts`. Frontmatter captures base Light tokens and effective component roles. Theme overrides remain in CSS; the sidecar adds metadata and examples.

## Colors

Base Light uses neutral editor surfaces and functional blue. Named themes keep the same semantic roles.

- **Primary:** `accent`, `accent-hover`, `accent-subtle`, `on-accent` serve links, focus, active tabs, selected files, and primary actions.
- **Neutral:** `bg` is canvas; `surface` is document; `surface-secondary` is explorer/tabs/footer/code; `surface-raised` is menus and floating controls. `text-primary/secondary/muted` establish content, supporting labels, and metadata. `border` separates ordinary regions; `border-strong` emphasizes elevated boundaries; `overlay` is the drawer backdrop.
- **Status:** success, warning, destructive and subtle fills accompany text. `warning-subtle` also highlights search matches. `syntax-*` supplies code-token roles.

Choices: Light, Dark, Catppuccin Mocha, Gruvbox Dark, Nord, Solarized Light, System. Light and Solarized Light resolve light; other named palettes and Dark resolve dark. System follows `prefers-color-scheme`. Choice persists under `servef-theme`. Keep `color-scheme`, Mermaid, tldraw, and Excalidraw synchronized with resolved polarity.

**The Functional Accent Rule.** Use accent for state and interaction; keep routine prose headings and large surfaces neutral.

## Typography

Inter Variable is bundled locally for UI and prose; code/source use the system monospace stack. No runtime font service.

| Role | Effective size / line-height | Treatment |
| --- | --- | --- |
| Preview H1 / H2 / H3 / H4 | 28 / 22 / 18 / 16px; 1.25 | 600 weight, -0.02em tracking |
| Preview prose | 15px / 1.6 desktop; 16px / 1.6 mobile | 400 weight, 75ch maximum |
| Explorer / tabs / result names | 13px | Compact labels |
| Supporting UI / search input / tables | 14px / 20px | Shared small text role |
| Footer / metadata | 12px | Sans; footer line-height 1 |
| Source | 13.5px / 21px | Monospace, fixed line rows |
| Preview code | 13.5px | Monospace; block line-height 1.6 |

Workspace name uses 12px and 600 weight. The generic Tailwind scale serves primitives; preview uses `--preview-*` roles.

## Layout

Desktop: full-height explorer beside the tabbed document workspace, above a 28px footer split at the sidebar boundary. Its two sections track the resizable explorer and document columns. No app header. Explorer defaults to 18rem, resizes within 13–30rem, persists locally, and resets on separator double-click. The 1px separator has a wider pointer hit area and supports arrow keys. Explorer and document scroll independently.

Tabs and explorer header are 32px tall; Pierre rows are 24px. Preview uses a 32px inset, left alignment, and 75ch maximum. Article, errors, and empty-state containers share the measure; the empty state centers horizontally and vertically within the document pane. Source mode removes preview padding.

At `max-width: 48rem`, document fills the workspace. Files open in a native modal drawer, `min(85vw, 22rem)` wide, with toolbar trigger, close control, backdrop, and 40px rows. Tabs, explorer header, and footer become 44px tall. Prose becomes 16px with 24px vertical / 16px horizontal insets. Footer keeps a compact app section and flexible document section; filename truncates while Find in files and theme remain visible. Size/count, process details, and shortcut hints hide.

Content palette: maximum 38rem wide / 40rem tall, 48px from top, transparent backdrop. Mobile uses 8px outer gutters. File/path search stays in the explorer.

Spacing scale: 4, 8, 12, 16, 24, 32, 48, 64px. Prose blocks use 12px margins; H1/H2 start with 32px above, H3/H4 with 24px. Wide code, tables, and diagrams scroll within their bounds.

## Elevation & Depth

Workspace regions are flat. Surface contrast, 1px separators, and the active tab's 2px top accent establish structure. Shadows serve floating controls and overlays.

- `--shadow-control`: `0 1px 2px rgb(15 18 23 / 8%)` in Light.
- `--shadow-overlay`: `0 1.5rem 4rem rgb(15 18 23 / 22%)` in Light; palettes, theme menus, process panels.
- Dark and named themes override shadows in CSS. Drawer uses its boundary and backdrop.

Motion is functional: progress uses 120ms opacity / 800ms transform; theme menus use 100ms transitions. Honor reduced motion.

## Shapes

Explorer rows, tabs, and footer actions are square rectangles. Small controls, inline code, and search results use 2px radius; generic buttons, code blocks, diagrams, palette, and theme menu use 4px. The 6px token remains available to shared primitives.

Icons: 16px generic, 14px footer/search, 12px tabs; inherit text color. Focus defaults to 2px accent outline with 2px offset, or an equally visible component boundary.

## Components

### Buttons

Shared `Button`: primary, outline, ghost, destructive, link. Default 40px height, 4px radius, 14px medium label, 12px horizontal padding; compact sizes 32px. Hover changes color/surface without layout shift. Disabled controls suppress interaction and reduce opacity. Icon-only actions require accessible names. Footer overrides use square transparent 28px controls, 44px on mobile.

### Explorer

Pierre compact tree: complete built-in colored file icons, native folder expansion chevrons, square rows, semantic token overrides, 24px desktop / 40px mobile. Workspace name sits above. Selection uses accent text and subtle fill. Preserve editor keyboard selection, expansion, and Ctrl/Cmd-click semantics. File/path search starts hidden; the search icon toggles it, Ctrl/Cmd+K opens it (Ctrl/Cmd+P alias), and Escape hides it. Mobile opens the drawer first. Preserve expansion/search across the breakpoint.

The type picker sits below the explorer header, matches the filename search field's
width and appearance, and opens a checkbox menu with counts and a Show all types
action. No selection shows all types; selecting one or more limits visibility to
those families. It filters tree visibility without closing tabs or changing
the active document. A single CLI-allowed type disables selection and shows
“Set by CLI.” Filename search intersects the selected type; content search keeps
its session-wide CLI scope.

Beside Search files, a compact action toggles Collapse all/Expand all, including
folders hidden by type filters. It is disabled during a nonempty filename search,
which expands matching ancestors. Directory chevrons are 12px within existing row targets.

### Tabs and view mode

Filenames truncate at 12.5rem maximum tab width; titles expose paths. Active tab shares document surface with 2px top accent. Closing active selects nearest neighbour. Preview/Source (Markdown) and Formatted/Source (JSON/YAML) use `aria-pressed`, subtle active fill, and 12px labels. Buttons fill the toolbar height with 4px above and below: 24px desktop, 36px mobile. URL/history remain authoritative. View labels/defaults come from file-type metadata; source-only types hide the toggle. Formatted code shares Source's full-width, line-numbered viewer, displaying the handler's formatted output. Preview failures explain the problem and preserve access to Source.

### Footer and theme

Explorer footer: servef home, process state/details, file count. Document footer: full relative file path first, Find in files immediately after, then size and theme. File path truncates and exposes its full value in a title. Actions use flat hover fills. Process state pairs color with Ready/Loading/Error; details open above. Theme opens upward as a single-choice menu, marks configured choice, and shows its label plus resolved light/dark icon.

### Content search

Ctrl/Cmd+Shift+F or footer opens a separate dialog. Field: 14px text, accent bottom boundary on focus. Results: 13px names, 12px paths/snippets, 2px radius, hover surface, accent-subtle active fill; warning-subtle matches. Preserve focus, arrow navigation, Enter, Escape, and loading/error feedback.

### Markdown, code, diagrams

One prose system across documents. Tables: 14px, neutral headers, horizontal overflow. Quotes: 3px strong border. Code blocks: 8px vertical/12px horizontal padding, 4px radius, neutral fill, 1px border. Floating copy control has the same 4px radius, 8px inset and language-label padding. It shows recognized language until hover/focus and gives copy feedback; no extra header row. Markdown Preview hides leading closed YAML/TOML frontmatter and HTML comments; Source retains them and code literals stay visible. Diagrams retain local assets, resolved theme polarity, and compact floating controls. Errors pair destructive text/fill with alert semantics. Cards and chips are not current workspace patterns.

## Do's and Don'ts

### Do:

- Do reuse semantic CSS tokens and shared primitives across every theme.
- Do retain 15px desktop / 16px mobile prose, 75ch measure, and left alignment.
- Do keep file/path search separate from content search.
- Do preserve keyboard focus, accessible labels, modal behavior, and reduced motion.
- Do keep fonts, icons, and runtime assets local to the embedded Go distribution.
- Do review shadcn-generated code against this system and strict TypeScript: no `any`, non-null assertions, or type assertions.

### Don't:

- Don't add a top app header or logo; keep workspace identity above the explorer and app identity in the footer.
- Don't restore the mobile split; use the files drawer with 40px rows and 44px toolbar/footer.
- Don't center preview prose or substitute generic heading/body tokens for the preview scale.
- Don't introduce decorative gradients, pill containers, heavy shadows, or cards around document sections.
- Don't rely on color alone for selection, focus, errors, or status.
- Don't require a CDN, frontend sidecar, or system JavaScript runtime in production; build and embed assets, never edit `internal/web/dist/` directly.
