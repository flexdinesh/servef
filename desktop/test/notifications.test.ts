import assert from "node:assert/strict"
import { test } from "node:test"
import type { LiveChange } from "../../web/src/api/events-contract.ts"
import { createNoticeGate, createSettleTracker, headNotice, NOTICE_INTERVAL_MS, SETTLE_QUIET_MS, settledNotice, walkthroughNotice } from "../src/main/notifications.ts"
import type { NoticeSource } from "../src/main/notifications.ts"
import { createFakeTimers } from "./fake-timers.ts"

const project = "/src/my app"
const enc = "%2Fsrc%2Fmy+app"
const local: NoticeSource = { connection: "local", label: "" }
const box: NoticeSource = { connection: "ssh-devbox", label: "devbox" }

const change = (paths: string[], overflow = false): LiveChange => ({ paths, tree: false, git: false, overflow })
const gitOnly: LiveChange = { paths: [], tree: false, git: true, overflow: false }
const commitHead = { branch: "main", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } }

test("the baseline head and unchanged heads are ignored", () => {
  assert.equal(headNotice(local, project, { branch: "main", commit: "a", subject: "s", previous: null }), null)
  assert.equal(headNotice(local, project, { branch: "main", commit: "a", subject: "s", previous: { branch: "main", commit: "a" } }), null)
  assert.equal(headNotice(local, project, { branch: "main", commit: "", subject: "", previous: { branch: "main", commit: "a" } }), null)
})

test("a branch change is a switch notice", () => {
  assert.deepEqual(headNotice(local, project, { branch: "feature", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } }), {
    connection: "local", project, kind: "switch", title: "Switched to feature", body: "my app", target: `servef-app://local/?project=${enc}`,
  })
  assert.equal(headNotice(local, project, { branch: "", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } })?.title, "Switched to detached HEAD")
})

test("a new commit on the same branch is a commit notice", () => {
  assert.deepEqual(headNotice(local, project, { branch: "main", commit: "b2", subject: "Fix it", previous: { branch: "main", commit: "a1" } }), {
    connection: "local", project, kind: "commit", title: "New commit on main", body: "Fix it", target: `servef-app://local/view?project=${enc}&commit=b2`,
  })
})

test("walkthrough notices distinguish created and updated", () => {
  assert.deepEqual(walkthroughNotice(local, { project, id: "tour 1", title: "Tour", created: true }), {
    connection: "local", project, kind: "walkthrough", title: "Walkthrough ready: Tour", body: "my app", target: `servef-app://local/view?project=${enc}&wt=tour+1`,
  })
  assert.equal(walkthroughNotice(local, { project, id: "t", title: "Tour", created: false }).title, "Walkthrough updated: Tour")
})

test("settled notices count files", () => {
  assert.deepEqual(settledNotice(local, project, 3, false), {
    connection: "local", project, kind: "settled", title: "Edits settled in my app", body: "3 files", target: `servef-app://local/view?project=${enc}&review=working`,
  })
  assert.equal(settledNotice(local, project, 1, false).body, "1 file")
  assert.equal(settledNotice(local, project, 256, true).body, "256+ files")
})

test("remote notices target their connection origin and name the host", () => {
  assert.deepEqual(headNotice(box, project, { branch: "feature", commit: "b", subject: "s", previous: { branch: "main", commit: "a" } }), {
    connection: "ssh-devbox", project, kind: "switch", title: "Switched to feature", body: "my app on devbox", target: `servef-app://ssh-devbox/?project=${enc}`,
  })
  assert.equal(headNotice(box, project, commitHead)?.target, `servef-app://ssh-devbox/view?project=%2Fsrc%2Fmy+app&commit=b`)
  assert.deepEqual(walkthroughNotice(box, { project, id: "t", title: "Tour", created: true }), {
    connection: "ssh-devbox", project, kind: "walkthrough", title: "Walkthrough ready: Tour", body: "my app on devbox", target: `servef-app://ssh-devbox/view?project=${enc}&wt=t`,
  })
  assert.deepEqual(settledNotice(box, project, 2, false), {
    connection: "ssh-devbox", project, kind: "settled", title: "Edits settled in my app on devbox", body: "2 files", target: `servef-app://ssh-devbox/view?project=${enc}&review=working`,
  })
})

test("the settle tracker emits once per burst after the quiet period", () => {
  const clock = createFakeTimers()
  const emitted: [string, string, number, boolean][] = []
  const tracker = createSettleTracker(SETTLE_QUIET_MS, (c, p, files, overflow) => emitted.push([c, p, files, overflow]), clock.timers)
  tracker.change("local", "/a", change(["x", "y"]))
  clock.advance(SETTLE_QUIET_MS - 1)
  tracker.change("local", "/a", change(["y", "z"]))
  clock.advance(SETTLE_QUIET_MS - 1)
  assert.deepEqual(emitted, [])
  clock.advance(1)
  assert.deepEqual(emitted, [["local", "/a", 3, false]])
  clock.advance(SETTLE_QUIET_MS * 3)
  assert.deepEqual(emitted, [["local", "/a", 3, false]])
})

