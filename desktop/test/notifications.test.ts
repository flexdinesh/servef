import assert from "node:assert/strict"
import { test } from "node:test"
import type { LiveChange } from "../../web/src/api/events-contract.ts"
import { createNoticeGate, createSettleTracker, headNotice, NOTICE_INTERVAL_MS, SETTLE_QUIET_MS, settledNotice, walkthroughNotice } from "../src/main/notifications.ts"
import { createFakeTimers } from "./fake-timers.ts"

const project = "/src/my app"
const enc = "%2Fsrc%2Fmy+app"

const change = (paths: string[], overflow = false): LiveChange => ({ paths, tree: false, git: false, overflow })
const gitOnly: LiveChange = { paths: [], tree: false, git: true, overflow: false }

test("the baseline head and unchanged heads are ignored", () => {
  assert.equal(headNotice(project, { branch: "main", commit: "a", subject: "s", previous: null }), null)
  assert.equal(headNotice(project, { branch: "main", commit: "a", subject: "s", previous: { branch: "main", commit: "a" } }), null)
  assert.equal(headNotice(project, { branch: "main", commit: "", subject: "", previous: { branch: "main", commit: "a" } }), null)
})

test("a branch change is a switch notice", () => {
  assert.deepEqual(headNotice(project, { branch: "feature", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } }), {
    project, kind: "switch", title: "Switched to feature", body: "my app", target: `servef-app://local/?project=${enc}`,
  })
  assert.equal(headNotice(project, { branch: "", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } })?.title, "Switched to detached HEAD")
})

test("a new commit on the same branch is a commit notice", () => {
  assert.deepEqual(headNotice(project, { branch: "main", commit: "b2", subject: "Fix it", previous: { branch: "main", commit: "a1" } }), {
    project, kind: "commit", title: "New commit on main", body: "Fix it", target: `servef-app://local/view?project=${enc}&commit=b2`,
  })
})

test("walkthrough notices distinguish created and updated", () => {
  assert.deepEqual(walkthroughNotice({ project, id: "tour 1", title: "Tour", created: true }), {
    project, kind: "walkthrough", title: "Walkthrough ready: Tour", body: "my app", target: `servef-app://local/view?project=${enc}&wt=tour+1`,
  })
  assert.equal(walkthroughNotice({ project, id: "t", title: "Tour", created: false }).title, "Walkthrough updated: Tour")
})

test("settled notices count files", () => {
  assert.deepEqual(settledNotice(project, 3, false), {
    project, kind: "settled", title: "Edits settled in my app", body: "3 files", target: `servef-app://local/view?project=${enc}&review=working`,
  })
  assert.equal(settledNotice(project, 1, false).body, "1 file")
  assert.equal(settledNotice(project, 256, true).body, "256+ files")
})

test("the settle tracker emits once per burst after the quiet period", () => {
  const clock = createFakeTimers()
  const emitted: [string, number, boolean][] = []
  const tracker = createSettleTracker(SETTLE_QUIET_MS, (p, files, overflow) => emitted.push([p, files, overflow]), clock.timers)
  tracker.change("/a", change(["x", "y"]))
  clock.advance(SETTLE_QUIET_MS - 1)
  tracker.change("/a", change(["y", "z"]))
  clock.advance(SETTLE_QUIET_MS - 1)
  assert.deepEqual(emitted, [])
  clock.advance(1)
  assert.deepEqual(emitted, [["/a", 3, false]])
  clock.advance(SETTLE_QUIET_MS * 3)
  assert.deepEqual(emitted, [["/a", 3, false]])
})

test("changes without paths re-arm a burst but never start one", () => {
  const clock = createFakeTimers()
  const emitted: [string, number, boolean][] = []
  const tracker = createSettleTracker(1_000, (p, files, overflow) => emitted.push([p, files, overflow]), clock.timers)
  tracker.change("/a", gitOnly)
  clock.advance(5_000)
  assert.deepEqual(emitted, [])
  tracker.change("/a", change(["x"]))
  clock.advance(900)
  tracker.change("/a", gitOnly)
  clock.advance(900)
  assert.deepEqual(emitted, [])
  clock.advance(100)
  assert.deepEqual(emitted, [["/a", 1, false]])
})

test("overflow starts a burst and is reported; projects are independent; forget cancels", () => {
  const clock = createFakeTimers()
  const emitted: [string, number, boolean][] = []
  const tracker = createSettleTracker(1_000, (p, files, overflow) => emitted.push([p, files, overflow]), clock.timers)
  tracker.change("/a", change([], true))
  tracker.change("/b", change(["q"]))
  tracker.change("/c", change(["r"]))
  tracker.forget("/c")
  clock.advance(1_000)
  assert.deepEqual(emitted, [["/a", 0, true], ["/b", 1, false]])
  tracker.change("/d", change(["s"]))
  tracker.dispose()
  clock.advance(5_000)
  assert.equal(emitted.length, 2)
  assert.equal(clock.pending(), 0)
})

test("the gate suppresses the focused project and repeats within the interval", () => {
  const clock = createFakeTimers()
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, clock.timers)
  const commit = headNotice("/a", { branch: "main", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } })
  assert.ok(commit !== null)
  assert.equal(gate.allow(commit, "/a"), false, "focused project")
  assert.equal(gate.allow(commit, "/b"), true)
  assert.equal(gate.allow(commit, null), false, "repeat within interval")
  assert.equal(gate.allow(settledNotice("/a", 1, false), null), true, "other kind")
  assert.equal(gate.allow({ ...commit, project: "/c" }, null), true, "other project")
  clock.advance(NOTICE_INTERVAL_MS - 1)
  assert.equal(gate.allow(commit, null), false)
  clock.advance(1)
  assert.equal(gate.allow(commit, null), true)
})
