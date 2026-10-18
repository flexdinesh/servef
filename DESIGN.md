---
name: servef
description: Compact VS Code-style workspace for browsing local files.
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
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.6
  body-mobile:
    fontFamily: "\"Inter Variable\", Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.9375rem"
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
    fontSize: "0.8125rem"
  source:
    fontFamily: "ui-monospace, \"SFMono-Regular\", Consolas, \"Liberation Mono\", monospace"
    fontSize: "0.8125rem"
    lineHeight: "1.25rem"
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

servef follows familiar editor conventions: an activity bar, a compact explorer and search sidebar, document tabs, a readable preview, and a quiet status footer. Neutral surfaces, flat rows, subtle boundaries, and restrained controls keep local files central.

Workspace identity sits above the explorer as the project picker trigger. App identity, content search, process state, and the Settings gear sit in the footer; Ctrl/Cmd+Shift+P opens the command palette. There is no top app header or logo. Mobile keeps the document visible and opens files in a drawer.

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

Choices: Light, Dark, Catppuccin Mocha, Gruvbox Dark, Nord, Solarized Light, System. Light and Solarized Light resolve light; other named palettes and Dark resolve dark. System follows `prefers-color-scheme`. Theme is chosen in Settings (or via the command palette) and persists under `servef-theme`. Keep `color-scheme`, Mermaid, and Excalidraw synchronized with resolved polarity.

**The Functional Accent Rule.** Use accent for state and interaction; keep routine prose headings and large surfaces neutral.

## Typography

Inter Variable is bundled locally for UI and prose; code/source use the system monospace stack. No runtime font service.

One base size drives the whole scale: the Font Size setting (default 13px, 10–20px) sets `--font-size-base`, and `html` sets `1rem = base × 16 / 13`. Every size, spacing, and control height is in rem, so the interface scales together; Pierre's px row height is scaled in JS. Sizes below are at the 13px default.

| Role | Effective size / line-height | Treatment |
| --- | --- | --- |
| Preview H1 / H2 / H3 / H4 | 26 / 21 / 17 / 15px; 1.25 | 600 weight, -0.02em tracking |
| Preview prose | 14px / 1.6 desktop; 15px / 1.6 mobile | 400 weight, 75ch maximum |
| Explorer / tabs / result names | 13px (= base) | Compact labels |
| Supporting UI / search input / tables | 14px / 20px | Shared small text role |
| Footer / metadata | 12px | Sans; footer line-height 1 |
| Source | 13px / 22px (= base) | Monospace, fixed line rows |
| Preview code | 13px | Monospace; block line-height 1.6 |

Source matches the explorer so code never looks oversized beside the tree; prose sits one step above. Project name uses 12px and 600 weight. The generic Tailwind scale serves primitives; preview uses `--preview-*` roles.

## Layout

Desktop: a 2.5rem activity bar, then the full-height sidebar (Explorer or Search) beside the tabbed document workspace, above a 28px footer split at the sidebar boundary. Its two sections track the activity bar plus resizable sidebar, and the document column; with the sidebar hidden the footer's left section takes its natural width. No app header. The sidebar defaults to 18rem, resizes within 13–30rem, persists locally, and resets on separator double-click. The 1px separator has a wider pointer hit area and supports arrow keys. Explorer and document scroll independently.

Tabs and explorer header are 32px tall; Pierre rows are 24px. Preview uses a 32px inset, left alignment, and 75ch maximum. Article, errors, and empty-state containers share the measure; the empty state centers horizontally and vertically within the document pane. Source mode removes preview padding.

At `max-width: 48rem`, document fills the workspace. The sidebar opens in a native modal drawer, `min(85vw, 22rem)` wide, with the activity bar (44px items) at its left edge, toolbar trigger (a project picker trigger sits beside it), close control, backdrop, and 40px rows. Tabs, explorer header, and footer become 44px tall. Prose becomes 16px with 24px vertical / 16px horizontal insets. Footer keeps a compact app section and flexible document section; filename truncates while Find in files and the Settings gear remain visible. Size/count, type label, image details, process details, and shortcut hints hide.

