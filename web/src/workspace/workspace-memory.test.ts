import assert from "node:assert/strict"
import test from "node:test"

import {
  emptyMemory,
  forgetProject,
  lastProject,
  parseMemory,
  projectMemory,
  rememberExpanded,
  rememberProject,
  rememberedProjects,
  restoreTabs,
  restoreExpanded,
  startupTab,
} from "./workspace-memory.ts"
import { documentTab, reviewTab, welcomeTab } from "./workspace-tabs.ts"

const working = reviewTab({ kind: "working" })

const docs = (...paths: string[]) => paths.map((path) => documentTab(path))

test("a project opens on its remembered tab, else Welcome when nothing was restored", () => {
  const readme = documentTab("README.md")
  assert.deepEqual(startupTab(null, { tabs: [readme], active: readme }, true), readme)
  assert.deepEqual(startupTab(null, { tabs: [], active: null }, true), welcomeTab())
  assert.equal(startupTab(null, { tabs: [], active: null }, false), null)
  // Restored tabs without an active one keep the pane empty.
  assert.equal(startupTab(null, { tabs: [readme], active: null }, true), null)
  // A routed tab wins over memory and Welcome.
  assert.equal(startupTab(working, { tabs: [], active: null }, true), null)
  assert.equal(startupTab(working, { tabs: [readme], active: readme }, true), null)
})

test("the Welcome tab is remembered like any tab", () => {
  const memory = rememberProject(emptyMemory, ["/r"], "/r", [welcomeTab(), documentTab("a.md")], welcomeTab())
  assert.deepEqual(restoreTabs(projectMemory(memory, "/r"), new Set(["a.md"])), { tabs: [welcomeTab(), documentTab("a.md")], active: welcomeTab() })
})

test("memory shares a project's tabs across sessions and keeps the last project per set of roots", () => {
  let memory = rememberProject(emptyMemory, ["/a"], "/a/api", docs("README.md", "go.mod"), documentTab("go.mod"))
  memory = rememberProject(memory, ["/a", "/b"], "/b/web", docs("other.md"), documentTab("other.md"))
  assert.deepEqual(projectMemory(memory, "/a/api"), { tabs: ["README.md", "go.mod"], active: "go.mod" })
  assert.deepEqual(projectMemory(memory, "/b/web"), { tabs: ["other.md"], active: "other.md" })
  assert.equal(lastProject(memory, ["/a"]), "/a/api")
  assert.equal(lastProject(memory, ["/a", "/b"]), "/b/web")
  assert.equal(lastProject(memory, ["/b", "/a"]), "")
  assert.equal(projectMemory(memory, "/a/web"), undefined)
})

test("remembering a project drops an active tab that is not open", () => {
  const memory = rememberProject(emptyMemory, ["/a"], "/a/api", docs("README.md"), documentTab("closed.md"))
  assert.equal(projectMemory(memory, "/a/api")?.active, "")
})

test("remembered projects list most recent first and can be forgotten", () => {
  let memory = rememberProject(emptyMemory, ["/a"], "/a/web", [], null)
  memory = rememberProject(memory, ["/a"], "/a/notes", docs("a.md"), documentTab("a.md"))
  memory = rememberProject(memory, ["/a"], "/a/web", [], null)
  assert.deepEqual(rememberedProjects(memory), ["/a/web", "/a/notes"])
  memory = forgetProject(memory, "/a/web")
  assert.deepEqual(memory, { last: {}, projects: { "/a/notes": { tabs: ["a.md"], active: "a.md" } } })
  memory = forgetProject(memory, "/a/missing")
  assert.deepEqual(rememberedProjects(memory), ["/a/notes"])
})

test("memory caps remembered projects", () => {
  let memory = emptyMemory
  for (let index = 0; index < 110; index++) memory = rememberProject(memory, ["/a"], `/a/p-${index}`, [], null)
  const projects = rememberedProjects(memory)
  assert.equal(projects.length, 100)
  assert.equal(projects[0], "/a/p-109")
  assert.equal(projects.includes("/a/p-0"), false)
})

test("restored tabs keep only files in the fresh tree", () => {
  const files = new Set(["README.md", "src/main.go"])
  assert.deepEqual(restoreTabs({ tabs: ["README.md", "gone.md", "src/main.go", "README.md"], active: "gone.md" }, files),
    { tabs: docs("README.md", "src/main.go"), active: null })
  assert.deepEqual(restoreTabs({ tabs: ["src/main.go"], active: "src/main.go" }, files),
    { tabs: docs("src/main.go"), active: documentTab("src/main.go") })
  assert.deepEqual(restoreTabs(undefined, files), { tabs: [], active: null })
})

