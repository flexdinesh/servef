import assert from "node:assert/strict"
import test from "node:test"

import { followPresence, type PresenceDeps, type PresenceStream } from "./use-presence.ts"

interface FakeStream extends PresenceStream {
  fail(closed: boolean): void
  isClosed: boolean
}

function harness(answers: (boolean | Error)[]) {
  const streams: FakeStream[] = []
  const timers: (() => void)[] = []
  let asked = 0
  let signal: AbortSignal | null = null
  const deps: PresenceDeps = {
    wanted: (received) => {
      signal = received
      const answer = answers[Math.min(asked, answers.length - 1)]
      asked += 1
      return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer ?? false)
    },
    open: () => {
      let listener = () => {}
      let gaveUp = false
      const stream: FakeStream = {
        isClosed: false,
        closed: () => gaveUp,
        onError: (received) => { listener = received },
        close: () => { stream.isClosed = true },
        fail: (closed) => {
          gaveUp = closed
          listener()
        },
      }
      streams.push(stream)
      return stream
    },
    schedule: (run) => {
      timers.push(run)
      return () => { timers.splice(timers.indexOf(run), 1) }
    },
  }
  return {
    deps, streams, timers,
    asked: () => asked,
    signal: () => signal,
    runTimer: () => { timers.shift()?.() },
  }
}

const settled = () => new Promise((resolve) => setImmediate(resolve))

test("a server that wants presence gets one stream", async () => {
  const fake = harness([true])
  const stop = followPresence(fake.deps)
  await settled()
  assert.equal(fake.streams.length, 1)
  stop()
  assert.equal(fake.streams[0]?.isClosed, true)
})

test("a server that does not want presence gets none, and is not asked again", async () => {
  const fake = harness([false])
  followPresence(fake.deps)
  await settled()
  assert.equal(fake.streams.length, 0)
  assert.equal(fake.timers.length, 0)
  assert.equal(fake.asked(), 1)
})

test("a server that cannot answer is asked again", async () => {
  const fake = harness([new Error("away"), true])
  followPresence(fake.deps)
  await settled()
  assert.equal(fake.streams.length, 0)
  fake.runTimer()
  await settled()
  assert.equal(fake.asked(), 2)
  assert.equal(fake.streams.length, 1)
})

test("a stream that gave up is reopened after a wait", async () => {
  const fake = harness([true])
  followPresence(fake.deps)
  await settled()
  fake.streams[0]?.fail(true)
  assert.equal(fake.streams[0]?.isClosed, true)
  assert.equal(fake.streams.length, 1)
  fake.runTimer()
  assert.equal(fake.streams.length, 2)
  fake.streams[1]?.fail(true)
  fake.runTimer()
  assert.equal(fake.streams.length, 3)
})

test("a stream reconnecting by itself is left alone", async () => {
  const fake = harness([true])
  followPresence(fake.deps)
  await settled()
  fake.streams[0]?.fail(false)
  assert.equal(fake.streams[0]?.isClosed, false)
  assert.equal(fake.timers.length, 0)
})

test("stopping cancels the wait, the question, and any late answer", async () => {
  const waiting = harness([true])
  const stopWaiting = followPresence(waiting.deps)
  await settled()
  waiting.streams[0]?.fail(true)
  stopWaiting()
  assert.equal(waiting.timers.length, 0)

  const asking = harness([true])
  const stopAsking = followPresence(asking.deps)
  stopAsking()
  assert.equal(asking.signal()?.aborted, true)
  await settled()
  assert.equal(asking.streams.length, 0)
})
