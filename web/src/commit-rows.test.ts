import assert from "node:assert/strict"
import test from "node:test"

import {
  commitAge, commitRows, compareRowKey, folderKey, parentRowKey, rangeSelection, selectedCommits, tabRowKey, type CommitFiles,
  type PinnedCompare,
} from "./commit-rows.ts"
import type { Commit, CommitFile } from "./commits-contract.ts"
import { branchTab, commitTab, diffTab, documentTab, rangeTab } from "./workspace-tabs.ts"

function commit(hash: string, subject: string): Commit {
  return {
    hash: hash.repeat(40).slice(0, 40), parents: [], subject, body: "",
    author: { name: "servef", email: "servef@example.com" }, authorTime: "2026-10-21T14:02:00+11:00",
  }
}

const first = commit("a", "fix: first")
const second = commit("b", "feat: second")

function ready(hash: string, files: CommitFile[]): [string, CommitFiles] {
  return [hash, { kind: "ready", response: { files, truncated: false } }]
}

function labels(rows: ReturnType<typeof commitRows>): string[] {
  return rows.map((row) => {
    if (row.kind === "commit") return `${row.expanded ? "v" : ">"} ${row.commit.subject}`
    if (row.kind === "details") return `  details ${row.files?.kind ?? "none"}`
    if (row.kind === "compare") return `${row.expanded ? "v" : ">"} compare ${row.compare.kind}`
    if (row.kind === "compare-details") return `  compare details ${row.files?.kind ?? "none"}`
    const indent = "  ".repeat(row.depth)
    return row.kind === "folder" ? `${indent}${row.expanded ? "v" : ">"} ${row.name}/` : `${indent}${row.name} ${row.file.status}`
  })
}

test("collapsed commits show one row each", () => {
  assert.deepEqual(labels(commitRows([first, second], new Set(), new Map(), new Set())), ["> fix: first", "> feat: second"])
})

test("expanded commits show details, then folders before files with single-child folders joined", () => {
  const files = new Map([ready(first.hash, [
    { path: "README.md", status: "modified" },
    { path: "internal/lsp/manager.go", status: "modified" },
    { path: "internal/lsp/crash.go", status: "added" },
    { path: "web/src/App.tsx", status: "deleted" },
    { path: "web/index.html", status: "renamed", oldPath: "index.html" },
  ])])
  assert.deepEqual(labels(commitRows([first, second], new Set([first.hash, second.hash]), files, new Set())), [
    "v fix: first",
    "  details ready",
    "    v internal/lsp/",
    "      crash.go added",
    "      manager.go modified",
    "    v web/",
    "      v src/",
    "        App.tsx deleted",
    "      index.html renamed",
    "    README.md modified",
    "v feat: second",
    "  details none",
  ])
})

test("collapsed folders hide their files for that commit only", () => {
  const changed: CommitFile[] = [{ path: "src/a.ts", status: "modified" }, { path: "b.md", status: "added" }]
  const files = new Map([ready(first.hash, changed), ready(second.hash, changed)])
  const rows = commitRows([first, second], new Set([first.hash, second.hash]), files, new Set([folderKey(first.hash, "src")]))
  assert.deepEqual(labels(rows), [
    "v fix: first", "  details ready", "    > src/", "    b.md added",
    "v feat: second", "  details ready", "    v src/", "      a.ts modified", "    b.md added",
  ])
})

test("loading and failed file lists show only details", () => {
  const files = new Map<string, CommitFiles>([[first.hash, { kind: "loading" }], [second.hash, { kind: "error", message: "No" }]])
  assert.deepEqual(labels(commitRows([first, second], new Set([first.hash, second.hash]), files, new Set())),
    ["v fix: first", "  details loading", "v feat: second", "  details error"])
})

test("ArrowLeft moves from a file to its folder, and from a folder or details to its commit", () => {
  const files = new Map([ready(first.hash, [{ path: "src/a.ts", status: "modified" }, { path: "b.md", status: "added" }])])
  const rows = commitRows([first], new Set([first.hash]), files, new Set())
  assert.deepEqual(rows.map((_, index) => parentRowKey(rows, index)), [
    "", `commit:${first.hash}`, `commit:${first.hash}`, `folder:${first.hash}:src`, `commit:${first.hash}`,
  ])
})

