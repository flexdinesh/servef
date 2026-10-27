# 0055: Thin, hover-revealed scrollbars with reserved gutters

- Status: Accepted
- Date: 2026-10-28

## Context

servef styled none of its own scroll areas. Browsers with classic scrollbars
(Linux, Windows, macOS set to always show them) drew a wide grey bar with a
track beside the review and the sidebar views. Content also jumped sideways
whenever overflow came and went: switching from a short document to a long
one, or a list growing past its pane. Only Pierre's trees and diffs drew
their own thin, hover-revealed thumbs. VS Code overlays a 10px slider that
takes no layout space.

## Decision

- Every scroll area draws one scrollbar: `--scrollbar-size` (0.625rem, 10px
  at the default font size), no track, arrows, or corner, and a square thumb
  inset 2px. The thumb is translucent ink (`--color-scrollbar-thumb`, 22%
  of `text-primary`; 35% hovered, 45% dragged), so every theme follows.
- The thumb shows only while the pointer is over its scroll area, as in VS
  Code.
- Chromium and Safari use `::-webkit-scrollbar`. Firefox gets
  `scrollbar-width: thin` and `scrollbar-color` behind
  `@supports not selector(::-webkit-scrollbar)`; Chromium ignores the
  pseudo-elements once `scrollbar-color` is set.
- Scroll areas whose content changes reserve the gutter with
  `scrollbar-gutter: stable`: the document pane, Source Control panes and
  lists, the base picker, Search, Walkthroughs, Explorer warnings, the quick
  pick, and Settings. Not globally: it would also reserve space in every
  `overflow: hidden` ellipsis.
- The document pane's end padding gives the gutter back, so padded views
  stay symmetric; a walkthrough note's bleed follows it.
- Pierre's trees take the thumb token, the 10px gutter, and the square
  thumb; diffs' wide-code thumb takes the token and the square thumb.
- No JavaScript overlay scrollbars.

## Consequences

- The reserved gutter keeps content still, at the cost of 10px at each
  scroll area's edge, which VS Code's overlay does not take.
- Styling `::-webkit-scrollbar` lays out classic bars in Chromium and Safari
  even where macOS would overlay them, so Mac users see the gutter too.
- Keyboard and wheel scrolling show no thumb unless the pointer is over the
  area.
- The size is in rem, so it scales with the Font Size setting. Firefox's
  thin width is the browser's own.
