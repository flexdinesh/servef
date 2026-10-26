---
name: servef-walkthrough
description: Explain code, a pull request, a branch, or a feature as a servef walkthrough, an ordered tour of notes, file line ranges, files at a commit, and diffs with Markdown narration that servef shows beside the code. Use when the user asks for a walkthrough or tour, or wants a change or feature explained in servef.
---

# servef walkthroughs

A walkthrough is an ordered list of steps. Each step has a title, Markdown
narration, and at most one target:

- **note**: no target. It shows in the walkthrough's own tab.
- **file**: a line range in the working tree, or in a commit with `at`.
- **diff**: a file and line range in a compare: uncommitted changes, a
  branch, a commit, or a commit range.

servef opens each target in an ordinary tab, selects the lines, and inserts
the narration right after them. The user steps with Ctrl/Cmd+Right and
Ctrl/Cmd+Left.

## 1. Find servef and the project

- If the user gave a servef URL, use its origin. Otherwise probe ports 7971
  to 7980 for `GET /api/walkthrough-schema` and use the first that answers
  200.
- `project` is the absolute path of the repository or worktree root (use
  `git rev-parse --show-toplevel`), or the folder servef serves. Folders
  inside a repository are rejected. `GET /api/projects` lists what servef
  serves.
- Read `GET /api/walkthrough-schema` once if anything below is unclear; it is
  authoritative.

## 2. Plan the story

Read the code first. Then tell one story in reading order, not file order:

1. A note: what the reader will understand by the end, and why it matters.
2. The entry point: where the behavior starts (route, command, handler).
3. The path through the code, one idea per step.
4. The edges: errors, limits, and what changed.
5. Optionally, a closing note: what to read next or how to verify it.

Guidelines:

- 5 to 15 steps. Split a step that needs two ideas; drop a step that only
  says "this file exists".
- Ranges of at most about 30 lines, covering exactly the code the narration
  talks about.
- 40 to 120 words per body. Say *why* and *what to notice*; do not restate
  the code. Use `inline code` for names, and links to other files as relative
  Markdown links.
- Step titles are short statements ("Refresh retries once"), not labels
  ("refresh.go").

## 3. Choose targets

| Need | Target |
| --- | --- |
| Explain current code | `"file": {"path": "src/auth.go", "lines": [40, 58]}` |
| Code that must not drift | add `"at": "<commit>"` to a file target |
| Uncommitted work | `"diff": {"compare": {"kind": "working"}, "path": …, "lines": …}` |
| A pull request or branch | `"diff": {"compare": {"kind": "branch", "base": "main", "head": "feature"}, …}` |
| One commit | `"diff": {"compare": {"kind": "commit", "commit": "<hash>"}, …}` |
| Several commits | `"diff": {"compare": {"kind": "range", "from": "<hash>", "to": "<hash>"}, …}` |
| A whole compare, no file | a diff target with only `compare` |

- Paths are project-relative with forward slashes. Lines are `[start, end]`,
  1-based and inclusive; omit `lines` to open the file without a selection.
- Diff lines count on the new side by default; set `"side": "old"` to point
  at removed lines by their old line numbers. Renamed files accept either
  path.
- For a branch, `base` defaults to the detected base branch and `head` to the
  checked-out branch; name both for a pull request. Commit hashes may be
  abbreviated if unique.
- Working-tree file steps and working or branch diff steps record the start
  of their first line as `expect` automatically. When that line changes,
  servef warns that the step may not match. Pin to a commit when the story
  must stay exact.

## 4. Publish

```sh
curl -q -sS -X POST -H 'Content-Type: application/json' \
  --data @walkthrough.json "$SERVEF/api/walkthroughs"
```

`-q` skips `~/.curlrc`; settings such as `continue-at` break uploads.

```json
{
  "project": "/home/me/src/app",
  "id": "session-refresh",
  "title": "How login sessions refresh",
  "summary": "From the cookie check to the retrying token refresh this branch adds.",
  "open": true,
  "steps": [
    {"title": "What you'll see", "body": "Requests carry a session cookie; when it expires, servers refresh it once before failing."},
    {"title": "Every request checks the session", "body": "`requireSession` runs before every handler…", "file": {"path": "auth/session.go", "lines": [40, 58]}},
    {"title": "Refresh now retries once", "body": "The branch wraps the call in one retry…", "diff": {"compare": {"kind": "branch", "base": "main"}, "path": "auth/refresh.go", "lines": [12, 30]}}
  ]
}
```

- 201 creates, 200 replaces: reuse `id` to revise a walkthrough rather than
  publishing a new one. Without `id`, servef derives one from the title.
- The response is `{"id", "url", "created"}`. Give the user the `url`.
  `"open": true` opens it in their browser unless servef runs with
  `--no-open`.
- A 400 lists every problem as `field: message`, for example
  `steps[3].diff.path: is not changed in this compare`. Fix them all and post
  again; servef checks every path, line, and commit against the repository.
- Limits: 200 steps, 32 KiB per body, 200 characters per title, 1 MiB per
  request, 100 walkthroughs per project (the least recently updated go
  first).

## 5. Check it

`GET /api/walkthrough?project=…&id=…` returns what servef stored, with each
body rendered. Open the URL and step through once if you can; a narration
that sits after the wrong lines means the range is off.
