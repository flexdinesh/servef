# 0075: Desktop title band

- Status: Accepted
- Superseded by: [0095](0095-desktop-app-is-mygo-electron-removed.md): the band needs a window-controls overlay WebKit lacks
- Date: 2026-11-02
- Revises: [0067](0067-desktop-shell-and-protocol-proxy.md) Windows (window
  chrome, bridge v1 gains optional `showAppMenu`), DESIGN.md "No app header"

## Context

Desktop windows used the OS title bar. It ignores the srcz theme, so a
dark workspace sat under a light GTK or Windows caption, and on Wayland
Chromium drew its own unthemed one. VS Code hides the native bar and draws a
title bar in its theme. The preload bridge exists only on
`srcz-app://local` (0067), so anything every window needs (ssh and CLI
connections too) cannot depend on it.

## Decision

- Every window hides the native title bar (`titleBarStyle: "hidden"`). macOS
  keeps its traffic lights, centered in a 32px band; Windows and Linux use
  `titleBarOverlay` controls 32px tall.
- The page draws the band: `--titlebar-height` is
  `env(titlebar-area-height, 0px)`, so browsers render nothing and need no
  JS detection. The band is the theme's `canvas`, continuing the gutters and
  footer above the islands. It drags the window and shows the window title
  (`document.title`) centered on the window, 12px `text-secondary`,
  `text-muted` when the window is in the background, clipped clear of the
  controls.
- The page's `theme-color` meta carries the canvas to main. `theme.ts`
  resolves `--color-canvas` to `#rrggbb` on every theme change; main listens
  to `did-change-theme-color`, accepts only `#rrggbb` (pages are untrusted),
  and sets the window background and, outside macOS, the overlay color with
  a light or dark control color by luminance. New windows start with the
  last reported color, else the Source Zero canvas for the OS polarity. No bridge
  call, so ssh and CLI windows follow their page's theme too. Phone browsers
  also tint their chrome to the canvas.
- Windows and Linux lose the visible menu bar; accelerators still work. A
  menu button at the band's start pops the application menu. It needs the
  bridge, so bridge v1 gains optional `showAppMenu(x, y)`, exposed outside
  macOS only; the UI feature-detects it. Main honors it only from local
  frames and only for a finite point inside the sender's window. ssh and CLI
  windows keep accelerators only.
- App pages (failure, Connect) pad by the band, draw a drag strip, and
  report their background as `theme-color`.

## Consequences

- An older remote srcz UI (ssh) draws no band: the overlay controls cover
  the top-right of its tab strip and its top edge does not drag until the
  remote upgrades. Accepted over a native frame for non-local windows
  (inconsistent look) or a separate title view (one more renderer per
  window).
- The first window opens in the default canvas until its page reports, so a
  named theme flashes once at startup.
- The band is a fixed 32px; it does not scale with the Font Size setting.
