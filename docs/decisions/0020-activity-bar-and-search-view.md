# 0020: Activity bar and Search view

- Status: Accepted
- Date: 2026-10-18

## Context

Content search opened a palette-style dialog that ranked the top ten files by
fuzzy-prefix full-text score with one snippet each. It hid the workspace while
open, could not show every match, and Markdown was indexed as rendered text,
so results had no source lines to jump to. servef follows VS Code conventions,
where search lives in a sidebar view beside the Explorer.

## Decision

- A thin activity bar sits at the far left: Explorer (Cmd/Ctrl+Shift+E), then
  Search (Cmd/Ctrl+Shift+F). Clicking the active item, or Cmd/Ctrl+B, hides
  the sidebar on desktop. The view and visibility persist per browser under
  `servef-sidebar`. Mobile shows the bar inside the existing drawer.
- Find in files (footer, shortcut) reveals the Search view instead of a
  dialog, seeding the query from a single-line document selection.
- Search follows VS Code: literal, case-insensitive matching of every
  occurrence on every line; Match Case, Match Whole Word, and Use Regular
  Expression toggles (Alt+C/W/R); “files to include/exclude” path globs behind
  Toggle Search Details. Results group by file, sorted by path, with a count,
  then one row per match with a trimmed line preview. Click and Space open the
  match at `#L<line>` keeping focus in the results; Enter and double-click
  move focus to the document; F4/Shift+F4 step through matches. Refresh
  re-reads documents; Clear Search Results and Collapse/Expand All follow
  VS Code.
- Matching runs in the existing worker over documents loaded once per
  project. Results stop at 2,000 matches with a notice. Regular expressions
  match within one line.
- `/api/search-documents` returns original file text for every type. The
  `document.Handler.SearchText` hook is removed: search needs source lines,
  and every handler would return source. Binary, size, gitignore, and
  `--filename` exclusions are unchanged.

## Consequences

- Markdown search matches markup and link targets, as in VS Code.
- Include/exclude globs are path globs (`**`, `{a,b}`, a bare name matches at
  any depth and covers a folder); the explorer filename filter keeps base-name
  `--filename` semantics.
- The MiniSearch dependency is gone; ranking is replaced by path order.
- The command palette, project picker, and navigation results keep the
  palette frame.
