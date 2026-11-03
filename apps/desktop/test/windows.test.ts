import assert from "node:assert/strict"
import { test } from "node:test"
import { connectionOfOrigin, projectOfURL, routeWindow, targetOfURL } from "../src/main/windows.ts"
import type { WindowInfo } from "../src/main/windows.ts"

const local = "servef-app://local"
const box = "servef-app://ssh-box"
const target = (project: string, origin = local) => `${origin}/view?project=${encodeURIComponent(project)}&path=README.md`
const win = (id: number, project: string | null, focusedAt: number, origin = local, focused = false): WindowInfo => ({ id, origin, project, focused, focusedAt })

test("projectOfURL reads the project of servef-app URLs only", () => {
  assert.equal(projectOfURL("servef-app://local/view?project=%2Fsrc%2Fa"), "/src/a")
  assert.equal(projectOfURL("servef-app://ssh-box/view?project=%2Fsrc%2Fa"), "/src/a")
  assert.equal(projectOfURL("servef-app://local/?project="), null)
  assert.equal(projectOfURL("servef-app://local/"), null)
  assert.equal(projectOfURL("https://x/?project=%2Fa"), null)
  assert.equal(projectOfURL("nope"), null)
})

test("targetOfURL returns the servef-app origin and project", () => {
  assert.deepEqual(targetOfURL("servef-app://local/view?project=%2Fa&path=x"), { origin: local, project: "/a" })
  assert.deepEqual(targetOfURL("servef-app://ssh-box/"), { origin: box, project: null })
  assert.deepEqual(targetOfURL("servef-app://app/connect"), { origin: "servef-app://app", project: null })
  assert.deepEqual(targetOfURL("servef-app://local/?project="), { origin: local, project: null })
  for (const url of ["servef-app://user@local/", "servef-app://u:p@local/", "servef://local/", "https://local/?project=%2Fa", "nope", ""]) {
    assert.equal(targetOfURL(url), null, url)
  }
})

test("connectionOfOrigin returns the host of an origin only", () => {
  assert.equal(connectionOfOrigin(local), "local")
  assert.equal(connectionOfOrigin(box), "ssh-box")
  assert.equal(connectionOfOrigin("servef-app://app"), "app")
  assert.equal(connectionOfOrigin("servef-app://local/"), null)
  assert.equal(connectionOfOrigin("servef-app://local/?project=%2Fa"), null)
  assert.equal(connectionOfOrigin("https://local"), null)
})

test("a target project loads into that project's focused or most recent window", () => {
  const windows: WindowInfo[] = [win(1, "/a", 10), win(2, "/a", 30), win(3, "/b", 50)]
  assert.deepEqual(routeWindow(windows, target("/a")), { kind: "load", id: 2, url: target("/a") })
  const focused = windows.map((window) => window.id === 1 ? { ...window, focused: true } : window)
  assert.deepEqual(routeWindow(focused, target("/a")), { kind: "load", id: 1, url: target("/a") })
})

test("a target project without a window opens a new one", () => {
  const windows: WindowInfo[] = [win(1, "/b", 5, local, true)]
  assert.deepEqual(routeWindow(windows, target("/a")), { kind: "new", url: target("/a") })
  assert.deepEqual(routeWindow([], target("/a")), { kind: "new", url: target("/a") })
})

test("a target without a project focuses the latest window or opens one", () => {
  const windows: WindowInfo[] = [win(1, "/a", 40), win(2, null, 20)]
  assert.deepEqual(routeWindow(windows, "servef-app://local/"), { kind: "focus", id: 1 })
  assert.deepEqual(routeWindow([...windows, win(3, "/c", 0, local, true)], "servef-app://local/"), { kind: "focus", id: 3 })
  assert.deepEqual(routeWindow([], "servef-app://local/"), { kind: "new", url: "servef-app://local/" })
})

test("windows on another origin never take a target, even on the same project path", () => {
  const windows: WindowInfo[] = [win(1, "/a", 40, local, true), win(2, "/a", 10, box)]
  assert.deepEqual(routeWindow(windows, target("/a", box)), { kind: "load", id: 2, url: target("/a", box) })
  assert.deepEqual(routeWindow(windows, target("/a")), { kind: "load", id: 1, url: target("/a") })
  assert.deepEqual(routeWindow(windows, `${box}/`), { kind: "focus", id: 2 })
  const other = "servef-app://ssh-other"
  assert.deepEqual(routeWindow(windows, target("/a", other)), { kind: "new", url: target("/a", other) })
  assert.deepEqual(routeWindow(windows, `${other}/`), { kind: "new", url: `${other}/` })
  assert.deepEqual(routeWindow(windows, "servef-app://app/connect"), { kind: "new", url: "servef-app://app/connect" })
})
