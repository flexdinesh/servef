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
- Source and Format highlight in the browser too. `/api/page` drops
  `sourceLines` and declares `highlightDisabled` (binary, too large, or over
  the 0004 4 MiB limit, large SVG included); `/api/format` returns the
  formatter's text as `formatted`. `web/src/syntax-highlight.ts` tokenizes
  the original text and renders escaped rows with `hl-<role>` classes; each
  row's text equals its source line, so navigation columns, selection, and
  copy are unchanged. Rows show plain text until highlighting arrives. One
  worker serves large Source and Format text and ends when the last tab
  closes. Highlights is imported lazily, so a WebAssembly failure leaves
  code plain without breaking the app.
- `document.Type.Language` names a Highlights language (`text` for none);
  diffs and Source read it from `documentType`. `NewRegistry` checks that a
  language is well formed; `testdata/document-families.json` lists every
  family's language, and web tests require each to be a Highlights language.
  `fish`, `ini`, `properties`, and `rst` have no Highlights lexer and use
  `text`, as Mermaid does.
- `@pierre/diffs` 1.6.1, `@pierre/highlights` 0.2.0, and `@pierre/theming`
  1.0.2 are exempt from pnpm's three-day release age until 2026-10-25.

## Consequences

- Fish, INI, Properties, and reStructuredText lose highlighting.
- An unknown but well-formed language fails tests, not startup: Go cannot
  see Highlights' language list.
- A 4 MB Go file: `/api/page` drops from 72.5 MB in 4.4 s to 4.4 MB in
  15 ms, and servef RSS after loading it four times from about 413 MB to
  58 MB. The browser shows it highlighted after 19 s instead of 34 s;
  rendering every row now dominates. A 100 KB file takes 0.76 s instead of
  1.07 s. Browser heap is unchanged (about 300 MB for 4 MB).
- Token boundaries and colors differ from Shiki's; Highlights keeps trailing
  whitespace in a token.
- Highlights is beta; its API may change between minor releases.
- The binary shrinks from 31.8 MB to 30.1 MB.
