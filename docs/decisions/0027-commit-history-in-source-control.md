# 0027: Commit history in Source Control

- Status: Accepted
- Date: 2026-10-22
- Extends: [0024](0024-source-control-view-and-working-tree-diffs.md)

## Context

Source Control (0024) shows uncommitted changes only. Reviewing what a branch
already committed, such as an agent's work before a pull request, meant
switching to another tool. servef never runs `git` (0023).

## Decision

- Source Control stacks two collapsible sections: Changes, then Commits. On
  desktop a horizontal separator resizes them while both are open (arrow keys
  step 5%, double-click resets to an even split); on phones they stack with no
  separator. Collapsed state, the split (15% to 85%), and the commit range
  persist in this browser under `servef-source-control`.
- Commits lists the branch range by default: commits on HEAD that the base
  branch does not have (`git log base..HEAD`). The base is the first that
  exists of `origin/HEAD`'s target, `main`, `master`, `origin/main`,
  `origin/master`. A [Branch][All] toggle switches to HEAD's whole history.
  The header reads “N ahead of <base>” in the branch range. With nothing ahead,
  or no base, the section says so and offers “Show all history”.
- Order follows `git log`: newest committer date first. The branch range
  follows git's `limit_list`: commits reachable from the base are marked
  uninteresting, and the walk stops a few steps after only uninteresting ones
  remain. Walks are bounded: 10,000 branch commits and 100,000 visited
  commits, checked against the request context and a 20s timeout. Past a cap
  the branch range fails; All still lists and reports `ahead` 0.
- Pages hold 50 commits; Load more fetches the next page of the same walk by
  pinning its head. A refresh that finds the same head, base, ahead count,
  and first commits keeps the pages already loaded.
- Each commit expands, independently, to its message body (clamped to four
  lines with More/Less), author, date, and short hash with copy, then its
  changed files under compact folders. Statuses are `modified`, `added`,
  `deleted`, and `renamed`. Files compare the commit with its first parent
  (merges included; an empty tree for a root commit). Renames are exact
  content and mode matches only, as in 0024. File lists cap at 1,000 and are
  read once per commit; commits never change.
- Opening a file opens a diff tab, exactly as Changes does: `name (3577ec6)`,
  URL `&commit=<hash>`, remembered as `{commit, path}`, restored without
  checking the tree. The diff toolbar is shared; Open File opens the working
  copy and is disabled when the tree no longer has the file. Commit diffs do
  not refetch on focus.
- `GET /api/commits?project=&range=branch|all&head=&skip=&limit=` returns
  `repository`, `branch`, `base`, `range`, `head`, `ahead`, `commits` (`hash`,
  `parents`, `subject`, `body`, `author` {`name`, `email`}, `authorTime` in
  RFC 3339), and `more`. `GET /api/commit-files?project=&commit=` returns
  `commit`, `files`, and `truncated`. `GET /api/diff` accepts `commit=`; its
  response shape is unchanged. Without `oldPath`, a rename destination
  resolves its old path from the commit. Hashes are full 40-character hex.
- History is repository-scoped like 0024: CLI scope does not apply. Both diff
  sides are git blobs; 0004 limits apply per side.
- Commits refresh on project load, view activation, window focus or
  visibility (throttled to 2s), range change, and Refresh, which rereads
  Changes and Commits together. No polling.

## Consequences

- The base branch is a heuristic and may differ from a pull request's real
  target.
- A renamed and edited file shows as a deletion plus an addition.
- Very large or unrelated histories can exceed the branch caps; the section
  then shows an error with Retry.
- Signatures, tags, refs on rows, committer identity, and per-file line
  stats are not shown.
