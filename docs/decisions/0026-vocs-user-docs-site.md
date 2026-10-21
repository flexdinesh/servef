# 0026: Vocs user docs site

- Status: Accepted
- Date: 2026-10-22

## Context

All user documentation lived in `README.md`: install, usage, scope, views,
limits, code navigation, and the contributor file-type guide. It had grown long
and mixed audiences. Contributor docs already live in `docs/`.

## Decision

- User docs are a [Vocs](https://vocs.dev) site in the `site/` pnpm workspace
  package (`@servef/site`). Pages: Introduction, Getting started, Usage,
  Browsing, Code navigation.
- Contributor docs (`docs/development.md`, `architecture.md`, `release.md`) and
  decision records stay in `docs/` and are not published. The file-type guide
  moves from README to `docs/development.md`.
- `renderStrategy: 'full-static'`: plain HTML, no server runtime, so no OG
  image, feedback, or MCP endpoints. Built-in search runs in the browser; the
  Ask AI menu only links out to external chat apps.
- Theme: Vocs defaults with servef's accent (`#2563eb` light, `#78a8ff` dark).
- CI builds the site on every run; the build fails on dead internal links. The
  site is not deployed yet.
- README keeps a short pitch, feature summary, install, quick start, and links
  to `site/src/pages` on GitHub until hosting exists.
- `site/` pins its own React 19.3 (a Vocs peer); `web/` keeps its React version.
  Install scripts of transitive `es5-ext` and `vue-demi` are denied.

## Consequences

- The site is build-time only; the servef binary and `web/` are unaffected.
- Until deployment, readers following the README link see raw MDX on GitHub.
  Pages stay plain Markdown so they read acceptably there.
- Release archives ship the shorter README.
- User-visible changes update `site/src/pages` and the README summary.
- Vocs depends on Waku, still a release candidate; versions are pinned exactly.
