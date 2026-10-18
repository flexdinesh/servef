# 0015: Code navigation

- Status: Superseded for navigation by [0018](0018-lsp-only-navigation.md); ignore sources by [0023](0023-git-ignore-sources.md)
- Date: 2026-10-16

## Context

Reading code across files needs go to definition and find references. LSP
servers give precise answers but need per-language toolchains installed and
running, which breaks the single self-contained binary. servef only reads
code, so name-based answers from syntax trees are good enough.

## Decision

- Go, JavaScript, JSX, TypeScript, and TSX are navigable. TSX is its own
  family (`tsx`, Chroma `typescript` lexer) so it gets the TSX grammar.
- Parsing uses tree-sitter through `github.com/odvcencio/gotreesitter`, a
  pure-Go runtime with no cgo. Only the imported grammar packages are
  linked, with no build tags, so Homebrew, `go install …@main`, `@latest`,
  and `@vX` builds behave the same. The binary grows by about 14 MB.
- Handlers declare the optional `document.Navigator` capability; the
  registry reports `navigable`. A navigator extracts symbols, binds the
  name at a position, lists occurrences, and decides which files a module
  path (`Provides`) or an unqualified name (`Shares`) can reach. Go shares
  a directory; JavaScript families share a file and resolve relative,
  index, `@/`, and `~/` imports.
- Resolution is by name, not types: locals by scope, then imports, then
  project definitions ranked by same file, same directory, and path
  closeness. Members match by spelling. Barrel files that define nothing by
  the name fall back to project-wide matches; external modules resolve to
  the import statement.
- `internal/navigation` keeps an in-memory index per project, built in the
  background when a project page is first served and refreshed
  incrementally by size and modified time at most once a second. Nothing is
  persisted. Responses report `indexing` while it builds.
- Gitignored files and directories (nested `.gitignore` files and
  `.git/info/exclude`; not global excludes) are excluded from the index
  and from content search. They stay browsable in the tree.
- Navigation runs in Source only, following VS Code: Ctrl/Cmd+click or F12
  goes to a definition; one result opens, several open a list; clicking a
  definition lists its references. Shift+F12 and the command palette find
  references. Holding Ctrl/Cmd underlines the name under the pointer.
- Limits: files over 4 MiB, binary files, and parses over 2 s yield no
  symbols; at most 4 parses run at once; definitions cap at 50 and
  references at 1000. A request that is cancelled stops scanning.
  When `GOMEMLIMIT` is unset, servef sets a 1 GiB soft heap limit so parse
  garbage is collected before the heap doubles.

## Consequences

- Same-named symbols in reachable files can produce extra definitions and
  references; there is no type checking or overload resolution.
- New languages add a grammar import and a navigator; HTTP and UI code do
  not change.
- On Go's standard library (8k files, 88 MB) indexing takes about 40 s and
  peaks near 1 GiB, settling around 500 MB; typical projects index in
  under a second.
- An LSP backend can be added later behind the same endpoints if precision
  matters.
