import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { isChangesResponse, isDiffResponse, type DiffResponse } from "./changes-contract.ts"
import { isBranchFilesResponse, isCommitFilesResponse, isCommitsResponse, isRangeFilesResponse } from "./commits-contract.ts"
import { isCompareStatsResponse } from "./compare-stats-contract.ts"
import { isRefsResponse, unknownBaseCode } from "./refs-contract.ts"
import { isStackResponse, unknownHeadCode } from "./stack-contract.ts"
import { isFormatResult, isPageData, type PageData } from "./page-contract.ts"
import { isNavigationResult, isNavigationStatus } from "./navigation-contract.ts"
import { isProcessMetrics } from "../workspace/process-metrics.ts"
import { isDirectoriesResponse, isProjectsResponse } from "./projects-contract.ts"
import { isSearchDocumentsResponse } from "../features/search/search-documents.ts"
import { isRecord } from "./http.ts"
import { isWalkthrough, isWalkthroughSummaries } from "./walkthrough-contract.ts"
import { isLiveChange, isLiveStatus } from "./events-contract.ts"

async function fixture(name: string): Promise<unknown> {
  const source = await readFile(
    new URL(`../../../testdata/api/default/${name}`, import.meta.url),
    "utf8",
  )
  const value: unknown = JSON.parse(source)
  return value
}

test("frontend API fixtures satisfy runtime contracts", async () => {
  assert.equal(isProcessMetrics(await fixture("metrics.json")), true)
  assert.equal(isProcessMetrics(await fixture("metrics-go.json")), true)
  assert.equal(isSearchDocumentsResponse(await fixture("search-documents.json")), true)
  assert.equal(isProjectsResponse(await fixture("projects.json")), true)
  assert.equal(isDirectoriesResponse(await fixture("directories.json")), true)
  for (const name of [
    "page-index.json", "page-document.json", "page-missing.json", "page-json.json",
    "page-json-broken.json", "page-image.json", "page-binary.json", "page-large.json", "page-too-large.json", "page-go.json",
    "page-commit.json",
  ]) {
    assert.equal(isPageData(await fixture(name)), true, name)
  }
  assert.equal(isFormatResult(await fixture("format-settings.json")), true)
  assert.equal(isFormatResult(await fixture("format-error.json")), false)
  assert.equal(isNavigationResult(await fixture("definition.json")), true)
  assert.equal(isNavigationResult(await fixture("references.json")), true)
  assert.equal(isNavigationResult(await fixture("error-not-found.json")), false)
  assert.equal(isNavigationStatus(await fixture("navigation-status.json")), true)
  assert.equal(isNavigationStatus(await fixture("definition.json")), false)
})

async function page(name: string): Promise<PageData> {
  const value = await fixture(name)
  assert.ok(isPageData(value), name)
  return value
}

test("page fixtures follow document-shape invariants", async () => {
  const json = await page("page-json.json")
  assert.equal(json.documentType?.formattable, true)
  assert.equal(json.content, "")

  for (const name of ["page-image.json", "page-binary.json"]) {
    const binary = await page(name)
    assert.equal(binary.binary, true, name)
    assert.equal(binary.source, "", name)
    assert.equal(binary.highlightDisabled, true, name)
    assert.equal(binary.content, "", name)
  }
  assert.equal((await page("page-image.json")).documentType?.previewKind, "image")
  assert.equal((await page("page-binary.json")).documentType?.label, "Plain text")

  const large = await page("page-large.json")
  assert.equal(large.previewDisabled, true)
  assert.equal(large.highlightDisabled, true)
  assert.equal(large.searchExcluded, true)
  assert.ok(large.fileSize > 8 * 1_048_576)
  assert.equal(large.tooLarge, false)

  const tooLarge = await page("page-too-large.json")
  assert.equal(tooLarge.tooLarge, true)
  assert.equal(tooLarge.binary, false)
  assert.equal(tooLarge.previewDisabled, true)
  assert.equal(tooLarge.searchExcluded, true)
  assert.equal(tooLarge.source, "")
  assert.equal(tooLarge.highlightDisabled, true)
  assert.equal(tooLarge.content, "")
  assert.ok(tooLarge.fileSize > 32 * 1_048_576)
})

