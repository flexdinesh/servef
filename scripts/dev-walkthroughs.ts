import { spawnSync } from "node:child_process"
import { existsSync } from "node:fs"
import path from "node:path"

// dev-walkthroughs publishes sample walkthroughs to the mise run dev server
// (decision 0047): one over testdata/markdown and two over the git fixture,
// covering notes, line ranges, previews, an image, a snapshot, every compare
// kind, and a step whose code changed. Fixed IDs replace them on each start.

const port = process.env.SERVEF_DEV_SERVER_PORT || "8080"
const origin = `http://127.0.0.1:${port}`
const root = path.resolve(import.meta.dirname, "..")
const library = path.join(root, "testdata/markdown")
const fixture = process.env.SERVEF_BROWSER_GIT_ROOT || path.join(root, ".dev/git-fixture")

// The server compiles with go run first, so wait up to two minutes for it.
async function waitForServer(): Promise<boolean> {
  const deadline = Date.now() + 120_000
  while (Date.now() < deadline) {
    try {
      if ((await fetch(`${origin}/api/projects`)).ok) return true
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  return false
}

// Git hooks export GIT_DIR and friends; drop them so git reads the fixture.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("GIT_")))

function commit(revision: string): string {
  const result = spawnSync("git", ["rev-parse", "--short=12", revision], { cwd: fixture, env, encoding: "utf8" })
  return result.status === 0 ? result.stdout.trim() : ""
}

function libraryTour() {
  return {
    project: library,
    id: "dev-library-tour",
    title: "A tour of the sample library",
    summary: "How `testdata/markdown` exercises each view: Markdown previews, highlighted source, a Mermaid diagram, and an image.",
    steps: [
      {
        title: "What this covers",
        body: "The library is servef's shared development and test corpus. A step with lines opens Source with the narration under the range; a step without lines opens the file's usual view with the narration above it.\n\nStep with **Ctrl+Right** and **Ctrl+Left**, or press Present.",
      },
      {
        title: "The index links every guide",
        body: "The README is the library's front page. These links are relative, so following them in Preview stays inside the project.",
        file: { path: "README.md", lines: [5, 8] },
      },
      {
        title: "A guide with a table and a fence",
        body: "Lines open Markdown in Source, under the table. Switch to Preview and the narration leads the rendered page instead.",
        file: { path: "guides/getting-started.md", lines: [7, 10] },
      },
      {
        title: "The Go sample",
        body: "A source-only family: highlighting, no preview. The `go-search-token` comment is what the content search tests look for.",
        file: { path: "code/main.go", lines: [6, 8] },
      },
      {
        title: "Two build stages",
        body: "Dockerfiles are matched by name, not extension. The first stage builds the binary:\n\n```dockerfile\nRUN go build -o /servef .\n```",
        file: { path: "code/Dockerfile", lines: [1, 4] },
      },
      {
        title: "A diagram from text",
        body: "`.mmd` files preview as themed SVG; switch to Preview for this one. It is the request path the diagrams guide describes.",
        file: { path: "guides/request-flow.mmd", lines: [1, 4] },
      },
      {
        title: "Images have no lines",
        body: "A step on an image shows the narration above it.",
        file: { path: "assets/preview.png" },
      },
      {
        title: "Written against an older base image",
        body: "This step expected Go 1.22. The file moved on, so servef says the code changed after the step was written.",
        file: { path: "code/Dockerfile", lines: [1, 1], expect: "FROM golang:1.22 AS build" },
      },
    ],
  }
}

function fixtureChanges(head: string, initial: string, lower: string) {
  return {
    project: fixture,
    id: "dev-fixture-changes",
    title: "What changed in the fixture",
    summary: "The git fixture's uncommitted edits, its feature commit, and the branch they sit on.",
    steps: [
      {
        title: "What this covers",
        body: "The fixture is rebuilt on every `mise run dev`. It has one change of every kind, so this tour visits the working tree, a commit, a branch compare, and a range.",
      },
      {
        title: "The answer went from 1 to 2",
        body: "The working tree bumps `answer`. The narration sits under the changed line in the diff.",
        diff: { compare: { kind: "working" }, path: "src/app.ts", lines: [1, 1] },
      },
      {
        title: "run returns the constant",
        body: "In the working copy `run` still hands back `answer`; only its indent changed.",
        file: { path: "src/app.ts", lines: [3, 5] },
      },
      {
        title: "The README before any edits",
        body: "Pinned to the first commit, the file opens as a read-only snapshot in its own tab.",
        file: { path: "README.md", lines: [3, 5], at: initial },
      },
      {
        title: "A note that goes away",
        body: "Deleted files only have an old side, so this step anchors to it.",
        diff: { compare: { kind: "working" }, path: "notes/removed.md", side: "old", lines: [1, 3] },
      },
      {
        title: "The feature commit",
        body: "One commit adds the history notes and edits `clean.md`.",
        diff: { compare: { kind: "commit", commit: head }, path: "docs/history.md", lines: [1, 3] },
      },
      {
        title: "Everything the branch adds",
        body: "Against `main`, the branch carries the lower layer's index too.",
        diff: { compare: { kind: "branch", base: "main" }, path: "docs/index.md", lines: [1, 3] },
      },
      {
        title: "Both layers as one range",
        body: "A range spans from the lower layer's commit to HEAD.",
        diff: { compare: { kind: "range", from: lower, to: head }, path: "clean.md" },
      },
    ],
  }
}

function fixtureShort() {
  return {
    project: fixture,
    id: "dev-fixture-short",
    title: "Where the README is going",
    summary: "A two-step walkthrough, for the list and the phone step bar.",
    steps: [
      {
        title: "Two lines changed",
        body: "The second line gains a word and a fourth line arrives.",
        diff: { compare: { kind: "working" }, path: "README.md", lines: [3, 5] },
      },
      {
        title: "The result",
        body: "The working copy as it reads now.",
        file: { path: "README.md" },
      },
    ],
  }
}

async function publish(walkthrough: { id: string; title: string }): Promise<boolean> {
  const response = await fetch(`${origin}/api/walkthroughs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(walkthrough),
  })
  if (response.ok) return true
  console.error(`dev-walkthroughs: ${walkthrough.id}: ${response.status} ${await response.text()}`)
  return false
}

async function main() {
  if (!(await waitForServer())) {
    console.error(`dev-walkthroughs: no server at ${origin}`)
    process.exitCode = 1
    return
  }
  const walkthroughs: { id: string; title: string }[] = [libraryTour()]
  const head = existsSync(path.join(fixture, ".git")) ? commit("HEAD") : ""
  if (head) walkthroughs.push(fixtureChanges(head, commit("main"), commit("docs-base")), fixtureShort())
  const published = await Promise.all(walkthroughs.map(publish))
  if (published.includes(false)) process.exitCode = 1
  else console.log(`dev-walkthroughs: published ${published.length} sample walkthroughs`)
}

await main()
