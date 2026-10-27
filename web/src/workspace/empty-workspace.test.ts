import assert from "node:assert/strict"
import test from "node:test"

import { emptyWorkspace, type EmptyWorkspaceInput } from "./empty-workspace.ts"

const base: EmptyWorkspaceInput = {
  view: "explorer", mobile: false, searchPattern: "", searchFiles: 0, changes: null, repository: true, walkthroughs: null,
}

test("the Explorer asks for a file and offers file search", () => {
  assert.deepEqual(emptyWorkspace(base), { heading: "Choose a file", hint: "Select a file from the folder tree.", action: "find-file" })
})

test("Search asks for a query, then for a match once results show", () => {
  const search = { ...base, view: "search" } satisfies EmptyWorkspaceInput
  assert.deepEqual(emptyWorkspace(search), { heading: "Search file contents", hint: "Type in the Search field, then open a match.", action: null })
  assert.equal(emptyWorkspace({ ...search, searchPattern: "needle" }).heading, "Search file contents")
  assert.equal(emptyWorkspace({ ...search, searchPattern: "  ", searchFiles: 2 }).heading, "Search file contents")
  assert.deepEqual(emptyWorkspace({ ...search, searchPattern: "needle", searchFiles: 2 }),
    { heading: "Open a match", hint: "Choose a result to open it at that line.", action: null })
})

test("Source Control offers the uncommitted review, history when clean, and explains folders", () => {
  const sourceControl = { ...base, view: "source-control" } satisfies EmptyWorkspaceInput
  assert.deepEqual(emptyWorkspace({ ...sourceControl, changes: 4 }), {
    heading: "Review your changes", hint: "4 uncommitted changes. Open a file, a commit, or a branch.", action: "review-uncommitted",
  })
  assert.equal(emptyWorkspace({ ...sourceControl, changes: 1 }).hint, "1 uncommitted change. Open a file, a commit, or a branch.")
  assert.equal(emptyWorkspace({ ...sourceControl, changes: 1200 }).hint, "1,200 uncommitted changes. Open a file, a commit, or a branch.")
  assert.deepEqual(emptyWorkspace({ ...sourceControl, changes: 0 }),
    { heading: "Review history", hint: "No uncommitted changes. Open a commit or a branch.", action: null })
  assert.equal(emptyWorkspace(sourceControl).heading, "Review history")
  assert.deepEqual(emptyWorkspace({ ...sourceControl, changes: 0, repository: false }),
    { heading: "Not a git repository", hint: "Source Control needs a git repository.", action: null })
})

test("Walkthroughs ask for one, or say none are published yet", () => {
  const walkthroughs = { ...base, view: "walkthroughs" } satisfies EmptyWorkspaceInput
  assert.deepEqual(emptyWorkspace({ ...walkthroughs, walkthroughs: 3 }),
    { heading: "Choose a walkthrough", hint: "Open one from the list to start.", action: null })
  assert.equal(emptyWorkspace(walkthroughs).heading, "Choose a walkthrough")
  assert.deepEqual(emptyWorkspace({ ...walkthroughs, walkthroughs: 0 }),
    { heading: "No walkthroughs yet", hint: "Agents publish guided tours of this project here.", action: null })
})

test("phones follow the last view and open its drawer when it offers no other action", () => {
  const mobile = { ...base, mobile: true } satisfies EmptyWorkspaceInput
  assert.equal(emptyWorkspace(mobile).action, "find-file")
  assert.deepEqual(emptyWorkspace({ ...mobile, view: "search" }),
    { heading: "Search file contents", hint: "Type in the Search field, then open a match.", action: "show-view" })
  assert.equal(emptyWorkspace({ ...mobile, view: "source-control", changes: 2 }).action, "review-uncommitted")
  assert.equal(emptyWorkspace({ ...mobile, view: "source-control", changes: 0 }).action, "show-view")
  assert.equal(emptyWorkspace({ ...mobile, view: "walkthroughs", walkthroughs: 0 }).action, "show-view")
})

test("a hidden sidebar offers to show it", () => {
  assert.deepEqual(emptyWorkspace({ ...base, view: "hidden" }),
    { heading: "Nothing open", hint: "Open a file, a review, or a walkthrough.", action: "show-sidebar" })
})
