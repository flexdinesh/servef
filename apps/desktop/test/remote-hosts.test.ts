import assert from "node:assert/strict"
import { test } from "node:test"
import type { CliList, CliServer } from "../src/main/cli-servers.ts"
import type { ConnectionLookup } from "../src/main/connections.ts"
import type { DirectoriesOutcome, OpenOutcome, ProbeOutcome } from "../src/main/daemon.ts"
import { createRemoteHosts, directHost, forwardHost, serverRows } from "../src/main/remote-hosts.ts"
import type { RemoteHostsDeps } from "../src/main/remote-hosts.ts"

const status = (roots: string[]) => ({ protocol: 1, version: "1", pid: 1, socket: "/remote/daemon.sock", roots })
const sshReady: ConnectionLookup = { kind: "ready", connection: { id: "ssh-devbox", kind: "ssh", origin: "servef-app://ssh-devbox", socket: "/run/f.sock", status: status(["/srv/old"]), label: "devbox" } }
const portReady: ConnectionLookup = { kind: "ready", connection: { id: "ssh-devbox-7971", kind: "ssh-port", origin: "servef-app://ssh-devbox-7971", socket: "/run/p.sock", port: 7971, label: "devbox:7971" } }
const denied: ConnectionLookup = { kind: "failed", connection: "ssh", label: "devbox", failure: { kind: "forwardDenied", destination: "devbox", detail: "refused" } }

const lan: CliServer = { host: "10.0.0.5", port: 7971, origin: "http://10.0.0.5:7971" }
const everywhere: CliServer = { host: "0.0.0.0", port: 7972, origin: "http://127.0.0.1:7972" }
const loopback: CliServer = { host: "127.0.0.1", port: 7973, origin: "http://127.0.0.1:7973" }
const named: CliServer = { host: "box.lan", port: 7974, origin: "http://box.lan:7974" }

interface Harness {
  deps: RemoteHostsDeps
  calls: string[]
  lookups: Map<string, ConnectionLookup>
  list: CliList
  probe: ProbeOutcome
  directories: DirectoriesOutcome
  open: OpenOutcome
  hostname: string | null
}

function harness(): Harness {
  const h: Harness = {
    calls: [],
    lookups: new Map([["ssh-devbox", sshReady], ["ssh-devbox-7971", portReady]]),
    list: { ok: true, servers: [lan, everywhere, loopback, named] },
    probe: "ok",
    directories: { ok: true, listing: { path: "/home/dee", parent: "/home", entries: [], truncated: false } },
    open: { ok: true, result: { root: "/srv/a", added: true, replaced: [], project: "/srv/a" } },
    hostname: "devbox.example",
    deps: {
      destination: (id) => id === "ssh-devbox" ? "devbox" : null,
      lookup: async (id) => {
        h.calls.push(`lookup ${id}`)
        return h.lookups.get(id) ?? { kind: "unknown" }
      },
      listServers: async () => {
        h.calls.push("list")
        return h.list
      },
      status: async (socket) => {
        h.calls.push(`status ${socket}`)
        return status(["/srv/a", "/srv/b"])
      },
      directories: async (socket, path) => {
        h.calls.push(`directories ${socket} ${path}`)
        return h.directories
      },
      open: async (socket, path) => {
        h.calls.push(`open ${socket} ${path}`)
        return h.open
      },
      registerPort: (id, host, port) => {
        h.calls.push(`register ${id} ${host}:${port}`)
        return { ok: true, id: `${id}-${port}` }
      },
      dropPort: async (id) => { h.calls.push(`drop ${id}`) },
      probe: async (socket, port) => {
        h.calls.push(`probe ${socket} ${port}`)
        return h.probe
      },
      sshHostname: async (id) => {
        h.calls.push(`hostname ${id}`)
        return h.hostname
      },
      rememberServer: (host, port) => {
        h.calls.push(`remember ${host}:${port}`)
        return `http-${host.replaceAll(".", "-")}-${port}`
      },
      route: (url) => { h.calls.push(`route ${url}`) },
    },
  }
  return h
}

