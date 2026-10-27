# 0051: Branch review with uncommitted changes

- Status: Accepted
- Date: 2026-10-27
- Extends: [0030](0030-branch-and-range-diffs.md), [0035](0035-compare-list-and-review-tab.md), and
  [0040](0040-source-control-panes-and-review-files-rail.md)

## Context

Source Control reviews the working tree against HEAD (Uncommitted) and the
branch's commits against its merge-base with the base (`vs main`). Work in
progress spans both: seeing everything a branch will change once committed
meant reading two reviews and combining them mentally. That compare is
`git diff $(git merge-base main HEAD)`: the merge-base's tree against the
working tree.

## Decision

- HEAD's branch review (no `head`) carries a scope switch in its header,
  between the meta line and totals: a small `SegmentedControl` reading
  **Committed** | **+ Uncommitted N**, N being the Uncommitted pane's file
  count. Committed is 0030's compare. + Uncommitted compares the merge-base
  with the index and working tree. Stack layers (`head` set) never show the
  switch: their worktree is not checked out.
- The review always opens on Committed, as a pull request shows it. The
  scope lives in the route only, `&uncommitted=1`, and in the remembered tab
  (`{review: "branch", base, uncommitted: true}`); it is not a preference.
- Switching replaces the tab in place, keeps the `#f=` file in view when the
  other scope lists it, and is replaced as the kept review (0048). The tab
  reads `vs main + uncommitted`; its heading “Changes on this branch since it
  left main, with uncommitted changes”.
- With nothing uncommitted, + Uncommitted stays visible and `aria-disabled`,
  titled “No uncommitted changes”. While included it stays enabled, so the
  review can switch back after the checkout goes clean.
- While included, files the Uncommitted list holds (path or a rename's old
  path) carry an accent dot, “Has uncommitted changes”, beside the status
  letter in file headers. Pierre's Files rail decorations are text or an
  icon, never both, and its CSS is fixed at creation, so rail rows show an
  accent-colored “●” text part before the line stats instead.
- The reference tree is the merge-base when HEAD is ahead, HEAD itself when
  nothing is ahead (the result equals Uncommitted), and an empty tree for
  unrelated histories. A file the branch changed and the working tree
  restored drops out. Statuses, renames, untracked files, ignore rules, and
  warnings follow 0024's status read against that tree.
- `uncommitted=1` on `/api/branch-files`, `/api/diff?branch=1`, and
  `/api/compare-stats?compare=branch` selects the compare; with `head`, or
  any value but `1`, it is 400. Lists cap at 5,000 files as `/api/changes`
  does. `/api/branch-files` gains `warnings`, `[]` without the working tree,
  and may list `untracked` files. Original sides read from the reference
  commit's blobs, modified sides from the worktree through
  `files.ResolveFile`, only for paths validated against the change set; a
  path outside it is 404 with `code: "unchanged"`. 0004 limits and 0035's
  stats budget apply.
- The review is live: its list, stats, and diffs reread with the branch and
  with every Uncommitted read, including live updates (0050), so files the
  working tree adds or restores join or leave it.
- Walkthrough targets (0047) stay committed-only.

## Consequences

- Switching scope rereads every diff, since diffs cache per compare.
- Viewed marks key on blob hashes, so files whose working copy differs from
  the commit lose their mark in the other scope.
- The rail's dot is a text glyph, not a drawn icon, until Pierre supports a
  decoration with both an icon and text.
