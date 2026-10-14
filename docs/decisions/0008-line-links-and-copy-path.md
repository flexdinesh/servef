# 0008: Line links and copy path

- Status: Accepted
- Date: 2026-10-15

## Context

Users hand files and line ranges to editors, reviews, and agents. servef
showed line numbers but could not select, link, or copy them, and the footer
path could not be copied.

## Decision

- Selection follows GitHub and VS Code: click a line number to select it,
  drag across line numbers or Shift-click to select a range, Escape to clear.
  A drag previews locally and writes the URL once, on release; touch selects
  on tap so the page still scrolls.
- The URL fragment owns the selection: `#L12` or `#L12-L20` (`#L12-20` also
  parses). Changes replace history; the route's loader deps ignore the hash, so
  nothing is refetched. The default-project redirect keeps the fragment.
- Copied references are project-relative, editor-style: `path`, `path:12`,
  `path:12-20`. A floating gutter control copies lines; the footer copies the
  path.
- Line selection exists in Source only. A line anchor opens Source when the
  tab has not chosen a view; choosing Preview or Format clears it, because
  formatted line numbers do not address the file.
- Parsing, clamping, range extension, and formatting live in the pure
  `web/src/line-selection.ts`.

## Consequences

- Line anchors are a client-side contract; the server is unchanged.
- Out-of-range anchors clamp to the last line or select nothing.
- Absolute paths are not offered; add them deliberately if needed.
