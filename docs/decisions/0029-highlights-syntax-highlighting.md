# 0029: Pierre Highlights for syntax highlighting

- Status: Accepted
- Date: 2026-10-22
- Supersedes in part: [0024](0024-source-control-view-and-working-tree-diffs.md) (Shiki grammars, grammar subset, and highlighting workers)

## Context

Diffs highlighted with Shiki's JavaScript regex engine through
`@pierre/diffs` 1.5.2: 35 bundled TextMate grammars (about 3 MB) and two
workers. `@pierre/diffs` 1.6 adds a third backend, Highlights
(`@pierre/highlights`): 80 built-in lexers compiled to one WebAssembly module
with SIMD. It tokenizes a 4 MB Go file in about 90 ms in Node, needs no
grammar downloads, and colors tokens from CSS variables.

## Decision

- Diffs highlight with Highlights only (`preferredHighlighter:
  'highlights'`). No Shiki backend ships: a build-only Vite plugin
  (`servef-diffs-subset`) resolves Pierre's `shiki` imports to a stub
  (`web/src/shiki-unavailable.ts`) and its Pierre, Shiki, and Highlights
  theme catalogs to empty subsets. Browsers without WebAssembly SIMD show
  plain text.
- One theme, `web/src/syntax-theme.ts`, maps Highlights' syntax categories
  to servef's `--color-syntax-*` tokens (comment, keyword, type, function,
  variable, string, number, operator), so diffs follow every servef theme.
- Text up to 100,000 characters (both sides of a diff) highlights on the
  main thread; larger text goes to a pool of two workers
  (`web/src/highlight-thread.ts`).
- Families map to Highlights languages in `web/src/diff-language.ts`.
  `fish`, `ini`, `properties`, and `rst` have no Highlights lexer and map to
  `text`, as images and Mermaid do. Tests require every mapped language to be
  a Highlights language.
- `@pierre/diffs` 1.6.1, `@pierre/highlights` 0.2.0, and `@pierre/theming`
  1.0.2 are exempt from pnpm's three-day release age until 2026-10-25.

## Consequences

- Fish, INI, Properties, and reStructuredText diffs lose highlighting.
- Token boundaries and colors differ from Shiki's; Highlights keeps trailing
  whitespace in a token.
- Highlights is beta; its API may change between minor releases.
- The binary shrinks from 31.8 MB to 29.8 MB.