Content palette (command palette, project picker, navigation results): maximum 38rem wide / 40rem tall, 48px from top, transparent backdrop. Mobile uses 8px outer gutters. File/path search stays in the explorer; content search is the Search view.

Spacing scale: 4, 8, 12, 16, 24, 32, 48, 64px. Prose blocks use 12px margins; H1/H2 start with 32px above, H3/H4 with 24px. Wide code, tables, and diagrams scroll within their bounds.

## Elevation & Depth

Workspace regions are flat. Surface contrast, 1px separators, and the active tab's 2px top accent establish structure. Shadows serve floating controls and overlays.

- `--shadow-control`: `0 1px 2px rgb(15 18 23 / 8%)` in Light.
- `--shadow-overlay`: `0 1.5rem 4rem rgb(15 18 23 / 22%)` in Light; palettes, theme menus, process panels.
- Dark and named themes override shadows in CSS. Drawer uses its boundary and backdrop.

Motion is functional: progress uses 120ms opacity / 800ms transform; theme menus use 100ms transitions. Honor reduced motion.

## Shapes

Explorer rows, tabs, and footer actions are square rectangles. Small controls, inline code, and search results use 2px radius; generic buttons, code blocks, diagrams, palette, and theme menu use 4px. The 6px token remains available to shared primitives.

Icons: 16px generic, 14px footer/search/filter, 12px tabs; inherit text color. Tabs show the explorer's Pierre colored file icon, mapped to theme tokens by hue family (unmapped icons inherit; unresolved files fall back to a generic file icon). Focus defaults to 2px accent outline with 2px offset, or an equally visible component boundary.

## Components

### Buttons

Shared `Button`: primary, outline, ghost, destructive, link. Default 40px height, 4px radius, 14px medium label, 12px horizontal padding; compact sizes 32px. Hover changes color/surface without layout shift. Disabled controls suppress interaction and reduce opacity. Icon-only actions require accessible names. Footer overrides use square transparent 28px controls, 44px on mobile.

### Activity bar

A 2.5rem column on `surface-secondary` with a 1px right border: Explorer (files icon, Cmd/Ctrl+Shift+E), then Search (Cmd/Ctrl+Shift+F). Items are square, 20px icons in `text-muted`; hover and the active item use `text-primary`, and the active item carries a 2px accent bar on its left edge with `aria-pressed`. Clicking the active item, or Cmd/Ctrl+B, hides the sidebar on desktop (the resizer hides with it); clicking any item restores it. Both views stay mounted so switching keeps tree and search state. View and visibility persist under `servef-sidebar`. Mobile puts the bar inside the drawer with 44px items; the drawer is named after the active view.

### Explorer

Pierre compact tree: complete built-in colored file icons, native folder expansion chevrons, square rows, semantic token overrides, 24px desktop / 40px mobile. The project name sits above as a trigger for the project picker. Selection uses accent text and subtle fill. Preserve editor keyboard selection, expansion, and Ctrl/Cmd-click semantics. File/path search starts hidden; the search icon toggles it, Ctrl/Cmd+K opens it (Ctrl/Cmd+P alias), and Escape hides it. Mobile opens the drawer first. Preserve expansion/search across the breakpoint.

The filename filter sits below the explorer header, always visible, matching the
filename search field's width and appearance: funnel icon, placeholder
“Filter: *.md, *.go”, and, when active, a muted 12px “visible / total” count in a
polite live region plus a clear action. It accepts comma-separated,
case-insensitive base-name globs with the CLI's `--filename` semantics (`*`, `?`,
`[...]`); empty shows all. Input applies after 150ms. An invalid glob marks the
field `aria-invalid`, adds a destructive bottom boundary and “Invalid pattern”
helper, and keeps the last valid filter. No matches show “No files match …” with
a Clear filter action. When the CLI narrows `--filename`, a muted “Serving …” line
sits under the field. Escape clears a value, otherwise blurs; Enter or ArrowDown
focuses the first visible row. The filter is not persisted and changes tree
visibility only: tabs, the active document, and expansion are preserved.
Filename search intersects the filter; content search keeps its session-wide CLI
scope.

