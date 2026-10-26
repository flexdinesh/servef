# 0042: Expandable commit rows

- Status: Accepted
- Date: 2026-10-26
- Supersedes: [0040](0040-source-control-panes-and-review-files-rail.md)'s branch-name chips on commit rows

## Context

Commits at stack layer tips carried branch-name chips on the row itself. The
chips crowded the subject in a narrow sidebar, and a commit's other details
(message body, author, full date, hash) showed only in a tooltip or its
review.

## Decision

- Commit rows show the subject and age only.
- A leading chevron, or Right and Left on the focused row, shows and hides a
  commit's details under it: the message body, `author <email>`, the absolute
  date, the short hash with a Copy action for the full hash, and the branches
  the commit tips (HEAD's in accent).
- Click still selects; Enter and double-click still open the review. Several
  commits can show details at once; the choice is not persisted.
- Details use data the commits list already holds; expanding sends no
  request.
- A listbox holds options only, so the details stay out of the accessibility
  tree and the row names them through `aria-describedby`. The row's
  accessible name keeps its branch names.

## Consequences

- The Copy action in the details is pointer-only; Cmd/Ctrl+C on the row still
  copies the hash from the keyboard.