test("forwardHost takes IP literals and localhost from the origin", () => {
  assert.equal(forwardHost(lan), "10.0.0.5")
  assert.equal(forwardHost(everywhere), "127.0.0.1")
  assert.equal(forwardHost({ host: "::1", port: 1, origin: "http://[::1]:1" }), "::1")
  assert.equal(forwardHost({ host: "localhost", port: 1, origin: "http://localhost:1" }), "localhost")
  assert.equal(forwardHost(named), null)
  assert.equal(forwardHost({ host: "10.0.0.5", port: 1, origin: "http://10.0.0.5:2" }), null)
})

test("directHost: the listed host, the ssh hostname for every-address binds, none for loopback", () => {
  assert.equal(directHost(lan, "devbox.example"), "10.0.0.5")
  assert.equal(directHost(named, null), "box.lan")
  assert.equal(directHost(everywhere, "DevBox.Example"), "devbox.example")
  assert.equal(directHost(everywhere, "2001:db8::5"), "[2001:db8::5]")
  assert.equal(directHost(everywhere, null), null)
  assert.equal(directHost(everywhere, "127.0.0.1"), null)
  assert.equal(directHost(loopback, "devbox.example"), null)
  assert.equal(directHost({ host: "2001:db8::7", port: 1, origin: "http://[2001:db8::7]:1" }, null), "[2001:db8::7]")
})

test("serverRows labels servers by origin and marks loopback-only ones", () => {
  assert.deepEqual(serverRows([lan, everywhere, loopback, { host: "x", port: 1, origin: "nope" }]), [
    { origin: lan.origin, label: "10.0.0.5:7971", direct: true },
    { origin: everywhere.origin, label: "127.0.0.1:7972", direct: true },
    { origin: loopback.origin, label: "127.0.0.1:7973", direct: false },
  ])
})

test("host: a ready host shows fresh roots and every server", async () => {
  const h = harness()
  const view = await createRemoteHosts(h.deps).host("ssh-devbox")
  assert.equal(view.kind, "ready")
  assert.ok(view.kind === "ready")
  assert.deepEqual(view.roots, ["/srv/a", "/srv/b"])
  assert.ok(view.servers.ok)
  assert.equal(view.servers.servers.length, 4)
  assert.deepEqual(h.calls, ["lookup ssh-devbox", "status /run/f.sock", "list"])
})

test("host: forwardDenied lists only directly reachable servers; other failures list none", async () => {
  const h = harness()
  h.lookups.set("ssh-devbox", denied)
  const view = await createRemoteHosts(h.deps).host("ssh-devbox")
  assert.ok(view.kind === "failed")
  assert.deepEqual(view.servers?.ok ? view.servers.servers.map((row) => row.origin) : null, [lan.origin, everywhere.origin, named.origin])
  h.lookups.set("ssh-devbox", { ...denied, failure: { kind: "auth", destination: "devbox", detail: "" } })
  const auth = await createRemoteHosts(h.deps).host("ssh-devbox")
  assert.ok(auth.kind === "failed")
  assert.equal(auth.servers, null)
  assert.deepEqual(await createRemoteHosts(h.deps).host("ssh-other"), { kind: "unknown" })
})

test("browse lists over the forwarded daemon socket; a host that is not ready shows its view", async () => {
  const h = harness()
  const hosts = createRemoteHosts(h.deps)
  const view = await hosts.browse("ssh-devbox", "/home/dee", true)
  assert.equal(view.kind, "listing")
  assert.ok(h.calls.includes("directories /run/f.sock /home/dee"))
  h.directories = { ok: false, status: 404, code: "", error: "Unknown daemon endpoint.", upgrade: true }
  assert.deepEqual(await hosts.browse("ssh-devbox", "", false), { kind: "error", id: "ssh-devbox", destination: "devbox", path: "", error: "Unknown daemon endpoint.", upgrade: true })
  h.lookups.set("ssh-devbox", denied)
  assert.equal((await hosts.browse("ssh-devbox", "/", false)).kind, "host")
})

