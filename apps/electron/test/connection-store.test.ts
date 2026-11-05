import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { test } from "node:test"
import { assignId, assignServerId, createFileStore, parseStore, serializeStore, serverHost } from "../src/main/connection-store.ts"
import type { KnownConnection, KnownServer, StoredConnections } from "../src/main/connection-store.ts"

const devbox: KnownConnection = { id: "ssh-devbox", destination: "devbox", port: null, addedAt: "2026-11-01T00:00:00Z" }
const work: KnownConnection = { id: "ssh-dee-work", destination: "dee@work", port: 2222, addedAt: "2026-11-02T00:00:00Z" }
const lan: KnownServer = { id: "http-10-0-0-5-7971", host: "10.0.0.5", port: 7971, addedAt: "2026-11-03T00:00:00Z" }
const six: KnownServer = { id: "http-2001-db8-5-8080", host: "[2001:db8::5]", port: 8080, addedAt: "2026-11-03T00:00:00Z" }

function only(ssh: KnownConnection[], http: KnownServer[] = []): StoredConnections {
  return { ssh, http }
}

test("serialize and parse round trip", () => {
  const text = serializeStore(only([devbox, work], [lan, six]))
  assert.deepEqual(JSON.parse(text), { version: 1, ssh: [devbox, work], http: [lan, six] })
  assert.deepEqual(parseStore(text), only([devbox, work], [lan, six]))
})

test("parseStore: malformed JSON or unknown version is null", () => {
  assert.equal(parseStore("{"), null)
  assert.equal(parseStore("[]"), null)
  assert.equal(parseStore(JSON.stringify({ version: 2, ssh: [] })), null)
  assert.equal(parseStore(JSON.stringify({ ssh: [] })), null)
  assert.equal(parseStore(JSON.stringify({ version: 1 })), null)
  assert.deepEqual(parseStore(JSON.stringify({ version: 1, ssh: [] })), only([]))
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
    { ...work, id: "ssh-x7", addedAt: 5 },
    "string",
    null,
    { ...work, hints: [{ user: "ignored" }] },
  ]
  assert.deepEqual(parseStore(JSON.stringify({ version: 1, ssh: entries })), only([devbox, work]))
})

test("parseStore drops invalid http servers, repeated ids, and repeated addresses", () => {
  const entries = [
    lan,
    { ...lan, id: "http-other-7971" },
    { ...six, id: "ssh-devbox" },
    { ...six, id: "http-UPPER" },
    { ...six, id: "http-x1", host: "localhost" },
    { ...six, id: "http-x2", host: "127.0.0.1" },
    { ...six, id: "http-x3", host: "[::1]" },
    { ...six, id: "http-x4", host: "Box.Lan" },
    { ...six, id: "http-x5", host: "box.lan:80" },
    { ...six, id: "http-x6", host: "2001:db8::5" },
    { ...six, id: "http-x7", port: 0 },
    { ...six, id: "http-x8", addedAt: null },
    six,
  ]
  assert.deepEqual(parseStore(JSON.stringify({ version: 1, ssh: [devbox], http: entries })), only([devbox], [lan, six]))
})

test("serverHost normalizes hosts and refuses loopback and non-hosts", () => {
  assert.equal(serverHost("10.0.0.5"), "10.0.0.5")
  assert.equal(serverHost("Box.Lan"), "box.lan")
  assert.equal(serverHost("[2001:DB8::5]"), "[2001:db8::5]")
  for (const host of ["", "localhost", "127.0.0.2", "[::1]", "a b", "a/b", "u@h", "h:80", "2001:db8::5"]) assert.equal(serverHost(host), null, host)
})

test("assignServerId slugs the host and keeps the port last", () => {
  assert.equal(assignServerId("10.0.0.5", 7971, new Set()), "http-10-0-0-5-7971")
  assert.equal(assignServerId("box.lan", 80, new Set()), "http-box-lan-80")
  assert.equal(assignServerId("[2001:db8::5]", 8080, new Set()), "http-2001-db8-5-8080")
  const hex = createHash("sha256").update("box.lan").digest("hex").slice(0, 4)
  assert.equal(assignServerId("box.lan", 80, new Set(["http-box-lan-80"])), `http-box-lan-${hex}-80`)
})

test("assignServerId keeps four-octet slugs for IPv4 addresses only", () => {
  // A name that slugs like an address would otherwise share its shareable origin.
  assert.match(assignServerId("10-0-0-5", 7971, new Set()), /^http-10-0-0-5-[0-9a-f]{4}-7971$/)
  assert.equal(assignServerId("10-0-0-5x", 7971, new Set()), "http-10-0-0-5x-7971")
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
  assert.deepEqual(parseStore(serializeStore(only([{ ...work, id, destination: "a".repeat(60) }])))?.ssh[0]?.id, id)
  const server = assignServerId(`${"b".repeat(60)}.lan`, 65_535, new Set([`http-${"b".repeat(40)}-65535`]))
  assert.deepEqual(parseStore(serializeStore(only([], [{ ...lan, id: server, host: `${"b".repeat(60)}.lan`, port: 65_535 }])))?.http[0]?.id, server)
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
    assert.deepEqual(store.load(), only([]))
    store.save(only([devbox]))
    assert.equal(statSync(file).mode & 0o777, 0o600)
    assert.deepEqual(readdirSync(dir), ["connections.json"])
    assert.deepEqual(createFileStore(file).load(), only([devbox]))
    store.save(only([devbox, work], [lan]))
    assert.deepEqual(store.load(), only([devbox, work], [lan]))
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
      assert.deepEqual(store.load(), only([]))
      assert.equal(existsSync(file), false)
      assert.equal(readFileSync(`${file}.invalid`, "utf8"), "{not json")
      assert.deepEqual(store.load(), only([]))
      assert.equal(errors.length, 1)
      writeFileSync(file, JSON.stringify({ version: 9, ssh: [] }))
      assert.deepEqual(store.load(), only([]))
      assert.equal(readFileSync(`${file}.invalid`, "utf8"), JSON.stringify({ version: 9, ssh: [] }))
    } finally {
      console.error = original
    }
  })
})