test("contract validators reject removed and malformed fields", async () => {
  const value = await fixture("page-document.json")
  assert.ok(isPageData(value))
  const { filenamePatterns: _patterns, ...withoutPatterns } = value
  assert.equal(isPageData(withoutPatterns), false)
  assert.equal(isPageData({ ...value, binary: "no" }), false)
  const { tooLarge: _tooLarge, ...withoutTooLarge } = value
  assert.equal(isPageData(withoutTooLarge), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, previewKind: "code" } }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, filenames: null } }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, formattable: undefined } }), false)
  assert.equal(isPageData({ ...value, documentType: null }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, previewLabel: "" } }), false)
  const { highlightDisabled: _highlightDisabled, ...withoutHighlightDisabled } = value
  assert.equal(isPageData(withoutHighlightDisabled), false)
  assert.equal(isFormatResult({ formatted: "{}\n" }), true)
  assert.equal(isFormatResult({ formatted: ["ok"] }), false)
  assert.equal(isFormatResult({ formattedLines: ["ok"] }), false)
})

test("navigation contract validators reject malformed ranges and states", async () => {
  const definition = await fixture("definition.json")
  assert.ok(isNavigationResult(definition))
  assert.equal(isNavigationResult({ ...definition, filtered: -1 }), false)
  assert.equal(isNavigationResult({ ...definition, server: 1 }), false)
  assert.equal(isNavigationResult({ ...definition, locations: [{ path: "a.go", line: 0, column: 0, endLine: 1, endColumn: 1, kind: "", snippet: "" }] }), false)
  const status = await fixture("navigation-status.json")
  assert.ok(isNavigationStatus(status))
  assert.ok(status.server)
  assert.equal(isNavigationStatus({ ...status, server: { ...status.server, state: "running" } }), false)
  assert.equal(isNavigationStatus({ ...status, server: { ...status.server, install: [{ label: "Go" }] } }), false)
  assert.equal(isNavigationStatus({ ...status, supported: false, eligible: false, server: null }), true)
})

test("project contract validators reject inconsistent availability", async () => {
  const value = await fixture("projects.json")
  assert.ok(isProjectsResponse(value))
  const [first] = value.projects
  assert.ok(first)
  assert.equal(isProjectsResponse({ ...value, projects: [{ ...first, id: "" }] }), false)
  assert.equal(isProjectsResponse({ ...value, projects: [{ ...first, kind: "submodule" }] }), false)
  const page = await fixture("page-document.json")
  assert.ok(isPageData(page))
  const { project: _project, ...withoutProject } = page
  assert.equal(isPageData(withoutProject), false)
})

async function diff(name: string): Promise<DiffResponse> {
  const value = await fixture(name)
  assert.ok(isDiffResponse(value), name)
  return value
}

test("source control fixtures satisfy runtime contracts", async () => {
  const changes = await fixture("changes.json")
  assert.ok(isChangesResponse(changes))
  assert.equal(changes.repository, true)
  assert.deepEqual(changes.files.map((file) => file.path), changes.files.map((file) => file.path).toSorted())
  assert.ok(changes.files.some((file) => file.status === "renamed" && file.oldPath))
  const folder = await fixture("changes-not-repo.json")
  assert.ok(isChangesResponse(folder))
  assert.equal(folder.repository, false)
  assert.deepEqual(folder.files, [])

  const modified = await diff("diff-modified.json")
  assert.equal(typeof modified.original?.content, "string")
  assert.equal(typeof modified.modified?.content, "string")
  assert.equal((await diff("diff-added.json")).original, null)
  assert.equal((await diff("diff-deleted.json")).modified, null)
  const untracked = await diff("diff-untracked.json")
  assert.equal(untracked.status, "untracked")
  assert.equal(untracked.original, null)
  assert.ok((await diff("diff-renamed.json")).oldPath)
  const binary = await diff("diff-binary.json")
  assert.equal(binary.original?.binary, true)
  assert.equal(binary.modified?.content, undefined)
  const large = await diff("diff-too-large.json")
  assert.equal(large.modified?.tooLarge, true)
  assert.equal(large.modified?.content, undefined)
  assert.equal(isDiffResponse(await fixture("error-not-found.json")), false)
})

