# 0074: Landing page and Cloudflare Pages

- Status: Accepted; install tabs (the CLI and desktop scripts) revised by [0079](0079-releases-edge-prerelease-and-install-scripts.md)
- Date: 2026-11-02
- Supersedes: [0026](0026-vocs-user-docs-site.md) docs pages at `/` and
  "not deployed yet"

## Context

The Vocs site (0026) put the docs Introduction at `/` and was never deployed.
servef needs a marketing landing page that links to the docs, and the site
will be hosted on Cloudflare Pages from this monorepo as plain static files.
The landing is a rough starting point that will change heavily.

## Decision

- The landing lives inside the existing Vocs site, not a separate package:
  `site/src/pages/index.mdx` uses `layout: blank` and renders a React
  component from `site/src/landing/`. One package, one build, one output
  directory.
- Docs move from `/<page>` to `/docs/<page>` (`site/src/pages/docs/`). No
  redirects: the site was never deployed, so no inbound links exist.
- Layout follows a single-column marketing scroll: nav, hero with a tabbed
  install command (script, Homebrew, Go) and CTAs, three feature sections
  (review, walkthroughs, desktop and ssh), a repeated install CTA, and a
  footer. Docs is a nav link.
- Visuals are inline React SVGs animated with CSS keyframes. Each scene's base
  styles are its final frame, so `prefers-reduced-motion` shows a complete
  picture.
- Styles are plain CSS scoped under `.landing` (`landing.css`), with the
  DESIGN.md palette (white light theme, `#2563eb` / `#78a8ff` accent) through
  `light-dark()`. Fonts are Vocs and system fonts; no new dependencies.
- Light and dark themes share Vocs' `vocs-theme` storage and
  `data-vocs-theme` attribute, so the landing and docs keep one theme.
- Hosting is Cloudflare Pages Git integration from the repository root
  (`.node-version` selects Node 26), with a filtered pnpm install for
  `@servef/site...`, `pnpm build:site`, and output `site/dist/public`.
  Settings are documented in `docs/development.md`; no `wrangler.toml`,
  because Pages build commands and variables live in the dashboard and the
  file would sit at the repository root.

## Consequences

- The landing relies on two small selectors outside `.landing` to remove the
  blank layout's article padding and main width cap; Vocs upgrades may move
  them.
- The Cloudflare build image's Node 26 and pnpm 11 support is unverified until
  the first deploy. If it fails, build in GitHub Actions and deploy with
  `wrangler pages deploy` instead.
- The README links to `site/src/pages/docs` on GitHub until a domain exists.
