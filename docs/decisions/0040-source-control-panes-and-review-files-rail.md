# 0040: Source Control panes, branch stacks, and the review Files rail

- Status: Accepted; commit-row branch chips superseded by [0042](0042-expandable-commit-rows.md)
- Date: 2026-10-25
- Supersedes: the compare list and Files pane of [0035](0035-compare-list-and-review-tab.md)
- Extends: [0030](0030-branch-and-range-diffs.md) branch compares with a head branch

## Context

0035 put Uncommitted, Branch vs base, and commits in one "Compare" list over
a Files pane. One list mixed three kinds of things; selecting a row changed
the Files pane below, so where files appeared depended on what was selected
last. A stack of branches had no place, and the branch compare always started
at HEAD.

## Decision

- Source Control stacks three panes in a fixed order: Uncommitted, Commits,
  Branches. Each has a 22px header that opens and closes it. Empty panes start
  closed (no changes, no commits in the range, no stack layers) and open when
  content arrives; once the user toggles a pane, that choice persists. On
  desktop open panes split the height by weight with sashes between them
  (arrow keys step 5%, double-click resets); phones stack them and the view
  scrolls.
- Uncommitted shows a count badge and lists changes flat (name, muted folder,
  `+N −M`, status letter) or as a tree; the choice persists, flat by default.
  Opening a file opens the Uncommitted review at that file. Uncommitted
  reviews have no Files rail: the pane is their file list.
- Commits lists the branch range (`base..HEAD`) or all history. When HEAD is
  the base branch (its name, or `origin/<name>`), it lists all history and the
  Branch/All toggle is disabled. Click selects; Shift-click or Shift+Arrow
  extends a range; Escape narrows it. Diffs open deliberately: a row's Diff
  action shows on hover and focus, stays on the selection, and on a range sits
  on its newest commit only, opening the combined range diff; Enter and
  double-click open too. Commits at stack layer tips carry branch-name chips.
- Branches shows the stack: local branches whose tips lie in `base..HEAD`
  (below HEAD) or whose `base..tip` contains HEAD (above it), HEAD's layer, and
  the base at the merge-base. Each layer's parent is the nearest layer it
  contains, else the base; branches on one commit share a row. A layer offers
  "Diff layer" (its own commits, against its parent layer; absent for the
  lowest) and "Diff vs base" (everything since the base). The base picker
  moves here.
- Branch compares take an optional head, a local branch: `head=` on
  `/api/branch-files`, `/api/diff?branch=1`, and
  `/api/compare-stats?compare=branch`; an unlisted head is 400 with
  `code: "unknownHead"`. Routes add `&head=`; tabs read `feat-b vs feat-a`.
  HEAD's own layer keeps `head` empty, so its review is the live branch review
  of 0035.
- `GET /api/branch-stack?project=&base=` returns `repository`, `branch`,
  `base`, `mergeBase`, `head`, `layers` (`{name, aliases, head, parent, ahead,
  current}`, top to bottom), and `truncated`. Nothing ahead of the base lists
  no layers. At most 100 local branches are checked, sharing one walk budget
  and the request's 20s timeout; past either, `truncated`. A detached HEAD's
  layer has an empty name and is never a parent.
- Commit, range, and branch reviews show a Files rail beside the diffs: full
  height, resizable from its leading edge (12–32rem, persisted), and hidden or
  shown from the toolbar's Files toggle (persisted). It holds the compare's
  tree with line stats; the file in view is selected. On phones the toolbar's
  Files button opens a bottom sheet instead; choosing a file jumps there and
  closes it.
- Split needs 48rem in the diff area itself, so a wide rail or sidebar forces
  Unified as narrow windows do.

## Consequences

- Commit details (body, author, hash) show in the review header and row
  tooltips only; selecting a commit no longer previews its files.
- Every stack candidate costs a bounded walk; repositories with many old
  branches may report `truncated`.
- Non-linear stacks (siblings, merges) list each branch against its nearest
  contained layer, which is correct but not a single chain.
- Long-press ranges on phones remain hard to discover.
