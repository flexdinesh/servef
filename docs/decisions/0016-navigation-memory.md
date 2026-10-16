# 0016: Navigation memory

- Status: Accepted
- Date: 2026-10-16
- Amends: [0015](0015-code-navigation.md)

## Context

After 0015, an idle server on a small repository held 90 to 280 MB, against
about 25 MB before:

- gotreesitter keeps parse arenas in a global pool with strong references,
  so the collector never reclaims them; parsers sit in a `sync.Pool` until
  two collections, and decoded grammars stay in a package cache.
- Index snippets were substrings of each file's text, pinning every indexed
  file in memory; entries were stored twice.
- Committed minified bundles (e.g. `internal/web/dist/`) cost most of the
  parse time and peak memory while offering nothing to navigate.
- Every project opened paid for an index, navigated or not.

## Decision

- Index on first use. The first definition or references request starts
  the background build; until it finishes, requests parse only the files
  that mention the name (for imports, only files the module can provide),
  so answers are complete. The `indexing` response field is removed.
- The index holds positions, not text: snippets are read from files when
  answering, and the by-name map points at per-file entries.
- Once navigation work has been idle for 2 s, drain gotreesitter's arena
  pools, collect twice, and return freed memory to the OS. After 5 minutes
  idle, also unload decoded grammars; the next parse reloads them in
  milliseconds.
- Files with a line over 1000 bytes in their first 64 KiB are treated as
  minified: no symbols, skipped by references. They stay browsable.
- Index builds use one parse worker per 2 MiB of changed source, up to 4.
- No SQLite. The compact index is about 80 MB for Go's standard library
  (393k symbols) and under 1 MB for typical projects; a database would add
  binary size, disk state, and invalidation for little gain.

## Consequences

- Release builds (`CGO_ENABLED=0`) on a 44-file Go repository: 29 MB with
  a page open and no navigation, about 38 MB idle after navigating (was 158
  MB). Go's standard library: 80 MB idle with its full index (was 500 MB).
- Peaks remain while indexing or finding references, then fall back within
  seconds.
- The first request after opening a project, or after an idle release,
  pays for parsing; typically tens of milliseconds.
