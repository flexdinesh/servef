import assert from "node:assert/strict"
import { test } from "node:test"
import { EXPECTED_PROTOCOL, isControlErrorBody, isControlStatus, isDaemonStatus, isDirectoryListing, isOpenResult, isWalkthroughAnnouncement } from "../src/main/daemon-contract.ts"
import { devFlags, ENSURE_TIMEOUT_MS, ensureDaemon, listDirectories, nodeExecFile, openPath, readStatus, stopDaemon } from "../src/main/daemon.ts"
import type { ExecFile, ExecOutcome } from "../src/main/daemon.ts"
import type { Forward, ForwardRequest } from "../src/main/forward.ts"

const status = { protocol: EXPECTED_PROTOCOL, version: "1.2.3", pid: 42, socket: "/run/servef.sock", roots: ["/src"], depth: 3, started: true }

function runner(outcome: ExecOutcome, calls: { file: string; args: readonly string[]; timeoutMs: number }[] = []): ExecFile {
  return async (file, args, options) => {
    calls.push({ file, args, timeoutMs: options.timeoutMs })
    return outcome
  }
}

test("devFlags maps only non-empty dev variables", () => {
  assert.deepEqual(devFlags({}), [])
  assert.deepEqual(devFlags({ SERVEF_DESKTOP_SOCKET: "", SERVEF_DESKTOP_ROOTS_FILE: "" }), [])
  assert.deepEqual(devFlags({ SERVEF_DESKTOP_SOCKET: "/s.sock", SERVEF_DESKTOP_ROOTS_FILE: "/roots.json", SERVEF_OTHER: "x" }), ["--socket", "/s.sock", "--roots-file", "/roots.json"])
})

test("ensure runs daemon ensure --json with flags and accepts a matching status", async () => {
  const calls: { file: string; args: readonly string[]; timeoutMs: number }[] = []
  const result = await ensureDaemon("/bin/servef", ["--socket", "/s"], runner({ code: 0, stdout: `${JSON.stringify(status)}\n`, stderr: "" }, calls))
  assert.deepEqual(calls, [{ file: "/bin/servef", args: ["daemon", "ensure", "--json", "--socket", "/s"], timeoutMs: ENSURE_TIMEOUT_MS }])
  assert.equal(result.ok, true)
  assert.ok(result.ok && result.status.socket === "/run/servef.sock")
})

test("ensure reads the last non-empty stdout line", async () => {
  const result = await ensureDaemon("servef", [], runner({ code: 0, stdout: `starting\n${JSON.stringify(status)}\n\n`, stderr: "" }))
  assert.equal(result.ok, true)
})

test("ensure reports a protocol mismatch from JSON printed with a non-zero exit", async () => {
  const other = { ...status, protocol: EXPECTED_PROTOCOL + 1, version: "9.0.0" }
  const result = await ensureDaemon("servef", [], runner({ code: 1, stdout: JSON.stringify(other), stderr: "protocol mismatch" }))
  assert.equal(result.ok, false)
  assert.ok(!result.ok)
  assert.equal(result.kind, "mismatch")
  assert.equal(result.status?.protocol, EXPECTED_PROTOCOL + 1)
  assert.equal(result.status?.version, "9.0.0")
})

test("ensure with garbage stdout fails with stderr", async () => {
  const result = await ensureDaemon("servef", [], runner({ code: 1, stdout: "not json", stderr: "  daemon did not start; see /s.log\n" }))
  assert.deepEqual(result, { ok: false, kind: "failed", detail: "daemon did not start; see /s.log" })
})

test("ensure bounds a long stderr", async () => {
  const result = await ensureDaemon("servef", [], runner({ code: 1, stdout: "", stderr: `${"x".repeat(10_000)}END` }))
  assert.ok(!result.ok)
  assert.ok(result.detail.length < 2_100)
  assert.ok(result.detail.endsWith("END"))
})

test("ensure with a matching status but non-zero exit fails", async () => {
  const result = await ensureDaemon("servef", [], runner({ code: 2, stdout: JSON.stringify(status), stderr: "boom" }))
  assert.ok(!result.ok)
  assert.equal(result.kind, "failed")
  assert.equal(result.status?.socket, "/run/servef.sock")
})

test("ensure timeout fails with a timeout detail", async () => {
  const result = await ensureDaemon("servef", [], runner({ code: null, stdout: "", stderr: "", error: { message: "killed", timedOut: true } }))
  assert.deepEqual(result, { ok: false, kind: "failed", detail: "servef daemon ensure timed out after 15 s." })
})

