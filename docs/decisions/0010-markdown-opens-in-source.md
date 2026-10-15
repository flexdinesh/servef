# 0010: Markdown opens in Source

- Status: Accepted
- Date: 2026-10-15
- Supersedes: the Markdown default view in [0003](0003-view-modes-and-on-demand-format.md)

## Context

Every text family opened in Source and offered an alternate view second
(Format for JSON/YAML). Markdown alone opened in Preview with Source second,
which made the toolbar inconsistent across text files.

## Decision

- Markdown declares `defaultMode: "source"`; Preview stays available on demand.
- The toolbar lists the default view first: Markdown shows `[Source] [Preview]`,
  formattable files `[Source] [Format]`.
- Images, including SVG, keep `defaultMode: "preview"` and `[Preview] [Source]`.
- Choosing Preview sets a preference: documents without a chosen view open in
  Preview when they offer it. Choosing Source clears it; Format leaves it
  unchanged. A line anchor still opens Source.
- The preference lives in `sessionStorage`, keyed by project path:
  it survives reloads, is separate per project, and ends with the browser tab.

## Consequences

- Reading flows stay in Preview: after choosing Preview, Markdown links, tree
  clicks, and tabs open other Markdown in Preview.
- Per-tab view choices stay in memory and reset on reload; the preference
  decides the view after a reload. Blocked storage keeps an in-memory
  preference for the page's lifetime.
