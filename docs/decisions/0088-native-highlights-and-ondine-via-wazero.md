# 0088: Native Highlights and Ondine via wazero

- Status: Superseded by [0094](0094-mygo-app-uses-web-views.md)
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
  and the error. At most GOMAXPROCS instances run at once (64 MiB each),
  two idle ones are pooled, and one that failed, panicked, ran past 10 s,
  or grew past 8 MiB is closed; `Resolve` is `resolveThemeSyntax`.
- `ondine`: the embedded module, called as the glue's `Wasm.ts` and
  `Render.ts` call it; `Render` returns the SVG, size, and links, or an
  `*Error` with code, line, column, snippet, and message. One instance under
  a mutex (at most 128 MiB; closed after an engine failure, a panic, a
  render past 10 s, or past 32 MiB). It loads lazily, on the first diagram.
- Neither keeps a failed load: the next call (or "Reload diagrams") tries
  again, as the web app clears its pending import.
- `wasmcache`: `Load` compiles on runtimes sharing one wazero compilation
  cache per process, optionally on disk (`SetDir`), with
  `WithCloseOnContextDone` so call deadlines stop runaway calls, and returns
  the compiler's garbage to the system. A disk entry wazero rejects
  (`compilationcache: checksum mismatch`) clears the directory and compiles
  again, then in memory alone. The native app keeps it in the user cache
  directory (`servef-native/wasm`, about 7 MiB) and warms Highlights at
  launch.
- `svgdraw`: draws Ondine's SVG with mygo's Painter (shapes, paths, arcs,
  groups, nested viewports, transforms, strokes, fills, dashes, opacity,
  text with anchors and baselines, `var()`, `color-mix()`); `Style.Vars`
  supplies the `--ondine-*` variables per frame, so a theme switch redraws
  without rendering again. Diagram authors control style values (Mermaid's
  `style` and `classDef`), so parsing rejects malformed and non-finite
  values, and `Parse` and `Draw` turn any panic into an error (`Doc.Err`);
  the view then shows the source with the web's generic message. Tests
  draw hostile values directly and through Ondine, plus a fuzz target.

Pins: `packages/mygo/scripts/sync-wasm.sh` (`mise run native:wasm-sync`)
copies each module and its license files from `apps/web/node_modules`, writes
`pin.go` (version, lockfile integrity, sha256), and regenerates the goldens
with the same JavaScript glue: token scopes for 36 samples covering every
family language, and Ondine's SVG for 11 diagrams. Tests fail when the
embedded bytes disagree with `pin.go` (`packages/mygo`), when
`pnpm-lock.yaml`'s version or integrity for `@pierre/highlights` or
`ondine` does (`apps/native/pins_test.go`; the lockfile is a native
validation input), and when Go's tokens or SVG differ from the goldens
(byte-identical SVG). Release packages carry `THIRD_PARTY_NOTICES.txt`
(`apps/native/scripts/notices.sh`): Highlights' LICENSE and NOTICE,
Ondine's LICENSE, and wazero's.

`apps/native` drops Chroma. `internal/highlight` mirrors
`syntax-highlight.ts` and `syntax-theme.ts` (servef's eight syntax
categories plus inserted, deleted, link, italic, and bold), and its test
compares native spans with the rows `highlightLines` renders
(`apps/native/scripts/web-highlights.ts`), and hashes the mirrored web files
so a web change asks for new goldens. Source, Markdown fences, and diff sides
all go through it. Fences gain their language label once highlighted, as on
the web. Mermaid fences, and the documents the server declares as the
`mermaid` family (`.mmd`; the page's `documentType.id`, since the contract
has no diagram capability), render through Ondine for the frame's width
(padding 16), centered on a bordered surface; the source shows while
rendering and on errors, with Ondine's diagnostic (or the web's generic
message) below. A new width renders again once it holds for 250 ms; the
last render scales meanwhile. Highlighted fences and renders are kept per
open document, one per fence or diagram source (mygo builds every block
each frame, so results must outlive it), and never started twice. The
`--ondine-*` mapping mirrors `mermaid.css`, and a test checks it.

Headless text (`ui.Tester`) shapes and draws through the system's Pango,
cairo, HarfBuzz, and fontconfig (dlopen); mygo bundles no fonts. The
`test:native` container installs them, and `MYGO_REQUIRE_TEXT=1` makes
svgdraw's glyph test fail rather than skip without them.

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