test("ensure of a binary that cannot run is missing", async () => {
  for (const code of ["ENOENT", "EACCES"]) {
    const result = await ensureDaemon("/nope/servef", [], runner({ code: null, stdout: "", stderr: "", error: { code, message: `spawn /nope/servef ${code}`, timedOut: false } }))
    assert.ok(!result.ok)
    assert.equal(result.kind, "missing")
    assert.match(result.detail, /\/nope\/servef/)
  }
})

test("nodeExecFile resolves exits, spawn failures, and timeouts without rejecting", async () => {
  const node = process.execPath
  assert.deepEqual(await nodeExecFile(node, ["-e", "process.stdout.write('hi')"], { timeoutMs: 10_000 }), { code: 0, stdout: "hi", stderr: "" })
  const failed = await nodeExecFile(node, ["-e", "process.stderr.write('bad'); process.exit(3)"], { timeoutMs: 10_000 })
  assert.equal(failed.code, 3)
  assert.equal(failed.stderr, "bad")
  assert.equal(failed.error, undefined)
  const missing = await nodeExecFile("/definitely/not/servef", [], { timeoutMs: 10_000 })
  assert.equal(missing.error?.code, "ENOENT")
  assert.equal(missing.error?.timedOut, false)
  const slow = await nodeExecFile(node, ["-e", "setTimeout(() => {}, 20000)"], { timeoutMs: 200 })
  assert.equal(slow.error?.timedOut, true)
})

function fakeForward(response: () => Response | Promise<Response>) {
  const requests: ForwardRequest[] = []
  const forward = async (request: ForwardRequest) => {
    requests.push(request)
    return response()
  }
  return { forward, requests }
}

async function text(body: ReadableStream<Uint8Array> | null): Promise<string> {
  return body === null ? "" : new Response(body).text()
}

test("openPath posts JSON without browser headers and validates the result", async () => {
  const result = { root: "/src", added: true, replaced: ["/src/a"], project: "/src/app" }
  const { forward, requests } = fakeForward(() => Response.json(result, { status: 201 }))
  assert.deepEqual(await openPath(forward, "/s.sock", "/src/app"), { ok: true, result })
  const [request] = requests
  assert.ok(request !== undefined)
  assert.equal(request.method, "POST")
  assert.equal(request.path, "/_daemon/open")
  assert.deepEqual(request.target, { socket: "/s.sock" })
  assert.equal(request.headers.host, "localhost")
  assert.equal(request.headers["content-type"], "application/json")
  assert.equal(request.headers.origin, undefined)
  assert.equal(request.headers["sec-fetch-site"], undefined)
  assert.deepEqual(JSON.parse(await text(request.body)), { path: "/src/app" })
})

test("openPath maps error bodies, invalid answers, and an absent daemon", async () => {
  const rejected = fakeForward(() => Response.json({ error: "not a directory", code: "invalidRoot" }, { status: 400 }))
  assert.deepEqual(await openPath(rejected.forward, "/s", "/x"), { ok: false, status: 400, code: "invalidRoot", error: "not a directory" })
  const invalid = fakeForward(() => Response.json({ root: 1 }))
  const outcome = await openPath(invalid.forward, "/s", "/x")
  assert.ok(!outcome.ok)
  assert.equal(outcome.status, 200)
  const gone = await openPath(async () => { throw Object.assign(new Error("connect ENOENT"), { code: "ENOENT" }) }, "/s", "/x")
  assert.ok(!gone.ok)
  assert.equal(gone.status, 0)
  assert.equal(gone.code, "daemonUnavailable")
})

test("stopDaemon posts /_daemon/stop and waits until the daemon is gone", async () => {
  const requests: ForwardRequest[] = []
  let answers = 2
  const forward: Forward = async (request) => {
    requests.push(request)
    if (request.path === "/_daemon/stop") return Response.json({ stopping: true }, { status: 202 })
    if (answers-- > 0) return Response.json(status)
    throw Object.assign(new Error("connect ECONNREFUSED"), { code: "ECONNREFUSED" })
  }
  assert.equal(await stopDaemon(forward, "/s"), true)
  assert.equal(requests[0]?.path, "/_daemon/stop")
  assert.equal(requests[0]?.method, "POST")
  assert.equal(requests[0]?.headers.origin, undefined)
  assert.deepEqual(requests.slice(1).map((request) => request.path), ["/_daemon/status", "/_daemon/status", "/_daemon/status"])
  const stays = fakeForward((): Response => Response.json(status, { status: 202 }))
  assert.equal(await stopDaemon(stays.forward, "/s", 250), false)
  assert.equal(await stopDaemon(fakeForward(() => Response.json({ error: "x" }, { status: 403 })).forward, "/s"), false)
  assert.equal(await stopDaemon(async () => { throw new Error("gone") }, "/s"), false)
})