test("commit history fixtures satisfy runtime contracts", async () => {
  const branch = await fixture("commits.json")
  assert.ok(isCommitsResponse(branch))
  assert.equal(branch.range, "branch")
  assert.equal(branch.head, branch.commits[0]?.hash)
  assert.equal(branch.ahead, branch.commits.length)
  assert.ok(branch.commits.some((commit) => commit.body.includes("\n")))
  const all = await fixture("commits-all.json")
  assert.ok(isCommitsResponse(all))
  assert.equal(all.range, "all")
  assert.deepEqual(all.commits.at(-1)?.parents, [])
  const folder = await fixture("commits-not-repo.json")
  assert.ok(isCommitsResponse(folder))
  assert.equal(folder.repository, false)

  const files = await fixture("commit-files.json")
  assert.ok(isCommitFilesResponse(files))
  assert.equal(files.commit, branch.head)
  assert.deepEqual(files.files.map((file) => file.path), files.files.map((file) => file.path).toSorted())
  assert.ok(files.files.some((file) => file.status === "renamed" && file.oldPath))

  const branchFiles = await fixture("branch-files.json")
  assert.ok(isBranchFilesResponse(branchFiles))
  assert.equal(branchFiles.head, branch.head)
  assert.equal(branchFiles.base, branch.base)
  assert.equal(branchFiles.ahead, branch.ahead)
  assert.notEqual(branchFiles.mergeBase, "")
  const rangeFiles = await fixture("range-files.json")
  assert.ok(isRangeFilesResponse(rangeFiles))
  assert.equal(rangeFiles.to, branch.head)
  assert.ok(branch.commits.some((commit) => commit.hash === rangeFiles.from))
})

test("commit contract validators reject malformed history", async () => {
  const branch = await fixture("commits.json")
  assert.ok(isCommitsResponse(branch))
  const [first] = branch.commits
  assert.ok(first)
  assert.equal(isCommitsResponse({ ...branch, range: "everything" }), false)
  assert.equal(isCommitsResponse({ ...branch, head: "3577ec6" }), false)
  assert.equal(isCommitsResponse({ ...branch, ahead: -1 }), false)
  assert.equal(isCommitsResponse({ ...branch, repository: false }), false)
  assert.equal(isCommitsResponse({ ...branch, commits: [{ ...first, authorTime: "yesterday" }] }), false)
  assert.equal(isCommitsResponse({ ...branch, commits: [{ ...first, parents: ["HEAD~1"] }] }), false)
  assert.equal(isCommitsResponse({ ...branch, commits: [{ ...first, author: "Dinesh" }] }), false)
  const files = await fixture("commit-files.json")
  assert.ok(isCommitFilesResponse(files))
  assert.equal(isCommitFilesResponse({ ...files, files: [{ path: "a.md", status: "untracked" }] }), false)
  assert.equal(isCommitFilesResponse({ ...files, files: [{ path: "a.md", status: "renamed" }] }), false)
  assert.equal(isCommitFilesResponse({ ...files, commit: "" }), false)
})

test("compare fixtures satisfy runtime contracts", async () => {
  const refs = await fixture("refs.json")
  assert.ok(isRefsResponse(refs))
  const commits = await fixture("commits.json")
  assert.ok(isCommitsResponse(commits))
  assert.equal(refs.branch, commits.branch)
  assert.equal(refs.defaultBase, commits.base)
  const names = refs.branches.map((branch) => branch.name)
  assert.ok(names.includes(refs.defaultBase))
  // Local branches lead, then remote-tracking ones, each sorted by name.
  const local = refs.branches.filter((branch) => !branch.remote).map((branch) => branch.name)
  const remote = refs.branches.filter((branch) => branch.remote).map((branch) => branch.name)
  assert.deepEqual(names, [...local.toSorted(), ...remote.toSorted()])

  const unknownBase = await fixture("error-unknown-base.json")
  assert.deepEqual(unknownBase, { error: "That base branch doesn't exist.", code: unknownBaseCode })
  const unknownHead = await fixture("error-unknown-head.json")
  assert.deepEqual(unknownHead, { error: "That head branch doesn't exist.", code: unknownHeadCode })

  // The stack runs top to bottom, HEAD's layer at commits.json's head, every
  // layer a listed local branch, and the lowest on the base.
  const stack = await fixture("branch-stack.json")
  assert.ok(isStackResponse(stack))
  assert.equal(stack.branch, commits.branch)
  assert.equal(stack.base, commits.base)
  assert.equal(stack.head, commits.head)
  assert.equal(stack.layers.filter((layer) => layer.current).map((layer) => layer.head).join(), commits.head)
  for (const layer of stack.layers) assert.ok(local.includes(layer.name))
  assert.equal(stack.layers.at(-1)?.parent, stack.base)
  assert.equal(isStackResponse({ ...stack, layers: [{ ...stack.layers[0], head: "HEAD" }] }), false)
  assert.equal(isStackResponse({ ...stack, layers: [{ ...stack.layers[0], name: "", current: false }] }), false)
  assert.equal(isStackResponse({ ...stack, repository: false }), false)

  const changes = await fixture("changes.json")
  assert.ok(isChangesResponse(changes))
  const working = await fixture("compare-stats.json")
  assert.ok(isCompareStatsResponse(working))
  assert.deepEqual(working.files.map((file) => file.path), changes.files.map((file) => file.path))
  assert.ok(working.files.some((file) => file.status === "binary"))
  assert.ok(working.files.some((file) => file.status === "tooLarge" && file.blob === ""))
  const commitFiles = await fixture("commit-files.json")
  assert.ok(isCommitFilesResponse(commitFiles))
  const history = await fixture("compare-stats-history.json")
  assert.ok(isCompareStatsResponse(history))
  assert.deepEqual(history.files.map((file) => file.path), commitFiles.files.map((file) => file.path))
})

