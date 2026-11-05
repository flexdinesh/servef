import { writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test } from "@playwright/test"

import { createRepository, createSession, ensureDaemon, launchApp, openRoot, removeSession, schemeURL, stopDaemon } from "./harness.ts"

// A 4x4 PNG.
const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAEElEQVR4nGNIaFgARwzEcQBsUhgB7OEKUAAAAABJRU5ErkJggg==", "base64")

// A Markdown image is the page's own origin: the daemon's CSP must allow it on
// a custom scheme, where http: and https: sources match nothing (decision 0094).
test("a Markdown image in a project draws", async () => {
  const session = createSession()
  const app = await (async () => {
    await ensureDaemon(session)
    const directory = createRepository(path.join(session.projects, "pictures"), "Pictures")
    writeFileSync(path.join(directory, "pic.png"), png)
    writeFileSync(path.join(directory, "README.md"), "# Pictures\n\n![pic](pic.png)\n")
    const project = await openRoot(session, directory)
    return launchApp(session, schemeURL(project, "README.md"))
  })()
  try {
    const page = await app.firstWindow()
    await page.getByRole("button", { name: "Preview", exact: true }).click({ timeout: 60_000 })
    const image = page.getByRole("main").getByRole("img", { name: "pic" })
    await expect(image).toBeVisible()
    await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(4)
  } finally {
    await app.close().catch(() => undefined)
    await stopDaemon(session)
    removeSession(session)
  }
})
