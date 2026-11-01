import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { test } from "node:test"
import type { KnownConnection } from "../src/main/connection-store.ts"
import { parseSshLink, resolveSshLink, sshTargetURL } from "../src/main/ssh-links.ts"
import type { ResolvedAlias, SshHints, SshLink } from "../src/main/ssh-links.ts"
import { isRecord } from "../../web/src/api/http.ts"

interface LinkCase {
  name: string
  route: string
  query: Record<string, string>
  url: string
  parsed: { host: string } & SshHints
}

function isLinkCase(value: unknown): value is LinkCase {
  return isRecord(value) && typeof value.name === "string" && typeof value.route === "string" && typeof value.url === "string"
    && isRecord(value.query) && isRecord(value.parsed) && typeof value.parsed.host === "string"
}

const fixture: unknown = JSON.parse(readFileSync(new URL("../../testdata/remote-links.json", import.meta.url), "utf8"))
assert.ok(Array.isArray(fixture))
const cases: LinkCase[] = fixture.filter(isLinkCase)
assert.equal(cases.length, fixture.length)

function parse(url: string): SshLink | null {
  return parseSshLink(new URL(url))
}

for (const testCase of cases) {
  test(`parseSshLink fixture: ${testCase.name}`, () => {
    const link = parse(testCase.url)
    assert.ok(link !== null)
    const { host, ...hints } = testCase.parsed
    assert.equal(link.host, host)
    assert.deepEqual(link.hints, hints)
    assert.equal(link.route, testCase.route)
    assert.equal(link.hash, "")
    assert.deepEqual(Object.fromEntries(new URLSearchParams(link.query)), testCase.query)
    assert.equal(link.query.includes("ssh-"), false)
  })
}

test("parseSshLink keeps the remaining query encoding and order, and the hash", () => {
  const link = parse("servef://ssh/devbox/view?project=%2Fwork%2Fmy+repo&ssh-user=dee&wt=tour&b=%20#f=a.ts")
  assert.ok(link !== null)
  assert.equal(link.query, "project=%2Fwork%2Fmy+repo&wt=tour&b=%20")
  assert.equal(link.hash, "#f=a.ts")
  assert.equal(link.route, "/view")
})

test("parseSshLink path forms", () => {
  assert.equal(parse("servef://ssh/devbox/")?.route, "/")
  assert.equal(parse("servef://ssh/devbox")?.route, "/")
  assert.equal(parse("servef://ssh/devbox/view")?.route, "/view")
  assert.equal(parse("servef://ssh/devbox")?.query, "")
})

test("parseSshLink rejects other forms", () => {
  for (const bad of [
    "servef://local/?project=%2Fsrv",
    "servef-app://ssh/devbox/",
    "https://ssh/devbox/",
    "servef://ssh:22/devbox/",
    "servef://user@ssh/devbox/",
    "servef://user:pw@ssh/devbox/",
    "servef://ssh/",
    "servef://ssh",
    "servef://ssh/devbox/other",
    "servef://ssh/devbox/view/",
    "servef://ssh/devbox/view/extra",
    "servef://ssh/%E0%A4%A/",
    "servef://ssh/dev%2Fbox/",
    "servef://ssh/dev%0Abox/",
    "servef://ssh/devbox/?ssh-port=0",
    "servef://ssh/devbox/?ssh-port=65536",
    "servef://ssh/devbox/?ssh-port=22.5",
    "servef://ssh/devbox/?ssh-port=abc",
  ]) {
    assert.equal(parse(bad), null, bad)
  }
})

test("parseSshLink drops unknown and empty ssh-* pairs", () => {
  const link = parse("servef://ssh/devbox/?ssh-other=1&ssh-user=&project=%2Fsrv")
  assert.ok(link !== null)
  assert.deepEqual(link.hints, {})
  assert.equal(link.query, "project=%2Fsrv")
})

test("sshTargetURL", () => {
  const link = parse("servef://ssh/devbox/view?project=%2Fsrv&ssh-user=dee&wt=tour#f=x")
  assert.ok(link !== null)
  assert.equal(sshTargetURL("ssh-devbox", link), "servef-app://ssh-devbox/view?project=%2Fsrv&wt=tour#f=x")
  const bare = parse("servef://ssh/devbox/?ssh-user=dee")
  assert.ok(bare !== null)
  assert.equal(sshTargetURL("ssh-devbox", bare), "servef-app://ssh-devbox/")
})

function link(host: string, hints: SshHints = {}): SshLink {
  return { host, route: "/", query: "", hash: "", hints }
}

function known(id: string, destination: string, hints: SshHints[] = [], port: number | null = null): KnownConnection {
  return { id, destination, port, hints, addedAt: "2026-11-01T00:00:00Z" }
}

const knownHosts = [
  known("ssh-devbox", "devbox", [{ hostname: "devbox", user: "dee", addr: "10.0.0.5" }]),
  known("ssh-dee-work-example-com", "dee@work.example.com", [{ hostname: "ip-10-0-0-9", user: "dee", addr: "10.0.0.9", port: 2222 }]),
]

test("step 1: a known destination, id slug, or remembered hostname resolves without asking", () => {
  assert.deepEqual(resolveSshLink(link("DevBox"), knownHosts, []), { kind: "known", id: "ssh-devbox" })
  assert.deepEqual(resolveSshLink(link("dee@work.example.com"), knownHosts, []), { kind: "known", id: "ssh-dee-work-example-com" })
  assert.deepEqual(resolveSshLink(link("dee-work-example-com"), knownHosts, []), { kind: "known", id: "ssh-dee-work-example-com" })
  assert.deepEqual(resolveSshLink(link("IP-10-0-0-9"), knownHosts, []), { kind: "known", id: "ssh-dee-work-example-com" })
})

