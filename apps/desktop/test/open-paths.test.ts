import assert from "node:assert/strict"
import { test } from "node:test"
import { droppedPathsOf, MAX_DROPPED_PATHS, openPathsOf, parseOpenPaths, projectLanding } from "../src/main/open-paths.ts"

test("parseOpenPaths reads arguments after --open-path only", () => {
  assert.deepEqual(parseOpenPaths(["/opt/servef/servef-desktop"], "/home/u"), [])
  assert.deepEqual(parseOpenPaths(["/opt/servef/servef-desktop", "servef://local/"], "/home/u"), [])
  assert.deepEqual(parseOpenPaths(["/opt/servef/servef-desktop", "--open-path"], "/home/u"), [])
  assert.deepEqual(parseOpenPaths(["electron", "desktop", "--open-path", "/src/a", "/src/b"], "/home/u"), ["/src/a", "/src/b"])
})

test("parseOpenPaths resolves relative paths against cwd and normalizes", () => {
  assert.deepEqual(parseOpenPaths(["x", "--open-path", "code/a", "../b", "/src/c/../d/"], "/home/u"), ["/home/u/code/a", "/home/b", "/src/d"])
})

test("parseOpenPaths converts local file URLs and drops other URLs", () => {
  const argv = [
    "x", "--open-path",
    "file:///src/a%20b",
    "file://localhost/src/c",
    "file://host/src/d",
    "file:///src/e%2Ff",
    "https://example.com/",
    "servef://local/?project=%2Fsrc",
    "",
  ]
  assert.deepEqual(parseOpenPaths(argv, "/home/u"), ["/src/a b", "/src/c"])
})

test("parseOpenPaths stops at the next switch and dedupes in order", () => {
  assert.deepEqual(parseOpenPaths(["x", "--open-path", "/a", "/b", "/a", "file:///b", "--no-sandbox", "/c"], "/"), ["/a", "/b"])
  assert.deepEqual(parseOpenPaths(["x", "--open-path", "/a", "--open-path", "/b"], "/"), ["/a"])
})

test("openPathsOf accepts absolute string arrays only", () => {
  assert.deepEqual(openPathsOf({ openPaths: ["/a", "/b c"] }), ["/a", "/b c"])
  assert.deepEqual(openPathsOf({ openPaths: [] }), [])
  for (const data of [null, undefined, "x", 1, ["/a"], {}, { openPaths: "/a" }, { openPaths: [1] }, { openPaths: ["/a", "relative"] }, { openPaths: ["/a", null] }]) {
    assert.deepEqual(openPathsOf(data), [], JSON.stringify(data) ?? "undefined")
  }
})

test("projectLanding encodes the project", () => {
  assert.equal(projectLanding("servef-app://local", "/src/a b&c"), "servef-app://local/?project=%2Fsrc%2Fa%20b%26c")
})

test("droppedPathsOf accepts a bounded array of absolute paths only", () => {
  assert.deepEqual(droppedPathsOf(["/a", "/b", "/a"]), ["/a", "/b"])
  assert.deepEqual(droppedPathsOf([]), [])
  for (const value of [["/a", "relative"], ["/a", 1], ["/a\u0000"], ["/" + "x".repeat(5000)], "/a", null, { paths: ["/a"] }]) {
    assert.deepEqual(droppedPathsOf(value), [], JSON.stringify(value))
  }
  assert.deepEqual(droppedPathsOf(Array.from({ length: MAX_DROPPED_PATHS + 1 }, (_, index) => `/p${index}`)), [])
  assert.equal(droppedPathsOf(Array.from({ length: MAX_DROPPED_PATHS }, (_, index) => `/p${index}`)).length, MAX_DROPPED_PATHS)
})
