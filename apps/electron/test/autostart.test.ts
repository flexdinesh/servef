import assert from "node:assert/strict"
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { test } from "node:test"
import { AUTOSTART_FILE, autostartEnabled, autostartEntry, autostartPlan, createXdgAutostart, desktopExec, LEGACY_AUTOSTART_FILE, migrateLegacyAutostart } from "../src/main/autostart.ts"

test("desktopExec quotes reserved characters and escapes per the Desktop Entry spec", () => {
  assert.equal(desktopExec(["/opt/servef/servef-electron"]), "/opt/servef/servef-electron")
  assert.equal(desktopExec(["/home/u/My Apps/servef.AppImage"]), `"/home/u/My Apps/servef.AppImage"`)
  assert.equal(desktopExec(["/a\"b"]), `"/a\\\\"b"`)
  assert.equal(desktopExec(["/a`b"]), `"/a\\\\\`b"`)
  assert.equal(desktopExec(["/a$b"]), `"/a\\\\$b"`)
  // A literal backslash takes four backslashes in a quoted argument.
  assert.equal(desktopExec(["/a\\b"]), `"/a\\\\\\\\b"`)
  assert.equal(desktopExec(["/100%"]), "/100%%")
  assert.equal(desktopExec(["/a b%"]), `"/a b%%"`)
  assert.equal(desktopExec(["/a\nb"]), `"/a\\nb"`)
  assert.equal(desktopExec(["/usr/lib/electron", "/src/servef/desktop"]), "/usr/lib/electron /src/servef/desktop")
  assert.equal(desktopExec(["/x", ""]), `/x ""`)
})

test("autostartEntry is the exact entry", () => {
  assert.equal(autostartEntry(["/opt/servef/servef-electron"]), [
    "[Desktop Entry]",
    "Type=Application",
    "Name=servef",
    "Comment=Open servef at login",
    "Exec=/opt/servef/servef-electron",
    "Icon=servef-electron",
    "Terminal=false",
    "X-GNOME-Autostart-enabled=true",
    "",
  ].join("\n"))
})

test("autostartPlan picks the directory and command per install", () => {
  const base = { home: "/home/u", execPath: "/opt/servef/servef-electron", appPath: "/opt/servef/resources/app.asar" }
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: {} }), { dir: "/home/u/.config/autostart", argv: ["/opt/servef/servef-electron"] })
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: { XDG_CONFIG_HOME: "/cfg" } }), { dir: "/cfg/autostart", argv: ["/opt/servef/servef-electron"] })
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: { XDG_CONFIG_HOME: "cfg" } }), { dir: "/home/u/.config/autostart", argv: ["/opt/servef/servef-electron"] })
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: { APPIMAGE: "/home/u/Apps/servef.AppImage" } }), { dir: "/home/u/.config/autostart", argv: ["/home/u/Apps/servef.AppImage"] })
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: { APPIMAGE: "servef.AppImage" } }), { dir: "/home/u/.config/autostart", argv: ["/opt/servef/servef-electron"] })
  // The seam is honored only unpackaged.
  assert.deepEqual(autostartPlan({ ...base, packaged: true, env: { SERVEF_DESKTOP_AUTOSTART_DIR: "/tmp/a" } }), { dir: "/home/u/.config/autostart", argv: ["/opt/servef/servef-electron"] })
  assert.deepEqual(autostartPlan({ ...base, packaged: false, env: { SERVEF_DESKTOP_AUTOSTART_DIR: "/tmp/a" } }), { dir: "/tmp/a", argv: [base.execPath, base.appPath] })
  assert.equal(autostartPlan({ ...base, packaged: false, env: { SERVEF_DESKTOP_AUTOSTART_DIR: "rel" } }), null)
  assert.equal(autostartPlan({ ...base, packaged: false, env: {} }), null)
})

test("autostartEnabled reads the Desktop Entry group", () => {
  assert.equal(autostartEnabled(null), false)
  assert.equal(autostartEnabled(""), false)
  assert.equal(autostartEnabled("junk\n"), false)
  assert.equal(autostartEnabled(autostartEntry(["/x"])), true)
  assert.equal(autostartEnabled("[Desktop Entry]\nExec=/x\nHidden=true\n"), false)
  assert.equal(autostartEnabled("[Desktop Entry]\nExec=/x\nX-GNOME-Autostart-enabled=false\n"), false)
  assert.equal(autostartEnabled("[Desktop Entry]\nExec=/x\n\n[Desktop Action a]\nHidden=true\n"), true)
  assert.equal(autostartEnabled("[Other]\nExec=/x\n"), false)
})