test("changes without paths re-arm a burst but never start one", () => {
  const clock = createFakeTimers()
  const emitted: [string, string, number, boolean][] = []
  const tracker = createSettleTracker(1_000, (c, p, files, overflow) => emitted.push([c, p, files, overflow]), clock.timers)
  tracker.change("local", "/a", gitOnly)
  clock.advance(5_000)
  assert.deepEqual(emitted, [])
  tracker.change("local", "/a", change(["x"]))
  clock.advance(900)
  tracker.change("local", "/a", gitOnly)
  clock.advance(900)
  assert.deepEqual(emitted, [])
  clock.advance(100)
  assert.deepEqual(emitted, [["local", "/a", 1, false]])
})

test("overflow starts a burst and is reported; projects are independent; forget cancels", () => {
  const clock = createFakeTimers()
  const emitted: [string, string, number, boolean][] = []
  const tracker = createSettleTracker(1_000, (c, p, files, overflow) => emitted.push([c, p, files, overflow]), clock.timers)
  tracker.change("local", "/a", change([], true))
  tracker.change("local", "/b", change(["q"]))
  tracker.change("local", "/c", change(["r"]))
  tracker.forget("local", "/c")
  clock.advance(1_000)
  assert.deepEqual(emitted, [["local", "/a", 0, true], ["local", "/b", 1, false]])
  tracker.change("local", "/d", change(["s"]))
  tracker.dispose()
  clock.advance(5_000)
  assert.equal(emitted.length, 2)
  assert.equal(clock.pending(), 0)
})

test("bursts key by connection and project", () => {
  const clock = createFakeTimers()
  const emitted: [string, string, number, boolean][] = []
  const tracker = createSettleTracker(1_000, (c, p, files, overflow) => emitted.push([c, p, files, overflow]), clock.timers)
  tracker.change("local", "/a", change(["x"]))
  tracker.change("ssh-devbox", "/a", change(["y", "z"]))
  tracker.change("ssh-other", "/a", change(["w"]))
  tracker.forget("ssh-other", "/a")
  clock.advance(1_000)
  assert.deepEqual(emitted, [["local", "/a", 1, false], ["ssh-devbox", "/a", 2, false]])
})

test("the gate suppresses the focused project and repeats within the interval", () => {
  const clock = createFakeTimers()
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, clock.timers)
  const commit = headNotice(local, "/a", commitHead)
  assert.ok(commit !== null)
  assert.equal(gate.allow(commit, { connection: "local", project: "/a" }), false, "focused project")
  assert.equal(gate.allow(commit, { connection: "local", project: "/b" }), true)
  assert.equal(gate.allow(commit, null), false, "repeat within interval")
  assert.equal(gate.allow(settledNotice(local, "/a", 1, false), null), true, "other kind")
  assert.equal(gate.allow({ ...commit, project: "/c" }, null), true, "other project")
  clock.advance(NOTICE_INTERVAL_MS - 1)
  assert.equal(gate.allow(commit, null), false)
  clock.advance(1)
  assert.equal(gate.allow(commit, null), true)
})

test("focus suppression compares connection and project", () => {
  const clock = createFakeTimers()
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, clock.timers)
  const remote = headNotice(box, "/a", commitHead)
  assert.ok(remote !== null)
  assert.equal(gate.allow(remote, { connection: "ssh-devbox", project: "/a" }), false, "focused remote project")
  assert.equal(gate.allow(remote, { connection: "ssh-devbox", project: null }), true, "focused remote window without a project")
  const other = headNotice(box, "/b", commitHead)
  assert.ok(other !== null)
  assert.equal(gate.allow(other, { connection: "local", project: "/b" }), true, "same project path on another connection")
})

test("the gate keys repeats by connection, project, and kind", () => {
  const clock = createFakeTimers()
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, clock.timers)
  const localCommit = headNotice(local, "/a", commitHead)
  const remoteCommit = headNotice(box, "/a", commitHead)
  assert.ok(localCommit !== null && remoteCommit !== null)
  assert.equal(gate.allow(localCommit, null), true)
  assert.equal(gate.allow(remoteCommit, null), true, "same project and kind on another connection")
  assert.equal(gate.allow(remoteCommit, null), false, "repeat on the same connection")
  assert.equal(gate.allow(localCommit, null), false)
})
