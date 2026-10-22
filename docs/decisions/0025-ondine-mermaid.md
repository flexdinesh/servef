# 0025: Ondine renders Mermaid as themed SVG

- Status: Accepted; Source highlighting superseded by [0029](0029-highlights-syntax-highlighting.md)
- Date: 2026-10-21
- Supersedes: [0022](0022-excalidraw-only-mermaid.md) (renderer only; feature flags stay removed)

## Context

Excalidraw drew Mermaid on an interactive canvas. It shipped about 18 MB of
JavaScript and fonts, needed text-raster workarounds, could not draw
`gitGraph`, and reported every failure with one generic message. Its font
fallback was the only reason the CSP allowed `https://esm.sh`.

[Ondine](https://ondine.sh) compiles Mermaid to SVG with a Rust WebAssembly
module that has no imports. All diagram types fit in one 1.48 MB module; a
diagram renders in a few milliseconds. Its errors match Mermaid's diagnostics,
and it reads colors and fonts from `--ondine-*` CSS custom properties.

## Decision

- Ondine is the only Mermaid renderer. Excalidraw, its font assets, and
  `EXCALIDRAW_ASSET_PATH` are removed.
- Diagrams render in the browser. Go still sends Mermaid fences as escaped
  source; `/api/page` and servef memory are unchanged.
- One `all` module loads on the first visible diagram, compiles with
  `WebAssembly.compileStreaming`, and serves every diagram on the page. A failed
  load shows source with a reload action.
- Diagrams are inline SVG mapped to theme tokens (`--color-*`, `--font-mono`),
  so every theme applies without re-rendering. Output is monochrome plus accent.
- An expand button opens a dialog with zoom (buttons, Ctrl/Cmd+wheel) and drag
  pan. Inline diagrams do not zoom.
- Links inside diagrams are unwrapped to plain text; diagrams do not navigate.
- Syntax errors show the source plus ondine's message, line, and caret.
- `.mmd` and `.mermaid` files are the `mermaid` family. Its handler previews
  the file as one escaped mermaid fence under the existing `markdown` preview
  kind, so no contract or frontend change is needed. They open in Preview
  (`[Preview] [Source]`, like images); Source is plain text because Chroma has
  no Mermaid lexer. They stay searchable.
- The CSP allows `'wasm-unsafe-eval'` and drops `https://esm.sh`. Go serves
  `.wasm` assets as `application/wasm`.
- `ondine@0.0.3` is exempt from the three-day `minimumReleaseAge` until
  2026-10-23.

## Consequences

- The binary loses Excalidraw's assets and gains a 1.48 MB WebAssembly module.
- `gitGraph` and the other ondine diagram types render; unknown types and
  invalid source fall back to source with a diagnostic.
- In-place canvas pan and zoom is gone; large diagrams use the dialog.
- Labels use a monospace font because ondine sizes text by character count.
- ondine is a young 0.0.x package pinned to an exact version.
