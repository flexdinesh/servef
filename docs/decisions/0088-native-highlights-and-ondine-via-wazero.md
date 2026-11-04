# 0088: Native Highlights and Ondine via wazero

- Status: Accepted (experimental, with 0082)
- Date: 2026-11-04
- Revises: [0025](0025-ondine-mermaid.md), [0029](0029-highlights-syntax-highlighting.md), [0082](0082-native-mygo-experiment.md), [0086](0086-native-agent-tooling-and-ports.md)

## Context

0086 chose to port the web app's engines rather than reimplement them: the
native app highlighted with Chroma (other token boundaries than Highlights)
and showed Mermaid as code. Both engines ship as WebAssembly the web app
already loads: `@pierre/highlights` (`highlights.wasm`, Apache-2.0) and
`ondine` (`all.generated.wasm`, MIT). The native app must stay pure Go: no
cgo, no web view.

## Decision

Run both modules under wazero (v1.12, its compiler) in generic
`packages/mygo` packages that import nothing from servef:

- `highlights`: the embedded module, called as `dist/highlighter.js` calls
  it (header in page 0, input at 64 KiB, output mode 3, UTF-16 line
  records); Go implements its only imports, `env.is_id_start` and
  `env.is_id_continue` (Unicode XID). `Tokens` mirrors `codeToTokens`
  (lines, scopes, `tokenizeMaxLineLength`) in byte offsets; an unknown
  language, invalid UTF-8, an engine error, or a panic return plain lines
  and the error. Instances are pooled (two idle, at most 64 MiB each; one
  that grew past 8 MiB is closed); `Resolve` is `resolveThemeSyntax`.
- `ondine`: the embedded module, called as the glue's `Wasm.ts` and
  `Render.ts` call it; `Render` returns the SVG, size, and links, or an
  `*Error` with code, line, column, snippet, and message. One instance under
  a mutex (at most 128 MiB; dropped after an engine failure or past
  32 MiB). It loads lazily, on the first diagram.
- `wasmcache`: one wazero compilation cache per process, optionally on disk
  (`SetDir`); `Compile` returns the compiler's garbage to the system once.
  The native app keeps it in the user cache directory
  (`servef-native/wasm`, about 7 MiB) and warms Highlights at launch.
- `svgdraw`: draws Ondine's SVG with mygo's Painter (shapes, paths, arcs,
  groups, nested viewports, transforms, strokes, fills, dashes, opacity,
  text with anchors and baselines, `var()`, `color-mix()`); `Style.Vars`
  supplies the `--ondine-*` variables per frame, so a theme switch redraws
  without rendering again.

Pins: `packages/mygo/scripts/sync-wasm.sh` (`mise run native:wasm-sync`)
copies each module and its license files from `apps/web/node_modules`, writes
`pin.go` (version, lockfile integrity, sha256), and regenerates the goldens
with the same JavaScript glue: token scopes for 36 samples covering every
family language, and Ondine's SVG for 11 diagrams. Tests fail when the
embedded bytes or `pnpm-lock.yaml` disagree with `pin.go`, and when Go's
tokens or SVG differ from the goldens (byte-identical SVG).

`apps/native` drops Chroma. `internal/highlight` mirrors
`syntax-highlight.ts` and `syntax-theme.ts` (servef's eight syntax
categories plus inserted, deleted, link, italic, and bold), and its test
compares native spans with the rows `highlightLines` renders
(`apps/native/scripts/web-highlights.ts`), and hashes the mirrored web files
so a web change asks for new goldens. Source, Markdown fences, and diff sides
all go through it. Fences gain their language label once highlighted, as on
the web. Mermaid fences and `.mmd` previews render through Ondine for the
frame's width (padding 16), centered on a bordered surface; the source shows
while rendering and on errors, with Ondine's diagnostic (or the web's
generic message) below. The `--ondine-*` mapping mirrors `mermaid.css`, and
a test checks it.

## Measurements

Headless agent host (`cmd/native-agent`, software renderer) on the API
fixtures, Linux x86-64 (Core Ultra 7 155U), `VmRSS` after the preview
settles, three runs each, before = origin/main (Chroma):

| Open document | Before | After |
| --- | --- | --- |
| Markdown preview (`guides/getting-started.md`) | 51 to 53 MiB | 51 to 54 MiB (peak 58 to 61 during Highlights' first compile) |
| Mermaid document (`guides/diagrams.md`, 5 diagrams) | 54 to 57 MiB | 66 to 68 MiB (peak 97 to 99 during Ondine's first compile) |

Without `Compile`'s release, the Mermaid document held 101 to 104 MiB: the
compiler's garbage. First use: Highlights compiles in 0.22 to 0.33 s, Ondine
in 1.3 to 1.6 s; from the disk cache they load in 10 ms and 50 to 70 ms.
A render takes about 2 ms. The binary grows by 1.5 MiB (23.6 to 25.1 MB).

## Consequences

- Native and web highlight and draw from the same engine bytes; a version
  bump is one sync run plus review of the golden diff.
- Ondine's compiled code (about 11 MiB) stays resident once a diagram has
  shown; the instance's memory is small (1.2 MiB for the samples).
- svgdraw covers what Ondine emits, not SVG in general; unknown elements
  draw nothing. Text uses mygo's monospace font, so widths can differ
  slightly from the browser's.
- Not yet: diagram expand and zoom (M4), and the web's copy-chip layout for
  fences (native keeps mygo's head row).