test("commit ages read compactly and in words", () => {
  const now = Date.parse("2026-10-21T12:00:00Z")
  const age = (iso: string) => commitAge(iso, now)
  assert.deepEqual(age("2026-10-21T11:59:30Z"), { short: "now", long: "just now" })
  assert.deepEqual(age("2026-10-21T13:00:00Z"), { short: "now", long: "just now" })
  assert.deepEqual(age("2026-10-21T11:55:00Z"), { short: "5m", long: "5 minutes ago" })
  assert.deepEqual(age("2026-10-21T09:00:00Z"), { short: "3h", long: "3 hours ago" })
  assert.deepEqual(age("2026-10-20T12:00:00+00:00"), { short: "1d", long: "1 day ago" })
  assert.deepEqual(age("2026-10-07T12:00:00Z"), { short: "2w", long: "2 weeks ago" })
  assert.deepEqual(age("2026-06-21T12:00:00Z"), { short: "4mo", long: "4 months ago" })
  assert.deepEqual(age("2024-10-01T12:00:00Z"), { short: "2y", long: "2 years ago" })
})

const third = commit("c", "chore: third")
const changed: CommitFile[] = [{ path: "src/a.ts", status: "modified" }, { path: "b.md", status: "added" }]

function pinned(expanded: boolean, files: CommitFiles | undefined): PinnedCompare {
  return { compare: { kind: "branch", base: "main", ahead: 2, mergeBase: third.hash }, expanded, files }
}

test("the pinned compare row leads, and expands to details and files like a commit", () => {
  const ready: CommitFiles = { kind: "ready", response: { files: changed, truncated: false } }
  assert.deepEqual(labels(commitRows([first], new Set(), new Map(), new Set(), pinned(false, ready))), ["> compare branch", "> fix: first"])
  const rows = commitRows([first], new Set(), new Map(), new Set(), pinned(true, ready))
  assert.deepEqual(labels(rows), ["v compare branch", "  compare details ready", "    v src/", "      a.ts modified", "    b.md added", "> fix: first"])
  assert.deepEqual(rows.map((_, index) => parentRowKey(rows, index)), ["", compareRowKey, compareRowKey, "folder:branch:src", compareRowKey, ""])
})

test("compare files open branch or range diff tabs and keep folders apart from commits", () => {
  const ready: CommitFiles = { kind: "ready", response: { files: changed, truncated: false } }
  const branchFiles = commitRows([], new Set(), new Map(), new Set([folderKey(first.hash, "src")]), pinned(true, ready))
  assert.deepEqual(branchFiles.flatMap((row) => row.kind === "file" ? [row.tab] : []), [branchTab("src/a.ts"), branchTab("b.md")])
  const range: PinnedCompare = { compare: { kind: "range", from: second.hash, to: first.hash, count: 2 }, expanded: true, files: ready }
  const rangeFiles = commitRows([], new Set(), new Map(), new Set(), range)
  assert.deepEqual(rangeFiles.flatMap((row) => row.kind === "file" ? [row.tab] : []),
    [rangeTab(second.hash, first.hash, "src/a.ts"), rangeTab(second.hash, first.hash, "b.md")])
  assert.equal(rangeFiles.some((row) => row.key === tabRowKey(rangeTab(second.hash, first.hash, "b.md"))), true)
})

test("open diff tabs select their file row", () => {
  const files = new Map([ready(first.hash, changed)])
  const rows = commitRows([first], new Set([first.hash]), files, new Set())
  assert.equal(rows.some((row) => row.kind === "file" && row.key === tabRowKey(commitTab(first.hash, "b.md"))), true)
  assert.equal(tabRowKey(diffTab("b.md")), "")
  assert.equal(tabRowKey(documentTab("b.md")), "")
})

test("range selections span listed commits oldest to newest in either direction", () => {
  const list = [first, second, third]
  assert.deepEqual(rangeSelection(list, first.hash, third.hash), { from: third.hash, to: first.hash })
  assert.deepEqual(rangeSelection(list, third.hash, second.hash), { from: third.hash, to: second.hash })
  assert.equal(rangeSelection(list, first.hash, first.hash), null)
  assert.equal(rangeSelection(list, first.hash, "f".repeat(40)), null)
  assert.deepEqual(selectedCommits(list, { from: third.hash, to: first.hash }), [first.hash, second.hash, third.hash])
  assert.deepEqual(selectedCommits(list, { from: second.hash, to: first.hash }), [first.hash, second.hash])
  assert.equal(selectedCommits([first, second], { from: third.hash, to: first.hash }), null)
  assert.equal(selectedCommits(list, null), null)
})
