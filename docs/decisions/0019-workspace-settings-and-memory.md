# 0019: Settings navigation and project explorer memory

- Status: Accepted
- Date: 2026-10-16
- Extends: [0009](0009-multiple-roots-and-worktree-picker.md), [0013](0013-ui-settings-and-base-font-size.md)

## Context

Settings need the same compact editor conventions as the workspace. Tabs
already persist per project, but folder expansion disappears after reloads
and project switches.

## Decision

- Keep Settings as a live browser-preferences dialog. Add category navigation,
  result counts, a modified-only filter, and an explicit clear-search action.
  Keep its header and footer visible within the viewport. Mobile categories
  become a horizontal row.
- Extend `servef-workspaces-v2` with optional folder expansion per absolute
  project path. Missing expansion preserves the existing default; an empty
  array means fully collapsed. Validate remembered folders against the fresh
  tree. Preserve folders hidden by filename filtering. Search and filters stay
  transient, as in 0006 and 0007.
- Restore tabs before writing workspace memory; URL/history still select the
  active document. Preserve independent tabs and expansion when updating either.
- Tab context menus provide Close, Close Others, Close to the Right, and Close
  All. Right-click preserves the active document. Closing the active tab selects
  the nearest surviving neighbor; closing all clears the document URL. Menu
  actions support keyboard access and return focus to a surviving tab or pane.

## Consequences

- Existing browser memory remains compatible. Deleted files and folders do not
  reappear from storage. Storage failure leaves the current session usable.
- Project switching and reloads restore the same tabs and folder expansion.
  Document modes remain session-only.
