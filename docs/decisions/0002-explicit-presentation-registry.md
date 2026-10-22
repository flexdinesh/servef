# 0002: Explicit registry decides presentation only

- Status: Accepted; Chroma highlighting superseded by [0029](0029-highlights-syntax-highlighting.md)
- Date: 2026-10-12

## Context

Supporting more languages should be deliberate. Inspiration project px0 lets
Chroma detect languages (`lexers.Match`, `lexers.Analyse`) and spreads
extension tables across Go and JS; that admits arbitrary guesses and drifts.

## Decision

- `internal/document.Builtins()` is the only composition point. One family per
  language, each declaring `Extensions` and/or exact `Filenames`, a Chroma
  `Language`, preview kind/label, and default mode.
- `Registry.Lookup` resolves exact base name, then extension, then the
  source-only plain-text fallback (`text`). Lookup always returns a family.
- Chroma only highlights the declared language. Never guess with
  `lexers.Match`/`lexers.Analyse`; unregistered text stays plain.
- `NewRegistry` rejects duplicate or clashing IDs/extensions/filenames (one
  case-insensitive namespace), unknown Chroma languages, unknown preview kinds,
  and inconsistent view policy.
- Highlighting recovers from lexer panics and falls back to escaped text.

## Consequences

- Adding a source-only language is one `source(...)` line plus tests; no
  discovery, HTTP, or frontend branches.
- Tree icons come from Pierre by file name and are independent of families.