Folder expansion is remembered per absolute project path in browser storage, including a fully collapsed tree. Restore only folders present in the fresh tree. Search and filename filters remain transient; changing them does not overwrite hidden folders' expansion.

Beside Search files, a compact action toggles Collapse all/Expand all, including
folders hidden by the filename filter. It is disabled during a nonempty filename search,
which expands matching ancestors. Directory chevrons are 12px within existing row targets.

### Project picker

The explorer header shows the current project's name with a 12px chevron; it
opens the picker, as does Ctrl/Cmd+O. Mobile also shows a project icon button
in the toolbar after the files toggle, so switching skips the drawer. The
picker is a quick pick on the content-palette frame with two modes:

- **Projects**: the served folder first, then discovered repositories; linked
  worktrees nest under their main worktree. Worktrees outside the served root
  stay listed but `aria-disabled`, with a muted “Outside <root>” note. Folders
  opened by hand follow under “Opened folders”, each with a hover Forget action
  (always visible on mobile) except the current one. The current project shows
  an accent “Current” check. Typing filters to ranked flat matches; `/` or
  **Open folder…** enters Browse.
- **Browse**: the served root is a muted fixed prefix before the path input.
  The first row opens the typed folder; child folders follow, repositories with
  a muted `git` label. Tab completes, Enter on a folder descends, Backspace on a
  trailing slash goes up a level and on an empty path returns to Projects.

Rows are 32px desktop / 44px mobile with 14px icons (folder, repository,
worktree, opened folder); paths truncate from the start so the deepest folders
stay visible. Status lines read “Scanning for projects…” and “Reading folder…”
after 150ms, name list or folder errors with Retry, and count unreadable
folders. Escape or the backdrop closes the picker and restores focus. Choosing
a project keeps the current workspace visible until the new project loads, then
starts fresh: tree filter, search index, and format results reset, and the
project's remembered tabs return. A project that no longer resolves shows
“The project <id> is unavailable…” with a Choose another project action.

### Tabs and view mode

Filenames truncate at 12.5rem maximum tab width; titles expose paths. Active tab shares document surface with 2px top accent. Closing active selects nearest neighbour. Right-click or Shift+F10 on a tab opens a themed context menu: Close, Close Others, Close to the Right, and Close All. Unavailable actions are disabled. Right-click preserves the active document; dismissal returns focus to the tab, and closing returns focus to a surviving tab or the document pane. The Document view group follows declared capabilities: Markdown and SVG show [Preview][Source]; formattable types (JSON, YAML) show [Source][Format]; everything else shows no toggle. Buttons are square segments that fill the toolbar height (32px desktop, 44px mobile) with 12px labels and 12px horizontal padding; the pressed one uses `aria-pressed` with accent text on an `accent-subtle` fill spanning the whole segment. View labels/defaults come from file-type metadata. Open tabs and the active document are remembered per project in browser storage and restored against the fresh tree; mode is remembered per open tab and resets on reload; URL/history remain authoritative for the document. Preview failures explain the problem and preserve access to Source.

Format runs on demand: Source stays visible until the result arrives (no blank flash), Format is `aria-busy`, and the footer shows Loading after 150ms. Results are cached per file for the session. Formatted output shares Source's full-width, line-numbered viewer. A failure keeps Source, un-presses Format, and shows a slim destructive notice, “Couldn't format: <reason>.”, above the source; pressing Format retries.

### Line selection

Source works like GitHub and VS Code line links. Clicking a line number selects that line; dragging across line numbers (or on into the code) previews a range and commits it on release; Shift-click extends from the last pressed line in either direction; Escape in the source clears. Touch selects on tap only, keeping scroll. The URL fragment owns the selection (`#L12`, `#L12-L20`, replace history, no refetch), so loading an anchor highlights the range, clamps it to the file, and scrolls its first line to the upper third unless visible. A line anchor opens Markdown/SVG in Source; switching to Preview or Format clears it, since line numbers address the original file only. Selected rows use `accent-subtle` with accent, 600-weight line numbers. A 19px raised copy control (2px radius, strong border, control shadow) sits in the gutter's 22px left inset on the first selected line and copies the project-relative `path:12` or `path:12-20`; “Copied” appears beside the gutter.

