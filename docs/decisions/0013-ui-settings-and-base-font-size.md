# 0013: UI settings and base font size

- Status: Accepted
- Date: 2026-10-15

## Context

Source (13.5px mono) and Markdown preview (15px) read larger than the 13px
explorer, unlike VS Code, and sizes mixed px and rem, so nothing could scale
from one value. Users want to set the interface size.

## Decision

- Settings are browser-only UI preferences: a dialog opened from a status-bar
  gear or Cmd/Ctrl+, styled after VS Code settings (search, grouped items,
  modified marker, per-item reset, Reset all). Changes apply immediately.
- Persist only values that differ from defaults, as JSON under
  `servef-settings` in `localStorage`; Reset all removes the key. Invalid or
  unreadable stored values fall back to defaults per field.
- Theme is a setting: it moves from its footer menu into Settings (a select)
  but keeps its `servef-theme` key, so existing choices survive. Reset all
  restores System.
- The first setting is Font Size: whole pixels, 10–20, default 13. It sets
  `--font-size-base`; `html` maps it to `1rem = base × 16 / 13`, so the scale
  authored for a 16px rem is unchanged at the default.
- All UI sizes are rem. Explorer, tabs, and source equal the base; prose is
  14/13 of it (15/13 on mobile); the status bar is 12/13. Pierre's row height
  is px, so the explorer scales it and remounts, keeping expansion and search.
- Settings are defined as data (`settingDefinitions`) so new ones add a
  definition, a field, and a parser.

## Consequences

- Source and explorer text match; prose sits one step larger; status stays
  smaller. Browser tests assert the new default sizes.
- Spacing and control heights scale with the font; the sidebar width stays
  in stored px. Media-query breakpoints use the browser default rem, so layout
  modes do not shift with the setting.
