import assert from "node:assert/strict"
import { mkdirSync, mkdtempSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs"
import { realpath } from "node:fs/promises"
import { tmpdir } from "node:os"
import path from "node:path"
import { test } from "node:test"
import {
  isPlainAbsolute, NOT_SERVED, parseRevealRequest, PATH_ESCAPES, PATH_MISSING, PROJECT_UNAVAILABLE, relativeSegments, resolveRevealTarget,
  servedProject, within,
} from "../src/main/reveal.ts"

// A served root with a project, a file, a directory, an inside symlink, and
// symlinks escaping to a sibling folder that is not served.
function fixture() {
  const dir = realpathSync(mkdtempSync(path.join(tmpdir(), "servef-reveal-")))
  const root = path.join(dir, "root")
  const project = path.join(root, "alpha")
  const outside = path.join(dir, "outside")
  mkdirSync(path.join(project, "src"), { recursive: true })
  mkdirSync(outside)
  writeFileSync(path.join(project, "README.md"), "# Alpha\n")
  writeFileSync(path.join(project, "src", "main.go"), "package main\n")
  writeFileSync(path.join(outside, "secret.txt"), "secret\n")
  symlinkSync(path.join(project, "README.md"), path.join(project, "readme-link"))
  symlinkSync(outside, path.join(project, "escape"))
  symlinkSync(path.join(outside, "secret.txt"), path.join(project, "secret-link"))
  symlinkSync(project, path.join(dir, "alias"))
  return { dir, root, project, outside, done: () => rmSync(dir, { recursive: true, force: true }) }
}

test("parseRevealRequest accepts an absolute project and a relative path only", () => {
  assert.deepEqual(parseRevealRequest({ project: "/src/a", path: "docs/README.md" }), { project: "/src/a", path: "docs/README.md" })
  assert.deepEqual(parseRevealRequest({ project: "/src/a", path: "docs/" }), { project: "/src/a", path: "docs/" })
  for (const value of [
    { project: "src/a", path: "README.md" },
    { project: "/src/a/", path: "README.md" },
    { project: "/src/../a", path: "README.md" },
    { project: "/src/a\u0000", path: "README.md" },
    { project: "/src/a", path: "" },
    { project: "/src/a", path: "/etc/passwd" },
    { project: "/src/a", path: "../b/README.md" },
    { project: "/src/a", path: "docs/../../b" },
    { project: "/src/a", path: "./README.md" },
    { project: "/src/a", path: "docs//README.md" },
    { project: "/src/a", path: "docs\\..\\b" },
    { project: "/src/a", path: "a\u0000b" },
    { project: "/src/a", path: "x".repeat(5000) },
    { project: 1, path: "README.md" },
    { project: "/src/a" },
    null,
    "README.md",
  ]) {
    assert.equal(parseRevealRequest(value), null, JSON.stringify(value))
  }
})

test("path helpers", () => {
  assert.equal(isPlainAbsolute("/"), true)
  assert.equal(isPlainAbsolute("/a/b"), true)
  assert.equal(isPlainAbsolute("/a/./b"), false)
  assert.deepEqual(relativeSegments("a/b/c.txt"), ["a", "b", "c.txt"])
  assert.equal(relativeSegments(".."), null)
  assert.equal(within("/a/b", "/a/b"), true)
  assert.equal(within("/a/b", "/a/b/c"), true)
  assert.equal(within("/a/b", "/a/bc"), false)
  assert.equal(within("/", "/a"), true)
})

test("reveals files and directories inside a served project", async () => {
  const { root, project, done } = fixture()
  try {
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "README.md" }, realpath), { ok: true, target: path.join(project, "README.md") })
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "src/" }, realpath), { ok: true, target: path.join(project, "src") })
    // A symlink inside the project reveals its target.
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "readme-link" }, realpath), { ok: true, target: path.join(project, "README.md") })
    // The project itself as a root.
    assert.deepEqual(await resolveRevealTarget([project], { project, path: "README.md" }, realpath), { ok: true, target: path.join(project, "README.md") })
  } finally {
    done()
  }
})

test("refuses symlink escapes, missing files, unserved and missing projects", async () => {
  const { dir, root, project, outside, done } = fixture()
  try {
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "escape/secret.txt" }, realpath), { ok: false, reason: PATH_ESCAPES })
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "secret-link" }, realpath), { ok: false, reason: PATH_ESCAPES })
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "gone.md" }, realpath), { ok: false, reason: PATH_MISSING })
    assert.deepEqual(await resolveRevealTarget([root], { project: outside, path: "secret.txt" }, realpath), { ok: false, reason: NOT_SERVED })
    assert.deepEqual(await resolveRevealTarget([], { project, path: "README.md" }, realpath), { ok: false, reason: NOT_SERVED })
    assert.deepEqual(await resolveRevealTarget([root], { project: path.join(root, "missing"), path: "README.md" }, realpath), { ok: false, reason: PROJECT_UNAVAILABLE })
    assert.deepEqual(await resolveRevealTarget([root], { project, path: "../outside/secret.txt" }, realpath), { ok: false, reason: PATH_ESCAPES })
    // A symlinked project ID resolves to the served real path.
    assert.equal(await servedProject([root], path.join(dir, "alias"), realpath), project)
    // A root that no longer exists serves nothing.
    assert.equal(await servedProject([path.join(dir, "gone")], project, realpath), null)
  } finally {
    done()
  }
})