Unregistered text files open as Source and the footer names the type (“Plain text”). Files over 4 MiB show a slim warning-subtle bar above Source, “Large file (6.2 MB): preview and formatting are off.”, adding “Not included in search.” past 8 MiB; Preview/Format stay visible but `aria-disabled` with the reason as a tooltip.

### Images and binary files

Images open in a fit-by-default viewer: centered, never upscaled, on a subtle surface/surface-secondary checkerboard with a 1px border; zoomed images scroll within the pane. The toggle slot holds [Fit][−][100%][+] (SVG adds [Preview][Source] first; zoom hides in Source). The middle button shows the current zoom and resets to 100%. Steps are ×1.25 within 10%–800%. With the viewer focused, `+`/`-` zoom, `0` fits, `1` shows actual size. Zoom changes are announced politely; the footer shows `1920 × 1080 · 64%` beside the size. Loading shows a neutral placeholder; decode failures read “Can't display this image.” Alt text is the filename.

Other binaries open to the centered empty-state layout: “Binary file”, `name · size`, and “servef shows text and images only.” No toggle.

Non-image files over 32 MiB use the same layout: “Too large to display”, `name · size`, and “servef shows text files up to 32 MB.” No toggle; the footer keeps the type label.

### Footer

Explorer footer: servef home, process state/details, file count. Document footer: full relative file path first, a Copy path icon action (14px; checks in success color for 1.6s, announced politely), Find in files, then selected lines (`Ln 12` / `Ln 12–20`, Source only), image dimensions/zoom (images only), size, file-type label, and a Settings gear (14px icon, square 28px target, 44px on mobile). Mobile keeps Copy path and hides selected lines. File path truncates and exposes its full value in a title. Actions use flat hover fills. Process state pairs color with Ready/Loading/Error; details open above.

LSP availability sits beside the document language for every text file, including
unsupported languages: LSP unavailable, missing, off, available, starting, ready,
failed, or unknown. Its details panel opens above the footer and names the
server, file restrictions, negotiated methods, memory, idle timeout, and errors.
Missing servers show host-specific install recipes with prerequisites, copyable
commands or manual installation links, documentation, Check again, and Settings.
Status checks never start a server. Mobile keeps the LSP status visible; its
panel fits the viewport with 12px side insets and 44px action/copy targets.

### Settings

The footer gear or Ctrl/Cmd+, opens Settings, a VS Code-style dialog (56rem maximum width, centered and bounded by the viewport, transparent backdrop so changes show live). Its header, search, filters, and footer stay visible while settings scroll independently. A compact left category rail filters All settings, Appearance, or Code Navigation, with counts; mobile moves categories into a horizontally scrollable row with 44px targets. A 32px header names it and holds Close. A bordered “Search settings” field (focused on open; accent ring) filters by every term across category, title, and description; Escape clears a query from any setting control, then closes. A clear-search action returns focus to search. A result count and Show modified toggle sit below search; search matches globally across categories, and Clear filters recovers empty results. Settings group under 16px section titles (Appearance). Each item reads “Category: **Title**”, a 13px secondary description, then its control. Flat items take a surface-secondary fill on hover/focus. A changed value shows a 2px accent bar in the item's left gutter (VS Code's modified marker), an italic muted “Modified”, and a reset icon (restores the default; focus moves to the control). Settings, in order: Theme (an 18rem maximum-width native select of the named themes, default System), Font Size, and Code Navigation: Language Servers (On by default; Off disables code navigation in this browser). Number controls are 5rem fields with a muted unit; values apply on every valid keystroke; invalid input keeps the last valid value, marks the field `aria-invalid`, and says “Enter a whole number from 10 to 20.” Selects apply on change. The footer reads “Saved in this browser. Changes apply immediately.” with an outline Reset all, disabled when nothing differs; it restores System, 13px, and language servers On. Only changed values persist, under `servef-settings` in localStorage; theme keeps `servef-theme`. Search also matches select option labels (“nord” finds Theme).

