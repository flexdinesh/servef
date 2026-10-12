# 0001: Filename globs define CLI scope

- Status: Accepted
- Date: 2026-10-12

## Context

`--file-type` selected registered families (`md,json,yaml`). Every new language
needed a registration before its files could appear, so scope and presentation
were coupled, and files such as `Dockerfile` or `LICENSE` could never be shown.

## Decision

- Remove `--file-type`. Add `--filename`: one comma-separated string of globs
  (Go `path.Match` syntax) matched against the file base name, case-insensitively.
  Default `*`.
- Reject empty entries, invalid globs, and patterns containing `/` at startup.
- `cmd` builds one `files.NameFilter` and passes it to discovery, document
  access, Markdown link rewriting, and content search.
- Any file matching the scope is browsable, registered or not.

## Consequences

- Scope says *which* files; the registry (0002) says *how* to show them.
- Path globs (`docs/**`) are out of scope; use `--exclude` for directories.
- The page response exposes `filenamePatterns` so the UI can show a narrowed
  CLI scope.
