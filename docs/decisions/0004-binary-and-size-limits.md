# 0004: Binary detection and size limits

- Status: Accepted
- Date: 2026-10-12

## Context

With `*` as the default scope, any file can be opened or searched, including
binaries and very large logs. Files were read whole with no limits.

## Decision

Constants live in `internal/document/content.go`:

| Rule | Effect |
| --- | --- |
| NUL byte in first `SniffLength` (8000) bytes | `binary`: no source, no search; "Binary file" note |
| Raster image family (`!Type.HasTextSource()`) | Always binary, regardless of sniff |
| Over `PreviewLimit` (4 MiB) | `previewDisabled`: no highlighting, Format, or non-image preview |
| Over `SearchLimit` (8 MiB) | `searchExcluded`: skipped by content search |
| Over `DisplayLimit` (32 MiB), non-image | `tooLarge`: Stat only, never read; "Too large to display" note |

Image families keep Preview at any size; bytes stream from `/api/raw`.

## Consequences

- No encoding detection: UTF-16 files contain NUL bytes and show as binary.
- Files over 32 MiB are not sniffed, so huge binaries show "Too large".
