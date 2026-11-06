# 0090: Native trees widget and Pierre icons

- Status: Superseded by [0094](0094-mygo-app-uses-web-views.md)
- Date: 2026-11-05
- Relates to: [0086](0086-native-agent-tooling-and-ports.md) (trees get a
  Pierre-like native widget; stays in force), [0082](0082-native-mygo-experiment.md)
  (the native app), [0006](0006-explorer-filename-filter.md) (Pierre
  icons), [0024](0024-source-control-view-and-working-tree-diffs.md)
  (changes), [0023](0023-git-ignore-sources.md) (gitignore)

## Context

The native Explorer used mygo's `ui.Outline`: no icons, no git statuses,
and a layout unlike Pierre's. 0086 asks for a Pierre-like native widget,
generic in `packages/mygo`.

## Decision

- `packages/mygo/trees` is the widget: `Tree` (immutable data by path;
  the web tree's sort, optional compact chains, `Filter`), `State` (open
  folders, selection, and keyboard cursor kept by path, so they survive a
  new `Tree` and a filtered one; `Reset` for another project), and `View`
  over `ui.List`, its rows keyed by path. It imports nothing from srcz.
- Own rows, not `ui.Outline`: Outline puts an arrow box before every row,
  leaves, too, and fixes row padding; Pierre draws the chevron in the icon
  slot. `ui.List` keeps virtualization; the widget handles Pierre's keys
  itself and leaves the list's own choice unset, so the list's mark never
  stands for the selection. No type-ahead, as Pierre.
- The order is the web tree's path store's (`path-store/src/sort.ts`, not
  `utils/sortChildren.js`): folders first, then names compared
  case-insensitively with digit runs as numbers, then exact names; no
  group for dot names.
- Compact chains (`Flatten`) are on in the Explorer, as Pierre's
  `flattenEmptyDirectories` default: a chain is its deepest directory's
  row; opening any directory opens those above it, closing closes it
  alone, so a change that breaks a chain shows its folders open.
- Accessibility: the list is a tree; each row a tree item with its level,
  open state, and selected state (the host's open document), and the
  keyboard cursor its active item. mygo gives a List's row holders the
  list item role; the tree items sit inside them.
- Pierre's behavior, as 1.0.0-beta.6 does it: Up, Down, Home, and End move
  the cursor only; click, Enter, or Space selects the row (folders too)
  and toggles a folder; Right opens or steps in, Left closes or goes to
  the parent; git
  letters A, D, M, R, U, a dot on folders holding changes, ignored status
  inherited and dimmed, colored icons keep their hue while the name takes
  the status color; indent guides while hovered.
- Decorations are data from the host (`Decoration`: status, text parts,
  viewed, dimmed, title); matches are ranges; the context menu is a hook.
  The widget never decides file types.
- Icons are Pierre's complete set by name (exact base name, then the
  longest extension, the set's overrides first; unknown names get the
  generic icon), generated into `pierre_icons.go` by
  `packages/mygo/scripts/sync-trees-icons.mjs` from the web app's
  `@pierre/trees`, pinned by its lockfile. Pierre's Apache-2.0 LICENSE
  and NOTICE sit beside it and ship in every native package's
  `THIRD_PARTY_NOTICES.txt` (`apps/native/scripts/notices.sh`). Rerun the
  script when the web bumps Pierre.
- The native Explorer maps srcz's tokens as `explorer.css` and
  `tree-css.ts` do, and shows the statuses of the files it shows. As the
  web's `useChanges`, it reads a project's changes on load, on live
  changes (so a failed read recovers), when Source Control opens (joining
  a read in flight), and when the window regains the focus, at most once
  per 2 s.

## Consequences

- Not ported: sticky folder rows and middle truncation.
- No gitignore marks: `TreeNode` carries no ignored flag and the web
  Explorer shows none; the widget's `Ignored` status is ready for it.
- Explorer RSS with 1240 entries open is unchanged within noise (about
  56 to 59 MiB before and after).
