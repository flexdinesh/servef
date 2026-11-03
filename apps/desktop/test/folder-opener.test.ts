import assert from "node:assert/strict"
import { test } from "node:test"
import type { OpenOutcome } from "../src/main/daemon.ts"
import { createFolderOpener, DAEMON_NOT_RUNNING } from "../src/main/folder-opener.ts"

const ORIGIN = "servef-app://local"

interface Fake {
  opened: string[]
  routes: string[]
  reports: { message: string; detail: string }[]
  windows: number
}

// fake answers /_daemon/open from projects (path → project, or an error string
// prefixed with "!"); routes open a window each.
function setup(projects: Record<string, string>, options: { socket?: string | null; windows?: number } = {}) {
  const fake: Fake = { opened: [], routes: [], reports: [], windows: options.windows ?? 0 }
  const opener = createFolderOpener({
    socket: async () => options.socket === undefined ? "/run/d.sock" : options.socket,
    open: async (socket, path): Promise<OpenOutcome> => {
      assert.equal(socket, "/run/d.sock")
      fake.opened.push(path)
      const answer = projects[path]
      if (answer === undefined || answer.startsWith("!")) return { ok: false, status: 400, code: "invalidRoot", error: answer?.slice(1) ?? "no such folder" }
      return { ok: true, result: { root: answer, added: true, replaced: [], project: answer } }
    },
    route: (url) => {
      fake.routes.push(url)
      fake.windows++
    },
    windowCount: () => fake.windows,
    origin: ORIGIN,
    report: (message, detail) => { fake.reports.push({ message, detail }) },
  })
  return { fake, opener }
}

test("opens sequentially and routes each distinct project once, in order", async () => {
  const { fake, opener } = setup({ "/src/b": "/src/b", "/src/a": "/src/a", "/src/a/sub": "/src/a" })
  const result = await opener.open(["/src/b", "/src/a", "/src/a/sub"])
  assert.deepEqual(fake.opened, ["/src/b", "/src/a", "/src/a/sub"])
  assert.deepEqual(result, { projects: ["/src/b", "/src/a"], failed: [], skipped: 0 })
  assert.deepEqual(fake.routes, [`${ORIGIN}/?project=%2Fsrc%2Fb`, `${ORIGIN}/?project=%2Fsrc%2Fa`])
  assert.deepEqual(fake.reports, [])
})

test("collects failures into one report and still routes successes", async () => {
  const { fake, opener } = setup({ "/src/a": "/src/a", "/src/file.txt": "!not a directory" })
  const result = await opener.open(["/src/file.txt", "/src/a", "/missing"])
  assert.deepEqual(result.projects, ["/src/a"])
  assert.deepEqual(fake.routes, [`${ORIGIN}/?project=%2Fsrc%2Fa`])
  assert.deepEqual(fake.reports, [{ message: "Could not open folders", detail: "/src/file.txt: not a directory\n/missing: no such folder" }])
})

test("all failed with no window routes home; with a window it does not", async () => {
  const cold = setup({ "/f": "!not a directory" })
  await cold.opener.open(["/f"])
  assert.deepEqual(cold.fake.routes, [`${ORIGIN}/`])
  assert.deepEqual(cold.fake.reports, [{ message: "Could not open folder", detail: "/f: not a directory" }])

  const warm = setup({ "/f": "!not a directory" }, { windows: 1 })
  await warm.opener.open(["/f"])
  assert.deepEqual(warm.fake.routes, [])
})

test("a daemon that is not ready reports and routes home only without a window", async () => {
  const cold = setup({ "/a": "/a" }, { socket: null })
  const result = await cold.opener.open(["/a"])
  assert.deepEqual(cold.fake.opened, [])
  assert.deepEqual(result, { projects: [], failed: [{ path: "/a", error: DAEMON_NOT_RUNNING }], skipped: 0 })
  assert.deepEqual(cold.fake.reports, [{ message: "Could not open folder", detail: DAEMON_NOT_RUNNING }])
  assert.deepEqual(cold.fake.routes, [`${ORIGIN}/`])

  const warm = setup({ "/a": "/a" }, { socket: null, windows: 2 })
  await warm.opener.open(["/a"])
  assert.deepEqual(warm.fake.routes, [])
})

test("opens at most 8 folders and reports the rest once", async () => {
  const paths = Array.from({ length: 10 }, (_, index) => `/p${index}`)
  const { fake, opener } = setup(Object.fromEntries(paths.map((path) => [path, path])))
  const result = await opener.open(paths)
  assert.deepEqual(fake.opened, paths.slice(0, 8))
  assert.equal(result.projects.length, 8)
  assert.equal(result.skipped, 2)
  assert.deepEqual(fake.reports, [{ message: "servef opens up to 8 folders at a time", detail: "Skipped 2 of 10:\n/p8\n/p9" }])
})

test("no paths does nothing", async () => {
  const { fake, opener } = setup({})
  assert.deepEqual(await opener.open([]), { projects: [], failed: [], skipped: 0 })
  assert.deepEqual(fake.routes, [])
})
