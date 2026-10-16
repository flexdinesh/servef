# 0017: Language servers for code navigation

- Status: Accepted
- Date: 2026-10-16
- Amends: [0015](0015-code-navigation.md), [0016](0016-navigation-memory.md)

## Context

Syntax navigation (0015) matches names, so it cannot follow types, methods
on values, or re-exports the way an editor does. Installed language servers
answer these exactly, but cost far more memory than servef itself:

| Process | Project | Resident memory |
| --- | --- | --- |
| servef, idle after navigating | 44-file Go repo | ~38 MB |
| gopls | same repo | 380 to 740 MB |
| typescript-language-server (tsserver) | mid-size TS repo | 585 to 956 MB |

Memory footprint is a priority, so servers must never run unasked.

## Decision

- Language servers are opt-in through the Language Servers setting, off by
  default, stored in the browser like other settings (0013). Navigation and
  status requests send `X-Servef-Language-Servers: on`; a header rather than
  a query parameter, so another site cannot start servers through the
  browser without a CORS preflight servef never grants.
- `internal/lsp` describes servers as data (`Builtins()`): binary, args,
  install commands, docs, and the navigable families they serve with LSP
  language IDs. Servers only add precision to families servef already
  navigates; `web.New` fails closed on a server for any other family.
  Phase 1: gopls (Go) and typescript-language-server (JS, JSX, TS, TSX).
- One server runs per server and project root, started by the first opted-in
  navigation request and stopped after 10 idle minutes. Binaries are found
  on `PATH`, then common install directories; discovery is cached until
  Check again.
- Requests ask the server first and fall back to syntax navigation when it
  is missing, failing (retried after 30 s or on Check again), still starting
  (a request waits up to 2 s), or answers only outside the project. Results
  keep only indexed, in-scope, non-ignored project files. Responses carry
  `provider` (`language-server` or `syntax`) and `server`.
- While a server is available, syntax navigation scans per request instead
  of building its index.
- Documents are synced with full text; files of the server's families are
  reported through `workspace/didChangeWatchedFiles` by size and mtime
  stamps. At most 64 documents stay open per server.
- `/api/navigation-status` reports, without starting anything, who answers
  navigation for a file and the server's state (missing, stopped, starting,
  ready, failed), memory, install commands, and docs. The status bar shows
  the provider and a panel with install instructions, Check again, and a
  link to the setting.
- Locations outside the project (standard library, dependencies) are
  dropped; opening them is deferred.

## Adding a server

Add an entry to `lsp.Builtins()` for families that already have a
`document.Navigator`, with install commands and docs. Add web tests through
the fake server in `internal/lsp/lsptest` when behavior differs.

## Consequences

- Off: no change in memory or behavior.
- On: compiler-accurate results once a server is ready, at the server's
  memory cost while it runs; servef reports that cost in the status panel.
- First requests answer from syntax navigation while servers start.
- Projects on TypeScript 7 (native) ship no `tsserver.js`, so
  typescript-language-server fails there; navigation falls back to syntax
  and the panel shows the error. TypeScript 7's own `tsc --lsp --stdio`
  measured ~180 MB on this repository and is a candidate follow-up.