test("parseMemory ignores malformed stored data", () => {
  assert.deepEqual(parseMemory(null), emptyMemory)
  assert.deepEqual(parseMemory(["x"]), emptyMemory)
  assert.deepEqual(parseMemory({ last: { "/a": 3, "/b": "/b/x" }, projects: { p: { tabs: "x" }, q: null } }),
    { last: { "/b": "/b/x" }, projects: { p: { tabs: [], active: "" } } })
  const hostile = parseMemory(JSON.parse('{"last": {"__proto__": "x"}, "projects": {"__proto__": {"tabs": ["t"]}}}'))
  assert.equal(projectMemory(hostile, "constructor"), undefined)
  assert.equal(lastProject(hostile, ["toString"]), "")
})

test("folder expansion and tabs preserve each other when saved independently", () => {
  let memory = rememberProject(emptyMemory, ["/a"], "/a/api", docs("README.md"), documentTab("README.md"))
  memory = rememberExpanded(memory, "/a/api", ["src/", "src/", "docs/"])
  memory = rememberExpanded(memory, "/a/web", [])
  memory = rememberProject(memory, ["/a"], "/a/api", docs("main.go"), documentTab("main.go"))
  assert.deepEqual(projectMemory(memory, "/a/api"), {
    tabs: ["main.go"], active: "main.go", expanded: ["src/", "docs/"],
  })
  assert.deepEqual(projectMemory(memory, "/a/web"), { tabs: [], active: "", expanded: [] })
  assert.equal(lastProject(memory, ["/a"]), "/a/api")
  assert.equal(projectMemory(forgetProject(memory, "/a/web"), "/a/web"), undefined)
})

test("restoring expansion validates fresh directories and preserves all-collapsed preference", () => {
  const directories = new Set(["src/", "docs/"])
  assert.deepEqual(restoreExpanded({ tabs: [], active: "", expanded: ["src/", "gone/", "../outside/", "main.go", "src/"] }, directories), ["src/"])
  assert.deepEqual(restoreExpanded({ tabs: [], active: "", expanded: [] }, directories), [])
  assert.equal(restoreExpanded({ tabs: [], active: "" }, directories), undefined)
  assert.equal(restoreExpanded(undefined, directories), undefined)
})

test("stored expansion is optional for existing memories and rejects malformed values", () => {
  const memory = parseMemory({ projects: {
    old: { tabs: ["a.md"], active: "a.md" },
    collapsed: { expanded: [] },
    mixed: { expanded: ["src/", 3, null] },
    invalid: { expanded: "src/" },
  } })
  assert.deepEqual(projectMemory(memory, "old"), { tabs: ["a.md"], active: "a.md" })
  assert.deepEqual(projectMemory(memory, "collapsed"), { tabs: [], active: "", expanded: [] })
  assert.deepEqual(projectMemory(memory, "mixed")?.expanded, ["src/"])
  assert.equal(projectMemory(memory, "invalid")?.expanded, undefined)
})

test("review tabs are remembered beside legacy document paths", () => {
  const tabs = [documentTab("README.md"), working]
  const memory = rememberProject(emptyMemory, ["/a"], "/a/api", tabs, working)
  assert.deepEqual(projectMemory(memory, "/a/api"), { tabs: ["README.md", { review: "working" }], active: { review: "working" } })
  assert.equal(projectMemory(rememberProject(emptyMemory, ["/a"], "/a/api", tabs, reviewTab({ kind: "branch", base: "", head: "", uncommitted: false })), "/a/api")?.active, "")
})

test("restored review tabs need no tree entry; documents still do", () => {
  const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
  const commit = reviewTab({ kind: "commit", hash })
  assert.deepEqual(restoreTabs({ tabs: ["README.md", { review: "commit", commit: hash }, "removed.md"], active: { review: "commit", commit: hash } },
    new Set(["README.md"])), { tabs: [documentTab("README.md"), commit], active: commit })
})

test("restored snapshots need no tree entry", () => {
  const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
  const snapshot = documentTab("removed.md", hash)
  assert.deepEqual(restoreTabs({ tabs: [{ path: "removed.md", at: hash }, "removed.md"], active: { path: "removed.md", at: hash } },
    new Set(["README.md"])), { tabs: [snapshot], active: snapshot })
})

test("per-file diff tabs stored before reviews restore as one review per compare", () => {
  const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
  const memory = parseMemory({ projects: { p: {
    tabs: ["a.md", { diff: "b.md" }, { diff: "c.md" }, { commit: hash, path: "a.md" }, { commit: hash, path: "b.md" }, { diff: 3 }, "", null],
    active: { diff: "c.md" },
  } } })
  assert.deepEqual(projectMemory(memory, "p"), {
    tabs: ["a.md", { review: "working" }, { review: "working" }, { review: "commit", commit: hash }, { review: "commit", commit: hash }],
    active: { review: "working" },
  })
  assert.deepEqual(restoreTabs(projectMemory(memory, "p"), new Set(["a.md"])), {
    tabs: [documentTab("a.md"), working, reviewTab({ kind: "commit", hash })], active: working,
  })
})
