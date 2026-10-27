# 0053: Viewed marks in the review Files rail

- Status: Accepted
- Date: 2026-10-27
- Extends: [0040](0040-source-control-panes-and-review-files-rail.md)

## Context

A review counts its Viewed files (“3 of 12 viewed”), but the Files rail drew
every file alike, so finding what was left meant scanning the diffs. Pierre's
tree has no per-row state hook: a row decoration is text or one icon, and
its `unsafeCSS` is fixed when the tree mounts.

## Decision

- The rail and its phone sheet mark Viewed files: a 12px lucide check (the
  Viewed toggle's icon) before the line stats, in the lane's muted ink, and
  “Viewed” in the stats' title. Unless it is the selected file in view, a
  viewed row's name takes `text-muted` and its icon 0.6 opacity; the status
  letter keeps its hue.
- The name dims to `text-muted`, not by opacity: muted holds 4.5:1 in every
  theme, while a dimmed status hue drops below it in most light themes (and
  in Miasma at any opacity).
- The check is a CSS mask of the lucide SVG, not a text glyph: the bundled
  Inter subsets lack U+2713, so a glyph falls back to a system font.
- A pnpm patch adds `setUnsafeCSS` to `@pierre/trees`' `FileTree`; the tree
  rewrites its own style element with per-path rules for viewed rows. No
  other Pierre behavior changes.
- 0051's uncommitted dot stays an accent text part; a viewed, marked row
  reads check, dot, then stats.
- The Uncommitted pane's tree and list stay unmarked: they are the live change
  list, not a review's progress.

## Consequences

- Upgrading `@pierre/trees` means re-applying or dropping the patch.
- Screen readers hear “viewed” only through the stats' title; Pierre names
  rows itself.
- Viewed rows lose their status tint on the name; the status letter and
  check carry the state, never color alone.
