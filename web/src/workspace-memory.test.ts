import assert from "node:assert/strict"
import test from "node:test"

import {
  forgetProject,
  parseMemory,
  projectMemory,
  rememberProject,
  rememberedProjects,
  restoreTabs,
  rootMemory,
} from "./workspace-memory.ts"

test("memory keeps projects apart per served root", () => {
  let memory = rememberProject({}, "/a", "api", ["README.md", "go.mod"], "go.mod")
  memory = rememberProject(memory, "/b", "api", ["other.md"], "other.md")
  assert.deepEqual(projectMemory(memory, "/a", "api"), { tabs: ["README.md", "go.mod"], active: "go.mod" })
  assert.deepEqual(projectMemory(memory, "/b", "api"), { tabs: ["other.md"], active: "other.md" })
  assert.equal(rootMemory(memory, "/a").last, "api")
  assert.equal(projectMemory(memory, "/a", "web"), undefined)
})

test("remembering a project drops an active tab that is not open", () => {
  const memory = rememberProject({}, "/a", "api", ["README.md"], "closed.md")
  assert.equal(projectMemory(memory, "/a", "api")?.active, "")
})

test("remembered projects list most recent first and can be forgotten", () => {
  let memory = rememberProject({}, "/a", "web", [], "")
  memory = rememberProject(memory, "/a", "notes", ["a.md"], "a.md")
  memory = rememberProject(memory, "/a", "web", [], "")
  assert.deepEqual(rememberedProjects(memory, "/a"), ["web", "notes"])
  memory = forgetProject(memory, "/a", "web")
  assert.deepEqual(rootMemory(memory, "/a"), { last: "", projects: { notes: { tabs: ["a.md"], active: "a.md" } } })
  memory = forgetProject(memory, "/a", "missing")
  assert.deepEqual(rememberedProjects(memory, "/a"), ["notes"])
})

test("memory caps remembered projects", () => {
  let memory = {}
  for (let index = 0; index < 60; index++) memory = rememberProject(memory, "/a", `p-${index}`, [], "")
  const projects = rememberedProjects(memory, "/a")
  assert.equal(projects.length, 50)
  assert.equal(projects[0], "p-59")
  assert.equal(projects.includes("p-0"), false)
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
  assert.deepEqual(parseMemory(null), {})
  assert.deepEqual(parseMemory(["x"]), {})
  assert.deepEqual(parseMemory({ "/a": { last: 3, opened: ["a", 2], projects: { p: { tabs: "x" }, q: null } } }),
    { "/a": { last: "", projects: { p: { tabs: [], active: "" } } } })
  const hostile = parseMemory(JSON.parse('{"__proto__": {"last": "x"}, "/a": {"projects": {"__proto__": {"tabs": ["t"]}}}}'))
  assert.equal(projectMemory(hostile, "/a", "constructor"), undefined)
  assert.equal(rootMemory(hostile, "toString").last, "")
})
