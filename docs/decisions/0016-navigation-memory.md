# 0016: Navigation memory release

- Status: Accepted
- Date: 2026-10-16
- Amends: [0015](0015-code-navigation.md)

## Context

After 0015, an idle server on a small repository held 150 to 280 MB, against
about 25 MB before. gotreesitter keeps parse arenas in a global pool with
strong references, so the collector never reclaims them; parsers sit in a
`sync.Pool` until two collections. Committed minified bundles (e.g.
`internal/web/dist/`) cost most of the parse time and peak memory while
offering nothing to navigate.

## Decision

- Once navigation work (builds and requests) has been idle for 2 s, drain
  gotreesitter's arena pools, collect twice, and return freed memory to the
  OS.
- Files with a line over 1000 bytes in their first 64 KiB are treated as
  minified: no symbols, skipped by references. They stay browsable.
- Index builds use one parse worker per 2 MiB of changed source, up to 4.
- LSP is not used to save memory: language servers run as separate
  processes that typically hold more than servef does.

## Consequences

- Idle memory: about 40 MB on a 44-file Go repository (was 158 MB), 57 MB on
  this repository (was 284 MB), 200 MB on Go's standard library.
- Transient peaks remain while indexing or finding references.
- The first parse after an idle release reallocates arenas; it costs
  milliseconds.
