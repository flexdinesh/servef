# 0104: Source Zero logo

- Status: Accepted
- Date: 2026-11-07
- Revises: [0100](0100-rename-to-srcz.md)'s status bar text
- Extends: [0102](0102-sharp-slate-panes.md)'s square corners to the desktop icon

## Context

The product had no logo: the web app shipped a blank favicon, the site had none,
and the desktop icon was a rounded terminal prompt, which fits neither the name
nor square corners.

## Decision

**The mark is a Z inside a zero.** A slashed zero and a Z share one diagonal, so
one shape says both `srcz` and Source Zero. The Z takes the ink. The zero's side
rails take coral and stand apart from the Z on a small gap, so the two colors
never touch. Coral belongs to the logo alone (`--color-brand-mark`); the app's
accent stays blue for state and interaction.

| Ground | Z | Rails |
| --- | --- | --- |
| Graphite `#2b2d33`, and dark surfaces | `#ffffff` | `#ff6b4a` |
| White, and light surfaces | `#2b2d33` | `#f2502e` |

Web and site draw it on two grids: 16 for 24px and under, 32 above.

**Where it ships.**

- Desktop: `apps/desktop/resources/icon.png`, a square graphite tile in the old
  icon's 64px transparent margin, so it matches neighboring icons' size. mygo
  derives the Linux PNG and the macOS `.icns` from this one raster, so the mark
  fills the tile at a scale that holds at small sizes.
- Web favicon: an inline data URI in `apps/web/index.html`, following light and
  dark. The CSP already allows `data:` images, so no route is added.
- Status bar: the mark replaces the text `srcz`, in one color so the chrome stays
  quiet, with the rails at 55% so they stay distinct at 16px. The link stays
  labelled `srcz home`.
- Welcome: the two-color mark above the title, with the Z in the theme's primary
  text color. The empty pane stays text-only.
- Site: favicon, docs logo, and a 1200x630 social image from
  `apps/site/public/`, and the mark beside the landing wordmark.
- README: a theme-aware mark.

**Masters live in `docs/brand/`.** The docs lockups and the social card outline
JetBrains Mono, so they need no font; the landing wordmark stays live text in its
system mono stack, so its face varies by OS. Rasters are committed and regenerated
by hand; there is no build step:

```sh
rsvg-convert -w 1024 -h 1024 docs/brand/tile.svg -o apps/desktop/resources/icon.png
rsvg-convert -w 1200 -h 630 docs/brand/og.svg -o apps/site/public/og.png
```

`BrandMark` (web) and `Mark` (site) repeat the paths, as does the favicon; the
site and web app share no package for it.

## Consequences

- The square tile sits slightly off among rounded macOS Dock icons.
- Zed, whose visual language 0102 follows, also has a Z-and-frame mark. Its
  concentric line squares are built differently, but both sit in one category. No
  trademark search was done.
- `og:image` is relative until the site sets `baseUrl`, and some crawlers ignore
  relative URLs. The config already uses `baseUrl` when it is set.
- GitHub's social preview is a manual upload.
- Vocs 2.10.3 hard-codes the docs logo's `alt` as "Logo", so
  `patches/vocs@2.10.3.patch` makes it the site title. Drop the patch when Vocs
  fixes it.
