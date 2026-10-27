// agentExample is the walkthrough the copied instructions show; its fields
// must stay within the server's JSON Schema (decision 0049).
export function agentExample(project: string) {
  return {
    project,
    id: "session-refresh",
    title: "How login sessions refresh",
    summary: "From the cookie check to the retrying token refresh this branch adds.",
    open: true,
    steps: [
      { title: "What you'll see", body: "Requests carry a session cookie; when it expires, servers refresh it once before failing." },
      { title: "Every request checks the session", body: "`requireSession` runs before every handler…", file: { path: "auth/session.go", lines: [40, 58] } },
      { title: "The token at release", body: "Pinned so the story stays exact.", file: { path: "auth/token.go", lines: [12, 20], at: "a1b2c3d" } },
      { title: "Refresh now retries once", body: "The branch wraps the call in one retry…", diff: { compare: { kind: "branch", base: "main", head: "feature" }, path: "auth/refresh.go", lines: [12, 30] } },
      { title: "The old error path", body: "Removed lines count on the old side.", diff: { compare: { kind: "commit", commit: "e4f5a6b" }, path: "auth/refresh.go", side: "old", lines: [5, 9] } },
    ],
  }
}

// agentInstructions is the prompt the Walkthroughs view copies (decision
// 0049): where servef runs, how to prepare a walkthrough, its format, and
// where to publish it. It ends with "Walk me through: " for the user's topic.
export function agentInstructions(origin: string, project: string): string {
  // One step per line keeps the example short enough to read.
  const { steps, ...fields } = agentExample(project)
  const example = JSON.stringify(fields, null, 2)
    .replace(/\n}$/, `,\n  "steps": [\n${steps.map((step) => `    ${JSON.stringify(step)}`).join(",\n")}\n  ]\n}`)
  return `Write a servef walkthrough of the topic at the end, and publish it to my running servef.

servef: ${origin}
Project: ${project}
Publish: POST ${origin}/api/walkthroughs (Content-Type: application/json)
Format: GET ${origin}/api/walkthrough-schema (JSON Schema; authoritative if anything below is unclear)

## What a walkthrough is

An ordered tour: a title, an optional Markdown summary, and steps. Each step has a title, a Markdown body, and at most one target:

- none: a note, shown in the walkthrough's own tab.
- "file": {"path", "lines": [start, end]}, a line range in the working tree; add "at": "<commit>" for the file at a commit.
- "diff": {"compare", "path", "lines", "side"}, a file in a compare. compare is {"kind": "working"} (uncommitted), {"kind": "branch", "base", "head"} (a pull request), {"kind": "commit", "commit"}, or {"kind": "range", "from", "to"}. Lines count on the new side; "side": "old" points at removed lines. A diff with only "compare" opens the whole compare.

Paths are project-relative with forward slashes. Lines are 1-based and inclusive; omit "lines" to open the file without a selection. Commit hashes may be abbreviated if unique.

## Prepare it

1. Read the code first. Tell one story in reading order, not file order: a note on what I'll understand by the end, the entry point, the path through the code one idea per step, the edges (errors, limits, what changed), and optionally a closing note.
2. 5 to 15 steps. Ranges of at most about 30 lines, covering exactly the code the body talks about.
3. 40 to 120 words per body: say why and what to notice; do not restate the code. Use \`inline code\` for names.
4. Step titles are short statements ("Refresh retries once"), not labels ("refresh.go").
5. Working-tree and branch steps drift as code changes; pin a file to a commit with "at" when the story must stay exact.

## Publish it

Write the JSON to walkthrough.json, then:

curl -q -sS -X POST -H 'Content-Type: application/json' --data @walkthrough.json ${origin}/api/walkthroughs

\`\`\`json
${example}
\`\`\`

- 201 creates, 200 replaces. Reuse "id" to revise rather than publishing again; omit it to derive one from the title.
- The response is {"id", "url", "created"}. Give me the url; "open": true also opens it in my browser.
- A 400 lists every problem as "field: message", e.g. "steps[3].diff.path: is not changed in this compare". Fix them all and post again.
- Limits: 200 steps, 200-character titles, 32 KiB per body and summary, 1 MiB per request.

Walk me through: `
}