test("step 2: remembered hints (hostname+user, or addr+port+user) resolve without asking", () => {
  assert.deepEqual(resolveSshLink(link("other-name", { hostname: "devbox", user: "dee" }), knownHosts, []), { kind: "known", id: "ssh-devbox" })
  assert.deepEqual(resolveSshLink(link("other-name", { hostname: "x", user: "dee", addr: "10.0.0.9", port: 2222 }), knownHosts, []), { kind: "known", id: "ssh-dee-work-example-com" })
  // An absent link port with an address means 22, which differs from the remembered 2222.
  assert.equal(resolveSshLink(link("other-name", { hostname: "x", user: "dee", addr: "10.0.0.9" }), knownHosts, []).kind, "ask")
  assert.deepEqual(resolveSshLink(link("other-name", { hostname: "x", user: "dee", addr: "10.0.0.5" }), knownHosts, []), { kind: "known", id: "ssh-devbox" })
  // A different user never matches.
  assert.equal(resolveSshLink(link("other-name", { hostname: "devbox", user: "root" }), knownHosts, []).kind, "ask")
})

const aliases: ResolvedAlias[] = [
  { alias: "devbox", hostname: "10.0.0.5", user: "dee", port: 22 },
  { alias: "devbox-root", hostname: "10.0.0.5", user: "root", port: 22 },
  { alias: "devbox-alt", hostname: "devbox.lan", user: "dee", port: 2222 },
  { alias: "box-by-name", hostname: "DEVBOX", user: "dee", port: 22 },
  { alias: "unrelated", hostname: "elsewhere", user: "dee", port: 22 },
]

test("step 3: candidates by alias, then -G hostname equal to host, hostname hint, or addr hint", () => {
  const resolved = resolveSshLink(link("devbox", { hostname: "devbox", user: "dee", addr: "10.0.0.5" }), [], aliases)
  assert.deepEqual(resolved, {
    kind: "ask",
    candidates: [
      { id: "", destination: "devbox", port: null },
      { id: "", destination: "box-by-name", port: null },
    ],
    fallbacks: [
      { id: "", destination: "dee@10.0.0.5", port: null },
      { id: "", destination: "dee@devbox", port: null },
    ],
  })
})

test("step 3: user and port exclusion", () => {
  const byPort = resolveSshLink(link("work", { hostname: "devbox.lan", user: "dee", addr: "10.1.1.1", port: 2222 }), [], aliases)
  assert.ok(byPort.kind === "ask")
  assert.deepEqual(byPort.candidates.map((target) => target.destination), ["devbox-alt"])
  const root = resolveSshLink(link("x", { user: "root", addr: "10.0.0.5" }), [], aliases)
  assert.ok(root.kind === "ask")
  assert.deepEqual(root.candidates.map((target) => target.destination), ["devbox-root"])
  // Without any connection hints, user and port do not exclude.
  const bare = resolveSshLink(link("devbox.lan"), [], aliases)
  assert.ok(bare.kind === "ask")
  assert.deepEqual(bare.candidates.map((target) => target.destination), ["devbox-alt"])
})

test("step 3: an alias equal to the host is a candidate even when -G user differs", () => {
  const resolved = resolveSshLink(link("devbox-root", { user: "dee" }), [], aliases)
  assert.ok(resolved.kind === "ask")
  assert.deepEqual(resolved.candidates.map((target) => target.destination), ["devbox-root"])
})

test("step 4: fallbacks carry the port, need a user, and must be valid destinations", () => {
  const ipv6 = resolveSshLink(link("devbox6", { hostname: "devbox6", user: "dee", addr: "2001:db8::5", port: 2200 }), [], [])
  assert.deepEqual(ipv6, {
    kind: "ask",
    candidates: [],
    fallbacks: [
      { id: "", destination: "dee@2001:db8::5", port: 2200 },
      { id: "", destination: "dee@devbox6", port: 2200 },
    ],
  })
  const spaced = resolveSshLink(link("dev box é", { hostname: "dev box é", user: "dee" }), [], [])
  assert.deepEqual(spaced, { kind: "ask", candidates: [], fallbacks: [] })
  const noUser = resolveSshLink(link("devbox", { hostname: "devbox", addr: "10.0.0.5" }), [], [])
  assert.deepEqual(noUser, { kind: "ask", candidates: [], fallbacks: [] })
  const same = resolveSshLink(link("h", { hostname: "10.0.0.5", user: "dee", addr: "10.0.0.5" }), [], [])
  assert.ok(same.kind === "ask")
  assert.deepEqual(same.fallbacks.map((target) => target.destination), ["dee@10.0.0.5"])
})

test("candidates are deduplicated and capped at 4", () => {
  const many: ResolvedAlias[] = Array.from({ length: 8 }, (_, index) => ({ alias: `box-${index}`, hostname: "devbox", user: "dee", port: 22 }))
  const resolved = resolveSshLink(link("devbox"), [], [...many, ...many])
  assert.ok(resolved.kind === "ask")
  assert.deepEqual(resolved.candidates.map((target) => target.destination), ["box-0", "box-1", "box-2", "box-3"])
})
