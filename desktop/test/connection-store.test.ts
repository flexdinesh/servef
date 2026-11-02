import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { test } from "node:test"
import { assignId, createFileStore, parseStore, serializeStore } from "../src/main/connection-store.ts"
import type { KnownConnection } from "../src/main/connection-store.ts"

const devbox: KnownConnection = {
  id: "ssh-devbox",
  destination: "devbox",
  port: null,
  hints: [{ hostname: "devbox", user: "dee", addr: "10.0.0.5" }],
  addedAt: "2026-11-01T00:00:00Z",
}
const work: KnownConnection = { id: "ssh-dee-work", destination: "dee@work", port: 2222, hints: [], addedAt: "2026-11-02T00:00:00Z" }

test("serialize and parse round trip", () => {
  const text = serializeStore([devbox, work])
  assert.deepEqual(JSON.parse(text), { version: 1, ssh: [devbox, work] })
  assert.deepEqual(parseStore(text), [devbox, work])
})

test("parseStore: malformed JSON or unknown version is null", () => {
  assert.equal(parseStore("{"), null)
  assert.equal(parseStore("[]"), null)
  assert.equal(parseStore(JSON.stringify({ version: 2, ssh: [] })), null)
  assert.equal(parseStore(JSON.stringify({ ssh: [] })), null)
  assert.equal(parseStore(JSON.stringify({ version: 1 })), null)
  assert.deepEqual(parseStore(JSON.stringify({ version: 1, ssh: [] })), [])
})

test("parseStore drops invalid entries and duplicate ids", () => {
  const entries = [
    devbox,
    { ...devbox, destination: "other" },
    { ...work, id: "devbox" },
    { ...work, id: "ssh-UPPER" },
    { ...work, id: `ssh-${"a".repeat(46)}` },
    { ...work, id: "ssh-x1", destination: "-oProxyCommand=x" },
    { ...work, id: "ssh-x2", destination: "dev box" },
    { ...work, id: "ssh-x3", port: 0 },
    { ...work, id: "ssh-x4", port: 65_536 },
    { ...work, id: "ssh-x5", port: "22" },
    { ...work, id: "ssh-x6", hints: {} },
    { ...work, id: "ssh-x7", addedAt: 5 },
    "string",
    null,
    work,
  ]
  assert.deepEqual(parseStore(JSON.stringify({ version: 1, ssh: entries })), [devbox, work])
})

test("parseStore sanitizes hints", () => {
  const hints = [
    { hostname: "devbox", user: "dee", addr: "10.0.0.5", port: 22, extra: "x" },
    { hostname: 5, user: "", port: 0 },
    "junk",
    { port: 2222 },
  ]
  const parsed = parseStore(JSON.stringify({ version: 1, ssh: [{ ...devbox, hints }] }))
  assert.deepEqual(parsed?.[0]?.hints, [{ hostname: "devbox", user: "dee", addr: "10.0.0.5", port: 22 }, { port: 2222 }])
})

test("assignId slugs the destination", () => {
  assert.equal(assignId("devbox", new Set()), "ssh-devbox")
  assert.equal(assignId("Dee@Work.Example.com", new Set()), "ssh-dee-work-example-com")
  assert.equal(assignId("dee@2001:db8::5", new Set()), "ssh-dee-2001-db8-5")
  assert.equal(assignId("_svc.", new Set()), "ssh-svc")
  assert.equal(assignId("...", new Set()), "ssh-host")
  const long = assignId(`${"a".repeat(39)}.b${"c".repeat(20)}`, new Set())
  assert.equal(long, `ssh-${"a".repeat(39)}`)
  assert.match(assignId("x".repeat(200), new Set()), /^ssh-x{40}$/)
})

test("assignId adds 4 hex of sha256(destination) on collision", () => {
  const hex = createHash("sha256").update("Devbox").digest("hex").slice(0, 4)
  assert.equal(assignId("Devbox", new Set(["ssh-devbox"])), `ssh-devbox-${hex}`)
  const second = assignId("Devbox", new Set(["ssh-devbox", `ssh-devbox-${hex}`]))
  assert.match(second, /^ssh-devbox-[0-9a-f]{4}$/)
  assert.notEqual(second, `ssh-devbox-${hex}`)
})

test("every assigned id parses back", () => {
  const taken = new Set<string>(["ssh-" + "a".repeat(40)])
  const id = assignId("a".repeat(60), taken)
  assert.deepEqual(parseStore(serializeStore([{ ...work, id, destination: "a".repeat(60) }]))?.[0]?.id, id)
})

function withDir(run: (dir: string) => void): void {
  const dir = mkdtempSync(join(tmpdir(), "servef-store-"))
  try {
    run(dir)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

test("file store: missing file loads empty; save writes 0600 atomically and loads back", () => {
  withDir((dir) => {
    const file = join(dir, "connections.json")
    const store = createFileStore(file)
    assert.deepEqual(store.load(), [])
    store.save([devbox])
    assert.equal(statSync(file).mode & 0o777, 0o600)
    assert.deepEqual(readdirSync(dir), ["connections.json"])
    assert.deepEqual(createFileStore(file).load(), [devbox])
    store.save([devbox, work])
    assert.deepEqual(store.load(), [devbox, work])
  })
})

test("file store: a malformed file is moved to .invalid once and loads empty", () => {
  withDir((dir) => {
    const file = join(dir, "connections.json")
    writeFileSync(file, "{not json")
    const errors: string[] = []
    const original = console.error
    console.error = (message: string) => errors.push(message)
    try {
      const store = createFileStore(file)
      assert.deepEqual(store.load(), [])
      assert.equal(existsSync(file), false)
      assert.equal(readFileSync(`${file}.invalid`, "utf8"), "{not json")
      assert.deepEqual(store.load(), [])
      assert.equal(errors.length, 1)
      writeFileSync(file, JSON.stringify({ version: 9, ssh: [] }))
      assert.deepEqual(store.load(), [])
      assert.equal(readFileSync(`${file}.invalid`, "utf8"), JSON.stringify({ version: 9, ssh: [] }))
    } finally {
      console.error = original
    }
  })
})