test("openFolder posts the path over the forward and opens the project window", async () => {
  const h = harness()
  const hosts = createRemoteHosts(h.deps)
  assert.deepEqual(await hosts.openFolder("ssh-devbox", "/srv/a b"), { ok: true })
  assert.deepEqual(h.calls.slice(-2), ["open /run/f.sock /srv/a b", "route servef-app://ssh-devbox/?project=%2Fsrv%2Fa"])
  assert.deepEqual(await hosts.openFolder("ssh-devbox", "relative"), { ok: false, error: "Choose an absolute folder path." })
  h.open = { ok: false, status: 400, code: "invalidRoot", error: "not a directory" }
  assert.deepEqual(await hosts.openFolder("ssh-devbox", "/etc/passwd"), { ok: false, error: "not a directory" })
})

test("openServer forwards a listed server and opens its ssh-port window", async () => {
  const h = harness()
  assert.deepEqual(await createRemoteHosts(h.deps).openServer("ssh-devbox", lan.origin), { ok: true })
  assert.deepEqual(h.calls, ["list", "register ssh-devbox 10.0.0.5:7971", "lookup ssh-devbox-7971", "probe /run/p.sock 7971", "route servef-app://ssh-devbox-7971/"])
})

test("openServer falls back to http when the forward is refused", async () => {
  const h = harness()
  h.lookups.set("ssh-devbox-7971", portReady)
  h.probe = "gone"
  assert.deepEqual(await createRemoteHosts(h.deps).openServer("ssh-devbox", lan.origin), { ok: true })
  assert.deepEqual(h.calls, [
    "list", "register ssh-devbox 10.0.0.5:7971", "lookup ssh-devbox-7971", "probe /run/p.sock 7971", "drop ssh-devbox-7971", "remember 10.0.0.5:7971", "route servef-app://http-10-0-0-5-7971/",
  ])
})

test("openServer reaches an every-address server at the ssh hostname, and a name without forwarding", async () => {
  const h = harness()
  h.lookups.set("ssh-devbox-7972", { kind: "failed", connection: "ssh", label: "devbox:7972", failure: { kind: "failed", destination: "devbox", detail: "prohibited" } })
  assert.deepEqual(await createRemoteHosts(h.deps).openServer("ssh-devbox", everywhere.origin), { ok: true })
  assert.deepEqual(h.calls.slice(-4), ["drop ssh-devbox-7972", "hostname ssh-devbox", "remember devbox.example:7972", "route servef-app://http-devbox-example-7972/"])
  const n = harness()
  assert.deepEqual(await createRemoteHosts(n.deps).openServer("ssh-devbox", named.origin), { ok: true })
  assert.deepEqual(n.calls, ["list", "remember box.lan:7974", "route servef-app://http-box-lan-7974/"])
})

test("openServer: a loopback-only server without the forward has no fallback; unlisted servers never open", async () => {
  const h = harness()
  h.lookups.set("ssh-devbox-7973", { kind: "failed", connection: "ssh", label: "devbox:7973", failure: { kind: "failed", destination: "devbox", detail: "prohibited" } })
  const outcome = await createRemoteHosts(h.deps).openServer("ssh-devbox", loopback.origin)
  assert.ok(!outcome.ok)
  assert.match(outcome.error, /^prohibited The server listens only on devbox's loopback/)
  assert.ok(!h.calls.some((call) => call.startsWith("route") || call.startsWith("remember")))
  const unlisted = await createRemoteHosts(h.deps).openServer("ssh-devbox", "http://10.9.9.9:1")
  assert.deepEqual(unlisted, { ok: false, error: "No servef server at http://10.9.9.9:1 is running on devbox." })
  h.list = { ok: false, detail: "timed out" }
  assert.deepEqual(await createRemoteHosts(h.deps).openServer("ssh-devbox", lan.origin), { ok: false, error: "Could not list servef servers on devbox: timed out" })
})
