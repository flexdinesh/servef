# 0030: Branch and commit-range diffs

- Status: Accepted; pinned row, selection, and diff tabs superseded by [0035](0035-compare-list-and-review-tab.md); head branches added by [0040](0040-source-control-panes-and-review-files-rail.md)
- Date: 2026-10-22
- Extends: [0027](0027-commit-history-in-source-control.md)

## Context

Commits (0027) diffs one commit at a time. Reviewing a branch before a pull
request, or a run of an agent's commits, meant opening every commit's files
and combining them mentally. servef never runs `git` (0023).

## Decision

- A pinned row leads Commits. In the Branch range, with commits ahead of a
  base, it reads “All changes vs main” with a file-count badge and lists the
  net changes from the merge-base of the base and HEAD to HEAD, as a pull
  request's Files changed does (`git diff base...HEAD`). Nothing ahead, no
  base, or HEAD on the base shows no row. The base is 0027's.
- Shift-click or Shift+Arrow on commit rows selects a contiguous run from an
  anchor (the last commit clicked or reached without Shift), in Branch and
  All. The pinned row then reads “3 commits” with the span
  `a1b2c3d..e4f5g6h` and a Clear action, opens, and lists the net changes from
  the first parent of the oldest selected commit (an empty tree for a root or
  shallow commit) to the newest (`git diff oldest^ newest`). Only the two
  endpoint trees count: across merges in All, changes of unselected commits
  between them are included. Plain click still toggles a commit. Escape,
  Clear, switching range or project, or a refresh that drops either end
  clears the selection; it is not persisted. Phones show the branch row but
  select no ranges.
- The pinned row expands like a commit: a details row (“3 commits ·
  merge-base a1b2c3d”, or the span), then files under compact folders with
  the same statuses, keyboard, and states. Collapse All Commits closes it.
- Files open per-file diff tabs. Branch tabs read `App.tsx (vs main)`
  (`(Branch)` until the base is known), route as `&branch=1`, are remembered
  as `{branch: path}`, and reread on activation and window focus, so new
  commits show. Range tabs read `App.tsx (a1b2c3d..e4f5g6h)`, oldest to
  newest inclusive, route as `&from=&to=`, are remembered as `{from, to,
  path}`, and never refetch on focus. A branch tab whose file the branch no
  longer changes shows “No changes to <path> vs main.” with Open File.
- The merge-base comes from 0027's bounded `base..HEAD` walk: its boundary
  holds every best common ancestor; candidates reachable from another drop
  out, and of several (criss-cross) the newest committer date wins. The
  walk's caps, context, and 20s timeout apply; go-git's unbounded
  `MergeBase` is not used. Unrelated histories compare with an empty tree.
- `GET /api/branch-files?project=` returns `repository`, `branch`, `base`,
  `mergeBase` (`""` while nothing is ahead or without a base), `head`,
  `ahead`, `files`, and `truncated`. `GET /api/range-files?project=&from=&to=`
  returns `from`, `to`, `files`, and `truncated`. `/api/diff` accepts
  `branch=1` or `from=&to=`; its response shape is unchanged; `commit`,
  `branch`, and `from`/`to` are mutually exclusive. Paths are validated
  against the compare's change set; a branch path outside it is 404 with
  `code: "unchanged"`. Lists cap at 1,000 files; renames are exact (0027);
  0004 limits apply per side. History stays repository-scoped.
- Branch files reread when the branch range's head, base, or ahead count
  changes; range files are read once per range.

## Consequences

- A range across merges in All can include changes from commits outside the
  selection.
- The branch row inherits 0027's base heuristic, which may differ from a pull
  request's target; criss-cross histories pick one merge-base.
- Live branch tabs cost a reread on focus.
- Phones cannot select ranges yet.
