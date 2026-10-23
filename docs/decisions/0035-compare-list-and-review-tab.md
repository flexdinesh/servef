# 0035: Compare list and review tab

- Status: Accepted
- Date: 2026-10-23
- Extends: [0034](0034-diff-anchor-reveal-and-hunk-navigation.md) to review tabs
- Supersedes: the Source Control UI of [0024](0024-source-control-view-and-working-tree-diffs.md),
  [0027](0027-commit-history-in-source-control.md), and
  [0030](0030-branch-and-range-diffs.md); per-file diff routes of
  [0028](0028-diff-line-links-and-copy.md)

## Context

Source Control had a Changes tree, a Commits tree whose commits expanded
inline with their files, and a pinned compare row that turned into a range on
Shift-click. Every file opened its own diff tab. Choosing what to compare and
browsing its files happened in one nested tree; reviewing a branch meant
opening tabs one file at a time, with no line counts and no way to track
progress. The base branch was a fixed heuristic.

## Decision

- One list (`role="listbox"`, “Compare”) holds what can be compared:
  Uncommitted (the working tree against HEAD, with a file count), Branch vs
  base (three-dot, as 0030, with a base picker and a file count), then the
  commits of the Branch or All range (0027). Click selects; Shift-click or
  Shift+Arrow extends a contiguous range of commits from the anchor; Escape
  narrows a range to its anchor; Enter or double-click opens the review.
  Selection is not persisted; it follows the active review tab, and otherwise
  defaults to Uncommitted when the checkout is dirty, else Branch.
- A Files pane below the list, behind 0024's separator, shows the selected
  compare: a header naming it (“Uncommitted”, “vs main”, the commit's subject,
  “3 commits”), the commit's body, author, date, and hash, the branch's ahead
  count and merge-base, or the range's span with Clear; then a Pierre tree of
  its files with status letters, `+N −M` line stats, and Expand/Collapse All
  Folders. Opening a file opens or focuses the compare's review tab, scrolled
  to it. The file in view in that tab is selected in the tree.
- A review tab per compare replaces per-file diff tabs. It stacks every file's
  diff under sticky headers (toggle, path, rename origin, stats, status,
  Viewed, Open File), loading diffs as they near the viewport with a shared
  virtualizer and worker pool, and keeps at most 30 parsed diffs. The
  toolbar's Expand/Collapse All Files opens or closes every file; Open File
  moves to each file's header. Labels: “Uncommitted”, “vs main”, `3577ec6
  subject…` (40 characters), `a1b2c3d..e4f5g6h`.
- Routes drop `path`: `?review=working`, `?review=branch[&base=]`,
  `?commit=<hash>`, `?from=&to=`. The fragment `#f=<path>` names the file in
  view and may carry 0028's line anchor: `#f=src/App.tsx&R12-R20`. Legacy
  per-file URLs redirect to the review with `#f=`; stored tabs migrate.
  Uncommitted and Branch reviews reread on activation and focus; commit and
  range reviews never refetch.
- 0034 carries over per file: an anchor reveals its first line near the top
  third once its diff shows, opening the gaps it names. F7 and Shift+F7, and
  the toolbar's Previous change and Next change, step through the open
  files' changes in review order: past a file's last change, the next file
  with changes (diffs not yet shown are read first), wrapping past the
  review's ends. Collapsed and Viewed files are skipped. Only pointer
  gestures write a gutter selection; Pierre's echo of a programmatic one is
  ignored.
- Viewed marks collapse a file and persist per project, path, and blob hash in
  `servef-review-viewed` (50 projects, 2,000 files each); a new blob clears
  the mark.
- The base picker lists local and remote-tracking branches (1,000 at most)
  with a filter and a Default choice. The choice persists per project in
  `servef-source-control` and travels in the review route.
- Phones show the list or the Files pane: tapping a row drills in, Back
  returns. Long-press a commit, then tap another, to select a range.
- `GET /api/refs?project=` returns `repository`, `branch`, `defaultBase`,
  `branches` (`{name, remote}`), and `truncated`. `base=<branch>` on
  `/api/commits?range=branch`, `/api/branch-files`, and `/api/diff?branch=1`
  overrides 0027's heuristic; a name the refs do not list is 400 with `code:
  "unknownBase"`.
- `GET /api/compare-stats?project=&compare=working|branch|commit|range` (with
  `base`, `commit`, or `from`/`to`) returns `files` (`{path, status,
  additions?, deletions?, blob}`, status `counted`, `binary`, `tooLarge`, or
  `budget`) and `budgetExceeded`. `blob` is the newer side's git blob hash,
  the older side's for deletions. Lines are counted with go-diff over a line
  mapping, with a one-second limit per file. 0004's 4 MiB limit applies per
  side; one request reads at most 64 MiB, and the first file past it and
  every later one report `budget`. A timeout (20s) or unreadable side fails
  the request.

## Consequences

- Stats read both sides of every file once per compare; very large compares
  show counts only for their first files.
- A review tab holds every file of its compare; its memory is bounded by lazy
  loading and the parse cache, not by the file count.
- Viewed marks are per browser and drop once their caps evict them.
- Long-press ranges on phones are hard to discover.
- 0030's “Phones cannot select ranges yet” no longer holds.