test("createXdgAutostart writes, rewrites, and removes only its entry", () => {
  const root = mkdtempSync(path.join(tmpdir(), "servef-autostart-"))
  try {
    const dir = path.join(root, "config", "autostart")
    const autostart = createXdgAutostart({ dir, argv: ["/opt/servef/servef-electron"] })
    const file = path.join(dir, AUTOSTART_FILE)
    assert.equal(autostart.file, file)
    assert.equal(autostart.enabled(), false)
    autostart.set(false)

    autostart.set(true)
    assert.equal(autostart.enabled(), true)
    assert.equal(readFileSync(file, "utf8"), autostartEntry(["/opt/servef/servef-electron"]))
    assert.equal(statSync(file).mode & 0o777, 0o644)

    writeFileSync(path.join(dir, "other.desktop"), "[Desktop Entry]\n")
    autostart.set(false)
    assert.equal(autostart.enabled(), false)
    assert.deepEqual(statSync(path.join(dir, "other.desktop")).isFile(), true)

    // A copy GNOME Tweaks wrote under the same id is replaced.
    writeFileSync(file, "[Desktop Entry]\nExec=/old\nX-GNOME-Autostart-enabled=false\n")
    assert.equal(autostart.enabled(), false)
    autostart.set(true)
    assert.equal(readFileSync(file, "utf8"), autostartEntry(["/opt/servef/servef-electron"]))
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test("createXdgAutostart throws when it cannot write", () => {
  const root = mkdtempSync(path.join(tmpdir(), "servef-autostart-"))
  try {
    writeFileSync(path.join(root, "file"), "")
    const autostart = createXdgAutostart({ dir: path.join(root, "file", "autostart"), argv: ["/x"] })
    assert.throws(() => autostart.set(true))
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

// The entry the app wrote before the rename (decision 0092).
function legacyEntry(exec: string): string {
  return [
    "[Desktop Entry]",
    "Type=Application",
    "Name=servef",
    "Comment=Open servef at login",
    `Exec=${exec}`,
    "Icon=servef-desktop",
    "Terminal=false",
    "X-GNOME-Autostart-enabled=true",
    "",
  ].join("\n")
}

function withAutostartDir(run: (dir: string, legacy: string, current: string) => void): void {
  const root = mkdtempSync(path.join(tmpdir(), "servef-autostart-"))
  try {
    const dir = path.join(root, "autostart")
    mkdirSync(dir)
    run(dir, path.join(dir, LEGACY_AUTOSTART_FILE), path.join(dir, AUTOSTART_FILE))
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}

const argv = ["/opt/servef/servef-electron"]

test("migrateLegacyAutostart moves an entry the app wrote to the current id and executable", () => {
  withAutostartDir((dir, legacy, current) => {
    writeFileSync(legacy, legacyEntry(`"/home/u/My Apps/servef-desktop.AppImage"`))
    const plan = { dir, argv }
    const autostart = createXdgAutostart(plan)
    migrateLegacyAutostart(plan, autostart)
    assert.equal(existsSync(legacy), false)
    assert.equal(readFileSync(current, "utf8"), autostartEntry(argv))
    assert.equal(autostart.enabled(), true)
  })
})

test("migrateLegacyAutostart leaves an old entry anything else wrote or edited, and registers nothing", () => {
  withAutostartDir((dir, legacy, current) => {
    const plan = { dir, argv }
    for (const foreign of [
      "[Desktop Entry]\nExec=/opt/servef/servef-desktop\n",
      legacyEntry("/opt/servef/servef-desktop").replace("X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false"),
      `${legacyEntry("/opt/servef/servef-desktop")}Hidden=true\n`,
    ]) {
      writeFileSync(legacy, foreign)
      migrateLegacyAutostart(plan, createXdgAutostart(plan))
      assert.equal(readFileSync(legacy, "utf8"), foreign)
      assert.equal(existsSync(current), false)
    }
  })
})

test("migrateLegacyAutostart removes the old entry but keeps one already under the current id", () => {
  withAutostartDir((dir, legacy, current) => {
    const plan = { dir, argv }
    writeFileSync(legacy, legacyEntry("/opt/servef/servef-desktop"))
    writeFileSync(current, "[Desktop Entry]\nExec=/custom\n")
    migrateLegacyAutostart(plan, createXdgAutostart(plan))
    assert.equal(existsSync(legacy), false)
    assert.equal(readFileSync(current, "utf8"), "[Desktop Entry]\nExec=/custom\n")
  })
})

test("migrateLegacyAutostart does nothing without an old entry", () => {
  withAutostartDir((dir, _legacy, current) => {
    const plan = { dir, argv }
    migrateLegacyAutostart(plan, createXdgAutostart(plan))
    assert.equal(existsSync(current), false)
  })
})
