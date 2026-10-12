# 0003: Preview for Markdown/images; Format on demand

- Status: Accepted
- Date: 2026-10-12

## Context

JSON/YAML used a `code` preview kind that opened formatted output by default.
Formatting is a transformation, not a preview, and most files need neither.

## Decision

- Preview kinds are `markdown` and `image` only. Other families show Source.
- Formatting is an optional capability: handlers implementing
  `document.Formatter` (`FormattedCodeHandler`) are reported as `formattable`.
- Formattable files open in Source; the UI offers `[Source] [Format]` and
  fetches `GET /api/format?path=` on demand (cached per path for the session).
  Format errors keep Source and show an inline notice.
- View mode is per tab and resets on reload.

## Consequences

- `/api/page` never formats eagerly. `/api/format` returns 400 when not
  formattable, 413 over the preview limit, 422 on format failure.
- Formatters must preserve meaningful values (JSON number precision/key order,
  YAML comments/styles/anchors/multiple documents).
