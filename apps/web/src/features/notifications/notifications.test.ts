import assert from "node:assert/strict"
import test from "node:test"

import { defaultSettings } from "../../settings/settings.ts"

import { notificationConfig, notificationProblem, sameNotificationStatus } from "./notifications.ts"

test("the desktop app gets the shown project and each kind's choice", () => {
  assert.deepEqual(notificationConfig("/code/a", defaultSettings), { project: "/code/a", commits: true, branches: true, edits: false, walkthroughs: true })
  assert.deepEqual(
    notificationConfig("/code/b", { ...defaultSettings, notifyCommits: "off", notifyBranches: "off", notifyEdits: "on", notifyWalkthroughs: "off" }),
    { project: "/code/b", commits: false, branches: false, edits: true, walkthroughs: false },
  )
})

test("a problem is named only when notices cannot reach the user", () => {
  assert.equal(notificationProblem(null), null)
  assert.equal(notificationProblem({ supported: true, denied: false }), null)
  assert.match(notificationProblem({ supported: false, denied: false }) ?? "", /no notification service/)
  assert.match(notificationProblem({ supported: true, denied: true }) ?? "", /System Settings › Notifications/)
})

test("statuses compare by value", () => {
  assert.ok(sameNotificationStatus(null, null))
  assert.ok(sameNotificationStatus({ supported: true, denied: false }, { supported: true, denied: false }))
  assert.ok(!sameNotificationStatus(null, { supported: true, denied: false }))
  assert.ok(!sameNotificationStatus({ supported: true, denied: true }, { supported: true, denied: false }))
})
