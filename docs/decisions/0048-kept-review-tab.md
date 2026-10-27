# 0048: The last review tab stays mounted behind other tabs

- Status: Accepted
- Date: 2026-10-27
- Extends: [0035](0035-compare-list-and-review-tab.md)

## Context

A review tab rendered only while active. Opening a file from it (Open File)
or switching to any other tab unmounted it; returning remounted it at the
top of its stack, with its sections re-rendered from the parse cache or
reread, its collapsed files reopened, its line selection and `#f=` fragment
dropped, and the Files rail rebuilt. Reviewing a branch meant finding your
place again after every look at a file's source.

## Decision

- The last active review stays mounted, hidden with React's `<Activity
  mode="hidden">`, while another tab is active: its diffs, open and
  collapsed files, selection, scroll, and Files rail keep their state. Hidden,
  its effects stop: no scroll tracking, F7, virtualizer, or near-viewport
  reads. One review is kept; activating another review replaces it, and
  closing its tab releases it.
- Returning to it, from its tab or Source Control, routes back to the
  fragment it last showed, and it restores its own scroll, since the
  document pane scrolled other documents meanwhile. A fragment it already
  revealed pins again only if the user had not scrolled away from it.
- While a navigation is pending, the location already carries the next
  route's hash; the review keeps the fragment it last resolved until the
  route resolves.
- 0035's rereads still apply: live reviews reread their list on activation,
  and diffs only when the list read changes.

## Consequences

- Returning to a review costs no diff requests unless its compare changed.
- The kept review's DOM and the diff worker pool stay in memory while
  another tab is active, bounded to one review.
- Line selections survive a round trip to another tab.
