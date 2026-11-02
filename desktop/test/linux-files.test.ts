import assert from "node:assert/strict"
import { existsSync, readFileSync } from "node:fs"
import path from "node:path"
import { test } from "node:test"
import { fileURLToPath } from "node:url"
import { debFpmArgs, LINUX_EXTRA_FILES, LINUX_INSTALLED_EXECUTABLE } from "../scripts/linux-files.ts"

const desktop = fileURLToPath(new URL("..", import.meta.url))

function read(source: string): string {
  return readFileSync(path.join(desktop, source), "utf8")
}

test("every deb extra file exists and opens folders with --open-path", () => {
  assert.equal(LINUX_EXTRA_FILES.length, 2)
  for (const file of LINUX_EXTRA_FILES) {
    assert.ok(existsSync(path.join(desktop, file.source)), file.source)
    assert.ok(path.isAbsolute(file.target), file.target)
    assert.ok(read(file.source).split("\n").includes(`Exec=${LINUX_INSTALLED_EXECUTABLE} --open-path %F`), file.source)
  }
})

test("the Dolphin service menu and Nemo action match folders only", () => {
  const [menu, nemo] = LINUX_EXTRA_FILES
  assert.equal(menu.target, "/usr/share/kio/servicemenus/servef-desktop.desktop")
  assert.ok(read(menu.source).split("\n").includes("MimeType=inode/directory;"))
  assert.equal(nemo.target, "/usr/share/nemo/actions/servef-desktop.nemo_action")
  assert.ok(read(nemo.source).split("\n").includes("Extensions=dir;"))
})

test("debFpmArgs maps absolute sources to their targets", () => {
  assert.deepEqual(debFpmArgs("/src/servef/desktop"), [
    "/src/servef/desktop/resources/linux/servef-desktop.servicemenu.desktop=/usr/share/kio/servicemenus/servef-desktop.desktop",
    "/src/servef/desktop/resources/linux/servef-desktop.nemo_action=/usr/share/nemo/actions/servef-desktop.nemo_action",
  ])
})

test("electron-builder.yml declares a macOS folder document type and no folder MIME type", () => {
  const config = read("electron-builder.yml")
  assert.match(config, /LSItemContentTypes: \[public\.folder\]/)
  assert.match(config, /LSHandlerRank: Alternate/)
  assert.doesNotMatch(config, /^[^#]*inode\/directory/m)
})