test("compare contract validators reject malformed refs and stats", async () => {
  const refs = await fixture("refs.json")
  assert.ok(isRefsResponse(refs))
  assert.equal(isRefsResponse({ ...refs, branches: [{ name: "", remote: false }] }), false)
  assert.equal(isRefsResponse({ ...refs, branches: [{ name: "main" }] }), false)
  assert.equal(isRefsResponse({ ...refs, repository: false }), false)
  assert.equal(isRefsResponse({ ...refs, defaultBase: null }), false)
  assert.equal(isRefsResponse({ repository: false, branch: "", defaultBase: "", branches: [], truncated: false }), true)

  const stats = await fixture("compare-stats.json")
  assert.ok(isCompareStatsResponse(stats))
  const blob = "62610a06115d711b3548a0231dde95ba14288358"
  const valid = (file: unknown, budgetExceeded = false) => isCompareStatsResponse({ files: [file], budgetExceeded })
  assert.equal(valid({ path: "a.go", status: "counted", additions: 1, deletions: 0, blob }), true)
  assert.equal(valid({ path: "a.go", status: "budget", blob: "" }, true), true)
  assert.equal(valid({ path: "a.go", status: "budget", blob }), false)
  assert.equal(valid({ path: "a.go", status: "counted", additions: 1, blob }), false)
  assert.equal(valid({ path: "a.go", status: "counted", additions: -1, deletions: 0, blob }), false)
  assert.equal(valid({ path: "a.go", status: "counted", additions: 1.5, deletions: 0, blob }), false)
  assert.equal(valid({ path: "a.go", status: "binary", additions: 0, deletions: 0, blob }), false)
  assert.equal(valid({ path: "a.go", status: "skipped", blob }), false)
  assert.equal(valid({ path: "a.go", status: "tooLarge", blob: "3577ec6" }), false)
  assert.equal(valid({ path: "", status: "tooLarge", blob: "" }), false)
  assert.equal(isCompareStatsResponse({ files: [] }), false)
})

test("walkthrough fixtures satisfy runtime contracts", async () => {
  const list = await fixture("walkthroughs.json")
  assert.ok(isWalkthroughSummaries(list))
  assert.equal(isWalkthroughSummaries([{ id: "a", title: "A", stepCount: -1, updatedAt: "2026-10-26T09:30:00Z" }]), false)
  assert.equal(isWalkthroughSummaries([{ id: "a", title: "A", updatedAt: "2026-10-26T09:30:00Z" }]), false)

  const tour = await fixture("walkthrough-tour.json")
  assert.ok(isWalkthrough(tour))
  assert.ok(isRecord(tour) && Array.isArray(tour.steps))
  const steps: unknown[] = tour.steps
  const withStep = (step: unknown) => isWalkthrough({ ...tour, steps: [step] })
  const [note, file, , , diff] = steps
  assert.ok(isRecord(note) && isRecord(file) && isRecord(diff))
  assert.equal(withStep({ ...note, bodyHTML: undefined }), false)
  assert.equal(withStep({ ...file, diff }), false, "a step targets a file or a diff, not both")
  assert.equal(withStep({ ...file, file: { path: "a.go", lines: [3] } }), false)
  assert.equal(withStep({ ...diff, diff: { compare: { kind: "stash" } } }), false)
})

test("live update fixtures satisfy runtime contracts", async () => {
  assert.ok(isLiveStatus(await fixture("events-status.json")))
  assert.ok(isLiveChange(await fixture("events-change.json")))
  assert.equal(isLiveStatus({ mode: "paused", reason: "" }), false)
  assert.equal(isLiveStatus({ mode: "off" }), false)
  assert.equal(isLiveChange({ paths: [""], tree: false, git: false, overflow: false }), false)
  assert.equal(isLiveChange({ paths: [], tree: false, git: false }), false)
})
