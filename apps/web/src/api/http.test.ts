import assert from "node:assert/strict"
import test from "node:test"

import { failureReason, fetchJSON, isAbort, isRecord, RequestError, type RequestFailure } from "./http.ts"

interface Greeting {
  greeting: string
}

function isGreeting(value: unknown): value is Greeting {
  return isRecord(value) && typeof value.greeting === "string"
}

const describe = (failure: RequestFailure) => `Couldn't greet: ${failureReason(failure)}.`

function respond(body: string, status = 200): () => Promise<Response> {
  return async () => new Response(body, { status, headers: { "Content-Type": "application/json" } })
}

async function failure(read: Promise<unknown>): Promise<RequestError> {
  try {
    await read
  } catch (error: unknown) {
    if (error instanceof RequestError) return error
    throw error
  }
  throw new Error("the read did not fail")
}

test("a valid payload is returned and the request asks for JSON", async (t) => {
  const fetchMock = t.mock.method(globalThis, "fetch", respond('{"greeting":"hi"}'))
  assert.deepEqual(await fetchJSON("/api/greeting", { headers: { "X-Test": "1" } }, isGreeting, describe), { greeting: "hi" })
  const init = fetchMock.mock.calls[0]?.arguments[1]
  assert.deepEqual(init?.headers, { Accept: "application/json", "X-Test": "1" })
})

test("a network failure becomes an unreachable message, never the browser's text", async (t) => {
  t.mock.method(globalThis, "fetch", async () => { throw new TypeError("Failed to fetch") })
  const error = await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))
  assert.equal(error.message, "Couldn't greet: the server didn't respond.")
  assert.deepEqual(error.failure, { kind: "unreachable" })
})

test("a body that is not JSON becomes a malformed failure, never a SyntaxError", async (t) => {
  t.mock.method(globalThis, "fetch", respond("<html>Bad gateway</html>", 502))
  const error = await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))
  assert.equal(error.message, "Couldn't greet: the server sent an invalid response.")
  assert.deepEqual(error.failure, { kind: "malformed", status: 502, ok: false })
})

test("a server error body gives its reason and code", async (t) => {
  t.mock.method(globalThis, "fetch", respond('{"error":"no changes to a.go","code":"unchanged"}', 404))
  const error = await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))
  assert.equal(error.message, "Couldn't greet: no changes to a.go.")
  assert.deepEqual(error.failure, { kind: "rejected", status: 404, error: "no changes to a.go", code: "unchanged" })

  t.mock.method(globalThis, "fetch", respond('{"error":"That folder is unavailable."}', 404))
  assert.deepEqual((await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))).failure,
    { kind: "rejected", status: 404, error: "That folder is unavailable.", code: "" })
})

test("an unexpected payload is invalid, including malformed error bodies", async (t) => {
  t.mock.method(globalThis, "fetch", respond('{"greeting":3}'))
  const error = await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))
  assert.equal(error.message, "Couldn't greet: the server sent an invalid response.")
  assert.deepEqual(error.failure, { kind: "invalid", status: 200, ok: true })

  for (const body of ['{"error":"x","code":3}', '{"error":""}', '{"message":"x"}', '["error"]']) {
    t.mock.method(globalThis, "fetch", respond(body, 500))
    assert.deepEqual((await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))).failure, { kind: "invalid", status: 500, ok: false }, body)
  }
})

test("a valid payload with an error status is a failure unless any status is accepted", async (t) => {
  t.mock.method(globalThis, "fetch", respond('{"greeting":"gone"}', 404))
  assert.deepEqual((await failure(fetchJSON("/api/greeting", {}, isGreeting, describe))).failure, { kind: "invalid", status: 404, ok: false })
  assert.deepEqual(await fetchJSON("/api/greeting", { anyStatus: true }, isGreeting, describe), { greeting: "gone" })
})

test("an abort rejects with the AbortError itself", async (t) => {
  t.mock.method(globalThis, "fetch", async (_url: string, init?: RequestInit) => {
    init?.signal?.throwIfAborted()
    return new Response('{"greeting":"late"}')
  })
  const controller = new AbortController()
  controller.abort()
  const read = fetchJSON("/api/greeting", { signal: controller.signal }, isGreeting, describe)
  await assert.rejects(read, (error: unknown) => isAbort(error) && !(error instanceof RequestError))
})

test("an abort while the body is read rejects with the AbortError itself", async (t) => {
  const controller = new AbortController()
  class AbortedBody extends Response {
    override async json(): Promise<unknown> {
      controller.abort()
      throw new DOMException("The operation was aborted.", "AbortError")
    }
  }
  t.mock.method(globalThis, "fetch", async () => new AbortedBody("{}"))
  await assert.rejects(fetchJSON("/api/greeting", { signal: controller.signal }, isGreeting, describe), (error: unknown) => isAbort(error))
})

test("records are plain objects", () => {
  assert.equal(isRecord({}), true)
  assert.equal(isRecord([]), false)
  assert.equal(isRecord(null), false)
  assert.equal(isRecord("x"), false)
})