test("contract guards accept daemon shapes and reject others", () => {
  assert.ok(isDaemonStatus(status))
  assert.ok(isDaemonStatus({ ...status, started: undefined }))
  assert.ok(!isDaemonStatus({ ...status, roots: [1] }))
  assert.ok(!isDaemonStatus({ ...status, protocol: "1" }))
  assert.ok(!isDaemonStatus({ ...status, started: "yes" }))
  assert.ok(isOpenResult({ root: "/a", added: false, replaced: [], project: "/a" }))
  assert.ok(!isOpenResult({ root: "/a", added: false, replaced: null, project: "/a" }))
  assert.ok(isControlStatus({ protocol: 1, version: "x" }))
  assert.ok(!isControlStatus({ protocol: 1.5, version: "x" }))
  assert.ok(isWalkthroughAnnouncement({ project: "/p", id: "w", title: "T", created: true }))
  assert.ok(!isWalkthroughAnnouncement({ project: "/p", id: "w", title: "T" }))
  assert.ok(isControlErrorBody({ error: "x" }))
  assert.ok(isControlErrorBody({ error: "x", code: "c" }))
  assert.ok(!isControlErrorBody({ error: "x", code: 1 }))
  assert.ok(!isControlErrorBody([]))
})

test("listDirectories gets the listing with the path in the query, without browser headers", async () => {
  const listing = { path: "/home/dee", parent: "/home", entries: [{ name: "a b", path: "/home/dee/a b", repository: true, hidden: false }], truncated: false }
  const { forward, requests } = fakeForward(() => Response.json(listing))
  assert.deepEqual(await listDirectories(forward, "/s.sock", "/home/dee/a&b c"), { ok: true, listing })
  const [request] = requests
  assert.ok(request !== undefined)
  assert.equal(request.method, "GET")
  assert.equal(request.path, "/_daemon/directories?path=%2Fhome%2Fdee%2Fa%26b+c")
  assert.deepEqual(request.target, { socket: "/s.sock" })
  assert.deepEqual(request.headers, { host: "localhost" })
})

test("listDirectories maps errors; a 404 without a code means an old servef", async () => {
  const missing = fakeForward(() => Response.json({ error: "no such directory", code: "pathUnavailable" }, { status: 404 }))
  assert.deepEqual(await listDirectories(missing.forward, "/s", "/x"), { ok: false, status: 404, code: "pathUnavailable", error: "no such directory", upgrade: false })
  const old = fakeForward(() => Response.json({ error: "Unknown daemon endpoint." }, { status: 404 }))
  assert.deepEqual(await listDirectories(old.forward, "/s", ""), { ok: false, status: 404, code: "", error: "Unknown daemon endpoint.", upgrade: true })
  const invalid = fakeForward(() => Response.json({ path: "/", entries: "x" }))
  const outcome = await listDirectories(invalid.forward, "/s", "")
  assert.ok(!outcome.ok)
  assert.equal(outcome.code, "invalidResponse")
  const gone = await listDirectories(async () => { throw Object.assign(new Error("connect ENOENT"), { code: "ENOENT" }) }, "/s", "")
  assert.ok(!gone.ok)
  assert.equal(gone.code, "daemonUnavailable")
})

test("readStatus validates the daemon status; anything else is null", async () => {
  assert.deepEqual(await readStatus(fakeForward(() => Response.json(status)).forward, "/s"), status)
  assert.equal(await readStatus(fakeForward(() => Response.json({ pid: 1 })).forward, "/s"), null)
  assert.equal(await readStatus(async () => { throw new Error("down") }, "/s"), null)
})

test("isDirectoryListing checks every entry", () => {
  const listing = { path: "/", parent: "", entries: [{ name: "a", path: "/a", repository: false, hidden: false }], truncated: true }
  assert.ok(isDirectoryListing(listing))
  assert.ok(!isDirectoryListing({ ...listing, entries: [{ name: "a", path: "/a", repository: "no", hidden: false }] }))
  assert.ok(!isDirectoryListing({ ...listing, parent: null }))
})