Opened at a setting (from the command palette), Settings clears its search, scrolls the item into view, focuses its control (selecting a number's text), and flashes the item: `accent-subtle` fill and a 1px inset accent ring hold for the first third of 1.6s, then ease out (`cubic-bezier(0.16, 1, 0.3, 1)`). Reduced motion shows the highlight statically for 1.6s.

### Command palette

Ctrl/Cmd+Shift+P or F1 opens a VS Code-style command palette on the content-palette frame (38rem, 12vh from top, sized to its rows). The field shows a muted `>` prompt, “Type a command”, and an Esc hint. Rows are 32px (44px mobile), 14px: secondary category, a 12px chevron, then the primary title, e.g. “Settings › Font Size”, with the current value right-aligned in muted 12px (“13px”, “System”). Typed terms must each match the category or title; matches render in accent, 600 weight. Commands with a title word starting with the first term rank first. The first row is active; arrows wrap, hover activates, Enter or click runs, Escape or the backdrop closes and restores focus. No matches reads “No matching commands”. Each option is named “Category: Title, value”. First is “Preferences › Open Settings”, showing its shortcut (Ctrl+, / ⌘,) in place of a value; it opens Settings with search focused. Every setting follows as a command that opens Settings at that setting.

### Content search

Ctrl/Cmd+Shift+F or Find in files reveals the Search view and focuses its field, seeding it from a single-line document selection. The 32px header reads “Search” (12px, 600) with Refresh, Clear Search Results, and Collapse All/Expand All icon actions (Close on mobile). The field matches the filename filter (1.75rem, 1px border, 2px radius, accent focus ring; 40px mobile), placeholder “Search”, with inline 22px toggles: Match Case, Match Whole Word, Use Regular Expression (Alt+C/W/R); pressed toggles use accent text, accent border, and `accent-subtle` fill. A right-aligned “…” Toggle Search Details reveals “files to include” and “files to exclude” (12px labels, same field style, placeholders “e.g. *.ts, src/**/include”). Invalid regexes or globs mark the field `aria-invalid` with a destructive bottom boundary and a 12px destructive message.

A 2px accent progress bar runs after 150ms while searching; previous results stay until new ones arrive. A polite 12px message reads “3 results in 2 files” (“2,000+ results” when truncated), then any notice: “No results found[ in '…' excluding '…'].”, unreadable-file warnings, the truncation advice, or “Search is unavailable.” with Retry search.

Results are a `role="tree"` with 22px rows (40px mobile): file rows show a 12px chevron, the Pierre file icon, the name in `text-primary`, the muted 12px folder, and a right-aligned count badge (6px radius) on `border`; match rows indent 2.5rem and show a trimmed line preview whose match uses `warning-subtle`. Hover uses `surface-raised`; keyboard focus a 1px inset accent outline; the opened match `accent-subtle`. Arrows, Home/End, Left/Right (collapse, parent, expand, first child), Space (open, keep focus), Enter (open, focus document), click (open, keep focus), double-click (open, focus document), F4/Shift+F4 (next/previous match). ArrowDown from the field enters the tree; ArrowUp from the first row returns. Escape leaves for the document (closes the drawer on mobile). Opening a match navigates to `#L<line>`, which selects that source line.

### Markdown, code, diagrams

One prose system across documents. Tables: 14px, neutral headers, horizontal overflow. Quotes: 3px strong border. Code blocks: 8px vertical/12px horizontal padding, 4px radius, neutral fill, 1px border. Floating copy control has the same 4px radius, 8px inset and language-label padding. It shows recognized language until hover/focus and gives copy feedback; no extra header row. Markdown Preview hides leading closed YAML/TOML frontmatter and HTML comments; Source retains them and code literals stay visible. Diagrams retain local assets, resolved theme polarity, and compact floating controls. Errors pair destructive text/fill with alert semantics. Cards and chips are not current workspace patterns.

## Do's and Don'ts

### Do:

- Do reuse semantic CSS tokens and shared primitives across every theme.
- Do retain 14px desktop / 15px mobile prose (at the 13px base), 75ch measure, and left alignment.
- Do size new UI in rem so it follows the Font Size setting; reserve px for 1–2px borders and offsets.
- Do keep file/path search (Explorer) separate from content search (Search view).
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
