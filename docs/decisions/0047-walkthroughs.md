# 0047: Walkthroughs

- Status: Accepted
- Date: 2026-10-26
- Extends: [0008](0008-line-links-and-copy-path.md), [0028](0028-diff-line-links-and-copy.md),
  [0034](0034-diff-anchor-reveal-and-hunk-navigation.md), [0035](0035-compare-list-and-review-tab.md)

## Context

Agents explain changes and features in chat, pointing at `path:12-20` and
diffs the reader must find by hand. servef already links lines (0008), diff
ranges (0028), and review files (`#f=`, 0035), and reveals them (0034). A
reader wants one ordered story: open each place in turn, highlighted, with
the agent's explanation beside it.

## Decision

- A walkthrough is a titled list of steps, each with a title and Markdown
  narration. A step targets a working-tree file (optionally lines), a file at
  a commit (`at`), a diff in a compare (working, branch with base and head,
  commit, or range; optionally a path, a side, and lines), or nothing (a
  note).
- Agents publish with `POST /api/walkthroughs` (`Content-Type:
  application/json`, at most 1 MiB). The model, validation, and store live in
  `internal/walkthrough`, behind a `Service` every transport shares; HTTP is
  the first adapter, so MCP or WebMCP can follow without changing the model.
  `GET /api/walkthrough-schema` serves the JSON Schema.
- Decoding is strict: unknown fields are rejected. Every problem is reported
  at once, by field (`steps[3].diff.path: is not changed in this compare`), as
  one `errorResponse` with `code: "invalidWalkthrough"`.
- Targets are checked as the browser will open them: working-tree files
  through the scanned index (CLI scope), commit files and diffs through git
  (repository-scoped, as Source Control). Abbreviated hashes (4+ digits) are
  resolved and stored in full; ambiguous ones are rejected. Lines must fall
  within the file or diff side under 0004's limits.
- A step may carry `expect`, the trimmed text its first line starts with.
  When omitted, the server records it for targets that drift (working-tree
  files and working or branch diffs); commit targets never drift. The browser
  flags a step whose line no longer starts with it.
- Limits: 200 steps, 200-character titles, 32 KiB per body and summary, 100
  walkthroughs per project (the least recently updated are evicted).
- Walkthroughs persist in the user cache (`servef/walkthroughs/<hash of the
  project>/<id>.json`, 0700 directories, 0600 files, atomic replace), never
  in the project. Posting an existing ID replaces it and keeps its creation
  time; a missing ID is generated from the title.
- Writes are guarded against other sites in the browser, not other local
  processes: POST requires JSON, which a cross-site page cannot send without
  a preflight servef never approves, and POST and DELETE refuse any Origin
  that is not the request's own host (`code: "foreignOrigin"`).
- `open: true` opens the walkthrough's URL in the browser, unless servef runs
  with `--no-open` (or over SSH), as startup does. The response always
  returns the URL: `/view?project=…&wt=<id>`.
- `GET /api/walkthroughs?project=` lists summaries, newest first;
  `GET /api/walkthrough?project=&id=` returns one with `summaryHTML` and each
  step's `bodyHTML`, rendered as Markdown previews are (HTML escaped, local
  links rewritten from the project root); `DELETE /api/walkthroughs?project=&id=`
  removes one.
- The browser shows walkthroughs in a Walkthroughs activity-bar view and a
  walkthrough tab (cover, notes, outline). Steps open the ordinary file or
  review tabs, highlighted through the existing anchors, with the narration
  as a card under the range. Ctrl/Cmd+Right and Ctrl/Cmd+Left step; present
  mode hides the chrome; there is no timed autoplay.

## Consequences

- Walkthroughs are per machine and not shared through git.
- Working-tree and branch steps drift as code changes; they are flagged, not
  re-anchored.
- Any local process can publish or delete walkthroughs; narration HTML is
  always escaped.
- The list is polled while the page is visible.
