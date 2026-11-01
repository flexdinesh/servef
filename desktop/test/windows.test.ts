import assert from "node:assert/strict"
import { test } from "node:test"
import { projectOfURL, routeWindow } from "../src/main/windows.ts"
import type { WindowInfo } from "../src/main/windows.ts"

const target = (project: string) => `servef-app://local/view?project=${encodeURIComponent(project)}&path=README.md`

test("projectOfURL reads the project of servef-app URLs only", () => {
  assert.equal(projectOfURL("servef-app://local/view?project=%2Fsrc%2Fa"), "/src/a")
  assert.equal(projectOfURL("servef-app://local/?project="), null)
  assert.equal(projectOfURL("servef-app://local/"), null)
  assert.equal(projectOfURL("https://x/?project=%2Fa"), null)
  assert.equal(projectOfURL("nope"), null)
})

test("a target project loads into that project's focused or most recent window", () => {
  const windows: WindowInfo[] = [
    { id: 1, project: "/a", focused: false, focusedAt: 10 },
    { id: 2, project: "/a", focused: false, focusedAt: 30 },
    { id: 3, project: "/b", focused: false, focusedAt: 50 },
  ]
  assert.deepEqual(routeWindow(windows, target("/a")), { kind: "load", id: 2, url: target("/a") })
  const focused = windows.map((window) => window.id === 1 ? { ...window, focused: true } : window)
  assert.deepEqual(routeWindow(focused, target("/a")), { kind: "load", id: 1, url: target("/a") })
})

test("a target project without a window opens a new one", () => {
  const windows: WindowInfo[] = [{ id: 1, project: "/b", focused: true, focusedAt: 5 }]
  assert.deepEqual(routeWindow(windows, target("/a")), { kind: "new", url: target("/a") })
  assert.deepEqual(routeWindow([], target("/a")), { kind: "new", url: target("/a") })
})

test("a target without a project focuses the latest window or opens one", () => {
  const windows: WindowInfo[] = [
    { id: 1, project: "/a", focused: false, focusedAt: 40 },
    { id: 2, project: null, focused: false, focusedAt: 20 },
  ]
  assert.deepEqual(routeWindow(windows, "servef-app://local/"), { kind: "focus", id: 1 })
  assert.deepEqual(routeWindow([...windows, { id: 3, project: "/c", focused: true, focusedAt: 0 }], "servef-app://local/"), { kind: "focus", id: 3 })
  assert.deepEqual(routeWindow([], "servef-app://local/"), { kind: "new", url: "servef-app://local/" })
})
