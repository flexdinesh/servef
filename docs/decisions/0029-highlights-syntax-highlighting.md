# 0029: Pierre Highlights for syntax highlighting

- Status: Accepted
- Date: 2026-10-22
- Supersedes in part: [0002](0002-explicit-presentation-registry.md) (Chroma languages and highlighting), [0024](0024-source-control-view-and-working-tree-diffs.md) (Shiki grammars, grammar subset, and highlighting workers), [0025](0025-ondine-mermaid.md) (Mermaid Source note)

## Context

The server highlighted Source, Format, and Markdown fences with Chroma and
sent every line as HTML beside the raw source: a 4 MB Go file became a
72.5 MB `/api/page` response that took 4.4 s. Diffs highlighted with Shiki's
JavaScript regex engine through `@pierre/diffs` 1.5.2: 35 bundled TextMate
grammars (about 3 MB) and two workers. `@pierre/diffs` 1.6 adds a third backend, Highlights
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
- Markdown fences render escaped with goldmark's `language-<name>` class and
  highlight in the preview when Highlights knows the name or alias; only
  those fences show a language label. Other fences stay plain. Chroma is no
  longer a dependency.
- `document.Type.Language` names a Highlights language (`text` for none);
  diffs and Source read it from `documentType`. `NewRegistry` checks that a
  language is well formed; `testdata/document-families.json` lists every
  family's language, and web tests require each to be a Highlights language.
  `fish`, `ini`, `properties`, and `rst` have no Highlights lexer and use
  `text`, as Mermaid does.
- `@pierre/diffs` 1.6.1, `@pierre/highlights` 0.2.0, and `@pierre/theming`
  1.0.2 are exempt from pnpm's three-day release age until 2026-10-25.

## Consequences

- Fish, INI, Properties, and reStructuredText lose highlighting, and fewer
  fence languages highlight than Chroma's lexers covered.
- Browsers without WebAssembly SIMD show plain text everywhere.
- An unknown but well-formed language fails tests, not startup: Go cannot
  see Highlights' language list.
- Token boundaries and colors differ from Chroma's and Shiki's; Highlights
  keeps trailing whitespace in a token.
- Plain rows swap to highlighted ones once tokenized, which clears a native
  text selection made in that moment.
- Highlights is beta; its API may change between minor releases.
- Measured with Go files of 100 KB, 1 MB, and 4 MB (before, after):

  | Measure | Before | After |
  | --- | --- | --- |
  | Binary | 31.8 MB | 26.3 MB |
  | `/api/page`, 4 MB file | 72.5 MB in 4.4 s | 4.4 MB in 16 ms |
  | servef RSS after loading the 4 MB file four times | 413 MB | 47 MB |
  | Time to highlighted Source, 100 KB / 1 MB / 4 MB | 1.1 s / 8.2 s / 34 s | 0.8 s / 5.2 s / 20 s |
  | Browser heap, 100 KB / 1 MB / 4 MB | 14 / 81 / 304 MB | 15 / 83 / 300 MB |

  Rendering every Source row now dominates large-file time.
