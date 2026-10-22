import assert from "node:assert/strict"
import test from "node:test"

import { appendedCommits, commitsUrl, refreshedCommits } from "./commit-pages.ts"
import type { Commit, CommitsResponse } from "../../api/commits-contract.ts"

function commit(letter: string): Commit {
  return {
    hash: letter.repeat(40), parents: [], subject: letter, body: "",
    author: { name: "servef", email: "servef@example.com" }, authorTime: "2026-10-21T14:02:00+11:00",
  }
}

function response(head: string, letters: string, more: boolean, change: Partial<CommitsResponse> = {}): CommitsResponse {
  return {
    repository: true, branch: "feature", base: "main", range: "branch", head: head.repeat(40), ahead: 4,
    commits: [...letters].map(commit), more, ...change,
  }
}

const hashes = (value: CommitsResponse) => value.commits.map((entry) => entry.subject).join("")

test("a refresh of unchanged history keeps the pages already loaded", () => {
  const loaded = response("a", "abcd", false)
  const kept = refreshedCommits(loaded, response("a", "ab", true))
  assert.equal(hashes(kept), "abcd")
  assert.equal(kept.more, false)
})

test("a refresh replaces the list when history, base, or range changed", () => {
  const loaded = response("a", "abcd", false)
  assert.equal(hashes(refreshedCommits(null, response("a", "ab", true))), "ab")
  assert.equal(hashes(refreshedCommits(loaded, response("e", "ea", true))), "ea")
  assert.equal(hashes(refreshedCommits(loaded, response("a", "ab", true, { ahead: 3 }))), "ab")
  assert.equal(hashes(refreshedCommits(loaded, response("a", "ab", true, { base: "origin/main" }))), "ab")
  assert.equal(hashes(refreshedCommits(loaded, response("a", "ab", true, { range: "all" }))), "ab")
  assert.equal(hashes(refreshedCommits(loaded, response("a", "ax", true))), "ax")
})

test("next pages append to the same walk only", () => {
  const loaded = response("a", "ab", true)
  const next = appendedCommits(loaded, response("a", "bcd", false))
  assert.equal(hashes(next), "abcd")
  assert.equal(next.more, false)
  assert.equal(appendedCommits(loaded, response("e", "cd", false)), loaded)
})

test("page URLs pin the head after the first page", () => {
  assert.equal(commitsUrl("/repo", "branch", "", 0), "/api/commits?project=%2Frepo&range=branch&limit=50")
  assert.equal(commitsUrl("/repo", "all", "a".repeat(40), 50), `/api/commits?project=%2Frepo&range=all&limit=50&head=${"a".repeat(40)}&skip=50`)
})
