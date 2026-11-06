# 0041: White canvas for Srcz Light

- Status: Accepted
- Date: 2026-10-26
- Amends: DESIGN.md's rule that the canvas recedes darker than every island, for Srcz Light only

## Context

Srcz Light set white islands on a `#d4d4d4` canvas. The deep grey read
heavy, and the theme's greys overall were darker than a light editor needs.

## Decision

- Srcz Light (and Auto when it resolves light) uses a white canvas. Grey
  elements float on it: chrome (activity bar, tab strip, review gutter)
  `#f4f4f4`, explorer `#f8f8f8`.
- Lighter neutrals: `bg` `#fafafa`, `border` `#ebebeb`, `border-strong`
  `#d0d5dc`. Text, accent, status, and syntax colors are unchanged, so the
  contrast floors hold.
- `workspace-chrome` is its own value for Srcz themes instead of the mix of
  `surface` and `canvas`; the mix would turn white with the canvas. Srcz
  Dark and named themes keep the mix.
- A new `--color-island-border` draws a 1px hairline on the navigation and
  document islands, since the white document island no longer contrasts with
  the canvas. It is `#e8e8e8` in Srcz Light and transparent everywhere else.
- The active activity tile takes `--shadow-control`, as a pressed segment
  does, so it still reads on the lighter chrome.

## Consequences

- Named light themes (Catppuccin Latte, Flexoki Light, Lupine, Rose Pine,
  Solarized Light, White) keep their darker canvases and no island edge.
- The footer sits on white in Srcz Light.
