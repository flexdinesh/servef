# 0006: Explorer filename filter and tab icons

- Status: Accepted
- Date: 2026-10-12

## Context

The explorer's type picker listed registered families. With ~40 families and
unregistered files in scope, a checkbox list stops scaling.

## Decision

- Replace the type picker with a filename filter field: comma-separated,
  case-insensitive base-name globs with the same semantics as `--filename`.
- `web/src/filename-filter.ts` ports Go `path.Match` and folds case rune by
  rune like Go `strings.ToLower`; tests reuse Go's cases.
- The filter changes tree visibility only and is not persisted. Invalid
  patterns keep the last valid filter. A `Serving …` line shows a narrowed CLI
  scope.
- Tabs use Pierre's icon resolver and sprite sheet. Pierre's colours live in
  its shadow DOM, so servef maps colour families to theme tokens.

## Consequences

- UI and CLI glob behavior must change together; parity tests guard it.
