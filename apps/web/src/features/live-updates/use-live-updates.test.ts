import assert from "node:assert/strict"
import test from "node:test"

import type { LiveChange, LiveStatus } from "../../api/events-contract.ts"
import { everything } from "./live-updates.ts"
import { followLiveUpdates, type LiveDeps, type LiveStream, type LiveStreamHandlers } from "./use-live-updates.ts"

interface FakeStream extends LiveStream {
  handlers: LiveStreamHandlers
  isClosed: boolean
  fail(closed: boolean): void
}

const watching: LiveStatus = { mode: "watching", reason: "" }
const edit: LiveChange = { paths: ["a.md"], tree: false, git: false, overflow: false }

function harness(start: { visible?: boolean } = {}) {
  const streams: FakeStream[] = []
  const timers: (() => void)[] = []
  const statuses: LiveStatus[] = []
  const changes: LiveChange[] = []
  const state = { visible: start.visible ?? true, now: 100_000 }
  const deps: LiveDeps = {
    open(handlers) {
      let gaveUp = false
      const stream: FakeStream = {
        handlers,
        isClosed: false,
        closed: () => gaveUp,
        close: () => { stream.isClosed = true },
        fail(closed) {
          gaveUp = closed
          handlers.error()
        },
      }
      streams.push(stream)
      return stream
    },
    visible: () => state.visible,
    now: () => state.now,
    schedule(run) {
      timers.push(run)
      return () => { timers.splice(timers.indexOf(run), 1) }
    },
    status: (value) => { statuses.push(value) },
    change: (value) => { changes.push(value) },
  }
  return { deps, streams, timers, statuses, changes, state, runTimer: () => { timers.shift()?.() } }
}

test("a visible page holds one stream and passes its changes on", () => {
  const fake = harness()
  followLiveUpdates(fake.deps)
  assert.equal(fake.streams.length, 1)
  fake.streams[0]?.handlers.status(watching)
  fake.streams[0]?.handlers.change(edit)
  assert.deepEqual(fake.statuses, [watching])
  assert.deepEqual(fake.changes, [edit])
})

test("a stream the browser gave up on is reopened, and rereads what it missed", () => {
  const fake = harness()
  followLiveUpdates(fake.deps)
  fake.streams[0]?.fail(true)
  assert.equal(fake.streams[0]?.isClosed, true)
  assert.equal(fake.statuses.at(-1)?.mode, "off")
  assert.equal(fake.streams.length, 1)

  fake.runTimer()
  assert.equal(fake.streams.length, 2)
  // The server is still away: keep trying.
  fake.streams[1]?.fail(true)
  fake.runTimer()
  assert.equal(fake.streams.length, 3)

  assert.deepEqual(fake.changes, [])
  fake.streams[2]?.handlers.status(watching)
  assert.deepEqual(fake.changes, [everything])
  assert.deepEqual(fake.statuses.at(-1), watching)
})

test("a stream reconnecting by itself is left alone, and rereads when it is back", () => {
  const fake = harness()
  followLiveUpdates(fake.deps)
  fake.streams[0]?.fail(false)
  assert.equal(fake.streams[0]?.isClosed, false)
  assert.equal(fake.timers.length, 0)
  assert.equal(fake.statuses.at(-1)?.mode, "off")
  fake.streams[0]?.handlers.status(watching)
  assert.deepEqual(fake.changes, [everything])
})

test("hiding closes the stream and cancels a wait; returning rereads and reopens", () => {
  const fake = harness()
  const follower = followLiveUpdates(fake.deps)
  fake.streams[0]?.fail(true)
  fake.state.visible = false
  follower.visibility()
  assert.equal(fake.timers.length, 0)

  fake.state.visible = true
  follower.visibility()
  assert.equal(fake.streams.length, 2)
  assert.deepEqual(fake.changes, [everything])
})

test("a page that starts hidden opens nothing until it is shown", () => {
  const fake = harness({ visible: false })
  const follower = followLiveUpdates(fake.deps)
  assert.equal(fake.streams.length, 0)
  fake.state.visible = true
  follower.visibility()
  assert.equal(fake.streams.length, 1)
})

test("focus rereads only without a working stream, at most every two seconds", () => {
  const fake = harness()
  const follower = followLiveUpdates(fake.deps)
  fake.streams[0]?.handlers.status(watching)
  follower.focus()
  assert.deepEqual(fake.changes, [])

  fake.streams[0]?.handlers.status({ mode: "off", reason: "limit" })
  follower.focus()
  follower.focus()
  assert.deepEqual(fake.changes, [everything])
  fake.state.now += 2_000
  follower.focus()
  assert.deepEqual(fake.changes, [everything, everything])
})

test("stopping closes the stream and cancels the wait", () => {
  const open = harness()
  const stopOpen = followLiveUpdates(open.deps)
  stopOpen.stop()
  assert.equal(open.streams[0]?.isClosed, true)

  const waiting = harness()
  const stopWaiting = followLiveUpdates(waiting.deps)
  waiting.streams[0]?.fail(true)
  stopWaiting.stop()
  assert.equal(waiting.timers.length, 0)
})
