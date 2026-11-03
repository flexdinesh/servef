import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { test } from "node:test"
import {
  addRecent, createRecentStore, MAX_RECENT, newlyShown, parseRecent, recentLabel, removeRecent, serializeRecent,
} from "../src/main/recent-folders.ts"

test("addRecent moves a folder to the front, deduplicates, and bounds the list", () => {
  assert.deepEqual(addRecent([], "/a"), ["/a"])
  assert.deepEqual(addRecent(["/a", "/b"], "/b"), ["/b", "/a"])
  let list: string[] = []
  for (let index = 0; index < MAX_RECENT + 3; index++) list = addRecent(list, `/p${index}`)
  assert.equal(list.length, MAX_RECENT)
  assert.equal(list[0], `/p${MAX_RECENT + 2}`)
  assert.equal(list.includes("/p0"), false)
  assert.deepEqual(removeRecent(["/a", "/b"], "/a"), ["/b"])
})

test("parseRecent keeps valid absolute entries of version 1 only", () => {
  assert.deepEqual(parseRecent(serializeRecent(["/a", "/b"])), ["/a", "/b"])
  assert.deepEqual(parseRecent(JSON.stringify({ version: 1, folders: ["/a", "relative", 3, "/a", "/b/../c", "/n\u0000"] })), ["/a"])
  assert.deepEqual(parseRecent(JSON.stringify({ version: 2, folders: ["/a"] })), [])
  assert.deepEqual(parseRecent(JSON.stringify({ version: 1, folders: "/a" })), [])
  assert.deepEqual(parseRecent("not json"), [])
  const many = Array.from({ length: MAX_RECENT + 5 }, (_, index) => `/p${index}`)
  assert.equal(parseRecent(serializeRecent(many)).length, MAX_RECENT)
})

test("recentLabel abbreviates the home directory", () => {
  assert.equal(recentLabel("/home/me/src/a", "/home/me"), "~/src/a")
  assert.equal(recentLabel("/home/me", "/home/me"), "~")
  assert.equal(recentLabel("/home/meta/a", "/home/me"), "/home/meta/a")
  assert.equal(recentLabel("/srv/a", ""), "/srv/a")
})

test("newlyShown lists projects a window just started showing", () => {
  assert.deepEqual(newlyShown(new Set(["/a"]), ["/a", "/b", "/c"]), ["/b", "/c"])
  assert.deepEqual(newlyShown(new Set(["/a", "/b"]), ["/a"]), [])
})

test("the store round-trips privately and loads missing or broken files as empty", () => {
  const dir = mkdtempSync(path.join(tmpdir(), "servef-recent-"))
  try {
    const file = path.join(dir, "nested", "recent-folders.json")
    const store = createRecentStore(file)
    assert.deepEqual(store.load(), [])
    store.save(["/a", "/b"])
    assert.deepEqual(store.load(), ["/a", "/b"])
    assert.equal(statSync(file).mode & 0o777, 0o600)
    assert.equal(readFileSync(file, "utf8"), serializeRecent(["/a", "/b"]))
    writeFileSync(file, "{")
    assert.deepEqual(store.load(), [])
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})
