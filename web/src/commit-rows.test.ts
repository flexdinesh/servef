import assert from "node:assert/strict"
import test from "node:test"

import { commitAge, commitRows, folderKey, parentRowKey, type CommitFiles } from "./commit-rows.ts"
import type { Commit, CommitFile } from "./commits-contract.ts"

function commit(hash: string, subject: string): Commit {
  return {
    hash: hash.repeat(40).slice(0, 40), parents: [], subject, body: "",
    author: { name: "servef", email: "servef@example.com" }, authorTime: "2026-10-21T14:02:00+11:00",
  }
}

const first = commit("a", "fix: first")
const second = commit("b", "feat: second")

function ready(hash: string, files: CommitFile[]): [string, CommitFiles] {
  return [hash, { kind: "ready", response: { commit: hash, files, truncated: false } }]
}

function labels(rows: ReturnType<typeof commitRows>): string[] {
  return rows.map((row) => {
    if (row.kind === "commit") return `${row.expanded ? "v" : ">"} ${row.commit.subject}`
    if (row.kind === "details") return `  details ${row.files?.kind ?? "none"}`
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
