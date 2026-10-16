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
} from "./workspace-memory.ts"

test("memory shares a project's tabs across sessions and keeps the last project per set of roots", () => {
  let memory = rememberProject(emptyMemory, ["/a"], "/a/api", ["README.md", "go.mod"], "go.mod")
  memory = rememberProject(memory, ["/a", "/b"], "/b/web", ["other.md"], "other.md")
  assert.deepEqual(projectMemory(memory, "/a/api"), { tabs: ["README.md", "go.mod"], active: "go.mod" })
  assert.deepEqual(projectMemory(memory, "/b/web"), { tabs: ["other.md"], active: "other.md" })
  assert.equal(lastProject(memory, ["/a"]), "/a/api")
  assert.equal(lastProject(memory, ["/a", "/b"]), "/b/web")
  assert.equal(lastProject(memory, ["/b", "/a"]), "")
  assert.equal(projectMemory(memory, "/a/web"), undefined)
})

test("remembering a project drops an active tab that is not open", () => {
  const memory = rememberProject(emptyMemory, ["/a"], "/a/api", ["README.md"], "closed.md")
  assert.equal(projectMemory(memory, "/a/api")?.active, "")
})

test("remembered projects list most recent first and can be forgotten", () => {
  let memory = rememberProject(emptyMemory, ["/a"], "/a/web", [], "")
  memory = rememberProject(memory, ["/a"], "/a/notes", ["a.md"], "a.md")
  memory = rememberProject(memory, ["/a"], "/a/web", [], "")
  assert.deepEqual(rememberedProjects(memory), ["/a/web", "/a/notes"])
  memory = forgetProject(memory, "/a/web")
  assert.deepEqual(memory, { last: {}, projects: { "/a/notes": { tabs: ["a.md"], active: "a.md" } } })
  memory = forgetProject(memory, "/a/missing")
  assert.deepEqual(rememberedProjects(memory), ["/a/notes"])
})

test("memory caps remembered projects", () => {
  let memory = emptyMemory
  for (let index = 0; index < 110; index++) memory = rememberProject(memory, ["/a"], `/a/p-${index}`, [], "")
  const projects = rememberedProjects(memory)
  assert.equal(projects.length, 100)
  assert.equal(projects[0], "/a/p-109")
  assert.equal(projects.includes("/a/p-0"), false)
})

test("restored tabs keep only files in the fresh tree", () => {
  const files = new Set(["README.md", "src/main.go"])
  assert.deepEqual(restoreTabs({ tabs: ["README.md", "gone.md", "src/main.go", "README.md"], active: "gone.md" }, files),
    { tabs: ["README.md", "src/main.go"], active: "" })
  assert.deepEqual(restoreTabs({ tabs: ["src/main.go"], active: "src/main.go" }, files),
    { tabs: ["src/main.go"], active: "src/main.go" })
  assert.deepEqual(restoreTabs(undefined, files), { tabs: [], active: "" })
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
  let memory = rememberProject(emptyMemory, ["/a"], "/a/api", ["README.md"], "README.md")
  memory = rememberExpanded(memory, "/a/api", ["src/", "src/", "docs/"])
  memory = rememberExpanded(memory, "/a/web", [])
  memory = rememberProject(memory, ["/a"], "/a/api", ["main.go"], "main.go")
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
