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

## 1. Find the project and the format

- `project` is the absolute path of the repository or worktree root: run
  `git rev-parse --show-toplevel`. A plain folder outside any repository
  works too. Folders inside a repository are rejected.
- `servef walkthrough schema` prints the JSON Schema. Read it once if
  anything below is unclear; it is authoritative.
- No running server is needed to publish.

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
servef walkthrough publish walkthrough.json
```

Or pass the JSON on standard input with `-`:

```sh
servef walkthrough publish - <<'EOF'
{"project": "…", "title": "…", "steps": […]}
EOF
```

For example:

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

- Reuse `id` to revise a walkthrough (it replaces) rather than publishing a
  new one. Without `id`, servef derives one from the title.
- servef prints `published <id> (created|replaced) for <project>`, then the
  links when the desktop app or a running servef shows the project:
  `desktop: servef://local/view?…` (desktop app on this machine), or
  `web: http://…` and `desktop: servef://…` for a running server. Give the
  user both; over SSH they open on the user's computer. `--json` prints
  `{"id", "project", "created", "url", "desktopUrl"}` instead (`url` only
  when something shows it; `desktopUrl` only with an `http://` url).
  `"open": true` opens the url in the app or their browser unless
  `--no-open` is passed or the machine has no UI (SSH, or Linux without a
  display). With no running servef, it prints how to view the walkthrough
  and exits 0.
- Exit 1 lists every problem as `field: message`, for example
  `steps[3].diff.path: is not changed in this compare`. Fix them all and
  publish again; servef checks every path, line, and commit against the
  repository. `--depth` (default 5) bounds how deep working-tree files may
  be.
- Fallback: if `servef walkthrough` is an unknown command (an older servef)
  and you know a running server's URL, post the same JSON:

  ```sh
  curl -q -sS -X POST -H 'Content-Type: application/json' \
    --data @walkthrough.json "$SERVEF/api/walkthroughs"
  ```

  `-q` skips `~/.curlrc`; settings such as `continue-at` break uploads. 201
  creates, 200 replaces, the response is `{"id", "url", "created"}`, and a
  400 lists every problem.
- Limits: 200 steps, 32 KiB per body, 200 characters per title, 1 MiB per
  request, 100 walkthroughs per project (the least recently updated go
  first).

## 5. Check it

Open the printed URL and step through once if you can; a narration that
sits after the wrong lines means the range is off. Optionally,
`GET /api/walkthrough?project=…&id=…` on a running servef returns what it
stored, with each body rendered.
