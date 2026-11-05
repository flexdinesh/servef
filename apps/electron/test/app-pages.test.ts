import assert from "node:assert/strict"
import { test } from "node:test"
import { browsePage, connectPage, createAppPages, CONNECT_PATH, FORGET_PATH, hostPage, OPEN_PATH, SSH_PATH } from "../src/main/app-pages.ts"
import type { ConnectModel, OpenOutcome } from "../src/main/app-pages.ts"
import { sshFailureInstructions } from "../src/main/instructions.ts"
import type { ActionOutcome, BrowseView, HostView, RemoteHosts } from "../src/main/remote-hosts.ts"
import type { SshFailure } from "../src/main/ssh-errors.ts"

const instructions = (failure: SshFailure) => sshFailureInstructions(failure, null)

const model: ConnectModel = {
  connections: [
    { id: "local", kind: "daemon", label: "local", state: "ready", detail: "" },
    { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "reconnecting", detail: "ssh: connect to host devbox: <timed out> & \"gone\"" },
    { id: "ssh-dee-work", kind: "ssh", label: "dee@work", state: "idle", detail: "" },
    { id: "http-10-0-0-5-7971", kind: "http", label: "10.0.0.5:7971", state: "ready", detail: "" },
    { id: "cli-7971", kind: "cli", label: "localhost:7971", state: "ready", detail: "" },
  ],
  aliases: ["build-1", "a b<c>"],
  serversDetail: "",
}

const ready: HostView = {
  kind: "ready",
  id: "ssh-devbox",
  destination: "devbox",
  roots: ["/srv/a b", "/srv/<x>"],
  servers: { ok: true, servers: [{ origin: "http://10.0.0.5:7971", label: "10.0.0.5:7971", direct: true }, { origin: "http://127.0.0.1:7972", label: "127.0.0.1:7972", direct: false }] },
}

interface FakeHosts extends RemoteHosts {
  calls: string[]
  view: HostView
  listing: BrowseView
  outcome: ActionOutcome
}

function fakeHosts(): FakeHosts {
  const hosts: FakeHosts = {
    calls: [],
    view: ready,
    listing: { kind: "listing", id: "ssh-devbox", destination: "devbox", hidden: false, listing: { path: "/home/dee", parent: "/home", entries: [], truncated: false } },
    outcome: { ok: true },
    async host(id) {
      hosts.calls.push(`host ${id}`)
      return hosts.view
    },
    async browse(id, path, hidden) {
      hosts.calls.push(`browse ${id} ${path} ${hidden}`)
      return hosts.listing
    },
    async openFolder(id, path) {
      hosts.calls.push(`open ${id} ${path}`)
      return hosts.outcome
    },
    async openServer(id, origin) {
      hosts.calls.push(`server ${id} ${origin}`)
      return hosts.outcome
    },
  }
  return hosts
}

function pages(overrides: Partial<ConnectModel> = {}, open: (raw: string) => OpenOutcome = () => ({ ok: true })) {
  const forgotten: string[] = []
  const opened: string[] = []
  const added: string[] = []
  const stopped: string[] = []
  const hosts = fakeHosts()
  const handle = createAppPages({
    model: async () => ({ ...model, ...overrides }),
    forget: async (id) => { forgotten.push(id) },
    open: async (raw) => {
      opened.push(raw)
      return open(raw)
    },
    addSsh: (alias) => {
      added.push(alias)
      return alias === "devbox" ? "ssh-devbox" : null
    },
    hosts,
    stopAndRetry: async (id) => { stopped.push(id) },
    instructions,
  })
  return { handle, forgotten, opened, added, stopped, hosts }
}

const post = (path: string, body: string) =>
  new Request(`servef-app://app${path}`, { method: "POST", body, headers: { "content-type": "application/x-www-form-urlencoded" } })

const form = (body: string, type = "application/x-www-form-urlencoded") =>
  new Request(`servef-app://app${FORGET_PATH}`, { method: "POST", body, headers: { "content-type": type } })

test("connectPage lists connections with state, detail, links, and Forget forms", async () => {
  const response = connectPage(model)
  assert.equal(response.status, 200)
  assert.equal(response.headers.get("content-type"), "text/html; charset=utf-8")
  assert.equal(response.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
  assert.equal(response.headers.get("cache-control"), "no-store")
  const html = await response.text()
  assert.ok(html.includes("<title>servef - Connect</title>"))
  assert.ok(!html.includes("<script"))
  assert.ok(html.includes("<h2>Connections</h2>"))
  assert.ok(html.includes("<a href=\"servef://local/\">local</a><span class=\"state\">Connected</span>"))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-devbox">devbox</a><span class="state">Reconnecting</span>`))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-dee-work">dee@work</a><span class="state">Not connected</span>`))
  assert.ok(html.includes("<a href=\"servef-app://http-10-0-0-5-7971/\">10.0.0.5:7971</a><span class=\"tag\">http</span>"))
  assert.ok(html.includes("ssh: connect to host devbox: &lt;timed out&gt; &amp; &quot;gone&quot;"))
  assert.ok(html.includes(`<form method="post" action="${FORGET_PATH}"><input type="hidden" name="id" value="ssh-devbox"><button>Forget</button></form>`))
  assert.ok(html.includes("value=\"ssh-dee-work\""))
  assert.ok(html.includes("value=\"http-10-0-0-5-7971\""))
  assert.equal(html.match(/<form method="post" action="\/forget">/g)?.length, 3, "the local row has no Forget form")
  assert.ok(!html.includes("servef://ssh"))
})

test("connectPage offers config aliases as POST forms and carries the notes", async () => {
  const html = await connectPage(model).text()
  assert.ok(html.includes("<h2>SSH hosts from your config</h2>"))
  assert.ok(html.includes(`<form class="host" method="post" action="${SSH_PATH}"><input type="hidden" name="alias" value="build-1"><button class="link">build-1</button></form>`))
  assert.ok(html.includes("value=\"a b&lt;c&gt;\"><button class=\"link\">a b&lt;c&gt;</button>"))
  assert.ok(html.includes("Notifications from SSH hosts arrive only while a window for that host is open."))
  assert.doesNotMatch(html, /confirm/)
  assert.match(html, /batch mode/)
  const empty = await connectPage({ connections: [], aliases: [], serversDetail: "" }).text()
  assert.match(empty, /No Host aliases found/)
})

test("GET and HEAD /connect render the page; / redirects to it", async () => {
  const { handle } = pages()
  const get = await handle(new Request(`servef-app://app${CONNECT_PATH}`))
  assert.equal(get.status, 200)
  assert.match(await get.text(), /devbox/)
  const head = await handle(new Request(`servef-app://app${CONNECT_PATH}`, { method: "HEAD" }))
  assert.equal(head.status, 200)
  assert.equal(head.body, null)
  assert.match(head.headers.get("content-security-policy") ?? "", /form-action 'self'/)
  const root = await handle(new Request("servef-app://app/"))
  assert.equal(root.status, 303)
  assert.equal(root.headers.get("location"), CONNECT_PATH)
})

test("POST /forget forgets ssh and http ids only and redirects to Connect", async () => {
  const { handle, forgotten } = pages()
  const response = await handle(form("id=ssh-devbox"))
  assert.equal(response.status, 303)
  assert.equal(response.headers.get("location"), CONNECT_PATH)
  for (const body of ["id=local", "id=app", "id=", "other=ssh-x", ""]) {
    assert.equal((await handle(form(body))).status, 303, body)
  }
  assert.equal((await handle(form("id=ssh-work", "application/x-www-form-urlencoded; charset=UTF-8"))).status, 303)
  assert.equal((await handle(form("id=http-10-0-0-5-7971"))).status, 303)
  assert.deepEqual(forgotten, ["ssh-devbox", "ssh-work", "http-10-0-0-5-7971"])
})

test("forget requires a form body; other methods are 405 and unknown paths 404", async () => {
  const { handle, forgotten } = pages()
  assert.equal((await handle(form("{\"id\":\"ssh-devbox\"}", "application/json"))).status, 415)
  assert.equal((await handle(form(`id=ssh-${"x".repeat(9000)}`))).status, 413)
  const get = await handle(new Request(`servef-app://app${FORGET_PATH}?id=ssh-devbox`))
  assert.equal(get.status, 405)
  assert.equal(get.headers.get("allow"), "POST")
  const post = await handle(new Request(`servef-app://app${CONNECT_PATH}`, { method: "POST", body: "id=ssh-devbox" }))
  assert.equal(post.status, 405)
  assert.equal(post.headers.get("allow"), "GET, HEAD")
  assert.equal((await handle(new Request("servef-app://app/", { method: "DELETE" }))).status, 405)
  assert.equal((await handle(new Request("servef-app://app/api/projects"))).status, 404)
  assert.equal((await handle(new Request("servef-app://app/connect/"))).status, 404)
  assert.deepEqual(forgotten, [])
})

// Servers on this computer and pasted links (decision 0071).

test("connectPage lists running CLI servers apart from connections", async () => {
  const html = await connectPage(model).text()
  assert.ok(html.includes("<h2>Servers on this computer</h2>"))
  assert.ok(html.includes("<li><a href=\"servef-app://cli-7971/\">localhost:7971</a></li>"))
  assert.ok(!html.includes("servef://local/\">localhost:7971"), "CLI servers are not connection rows")
  assert.equal(html.match(/value="cli-7971"/g), null, "CLI servers have no Forget form")
  const none = await connectPage({ ...model, connections: [] }).text()
  assert.match(none, /No servef servers are running\./)
  const failed = await connectPage({ ...model, connections: [], serversDetail: "Cannot run servef: <ENOENT>" }).text()
  assert.match(failed, /Could not list servef servers: Cannot run servef: &lt;ENOENT&gt;/)
})

test("connectPage has an Open a link GET form and refills it with an escaped error", async () => {
  const html = await connectPage(model).text()
  assert.ok(html.includes(`<form class="open" method="get" action="${OPEN_PATH}"><input type="text" name="url"`))
  assert.ok(html.includes("value=\"\" required>"))
  assert.ok(!html.includes("role=\"alert\""))
  const refilled = await connectPage(model, { value: "http://evil.example/\"><script>", error: "Not a servef link." }).text()
  assert.ok(refilled.includes("value=\"http://evil.example/&quot;&gt;&lt;script&gt;\""))
  assert.ok(refilled.includes("<p class=\"error\" role=\"alert\">Not a servef link.</p>"))
  assert.ok(!refilled.includes("<script"))
})

test("GET /open opens the pasted link and redirects to Connect", async () => {
  const { handle, opened } = pages()
  const response = await handle(new Request(`servef-app://app${OPEN_PATH}?url=${encodeURIComponent("http://localhost:7971/?project=/src")}`))
  assert.equal(response.status, 303)
  assert.equal(response.headers.get("location"), CONNECT_PATH)
  assert.deepEqual(opened, ["http://localhost:7971/?project=/src"])
})

test("GET /open shows the Connect page with the reason when the link does not open", async () => {
  const { handle } = pages({}, () => ({ ok: false, reason: "No servef server is running on port 8000." }))
  const response = await handle(new Request(`servef-app://app${OPEN_PATH}?url=${encodeURIComponent("http://localhost:8000/")}`))
  assert.equal(response.status, 200)
  const html = await response.text()
  assert.match(html, /No servef server is running on port 8000\./)
  assert.ok(html.includes("value=\"http://localhost:8000/\""))
})

test("/open refuses POST and opens nothing on HEAD", async () => {
  const { handle, opened } = pages()
  assert.equal((await handle(new Request(`servef-app://app${OPEN_PATH}?url=x`, { method: "POST" }))).status, 405)
  assert.equal((await handle(new Request(`servef-app://app${OPEN_PATH}?url=x`, { method: "HEAD" }))).status, 200)
  assert.deepEqual(opened, [])
})

// ssh hosts from the Connect page (decision 0076).

test("POST /ssh stores a config alias and opens its host page; others go back to Connect", async () => {
  const { handle, added } = pages()
  const opened = await handle(post(SSH_PATH, "alias=devbox"))
  assert.equal(opened.status, 303)
  assert.equal(opened.headers.get("location"), `${SSH_PATH}/ssh-devbox`)
  const unknown = await handle(post(SSH_PATH, "alias=nope"))
  assert.equal(unknown.headers.get("location"), CONNECT_PATH)
  assert.deepEqual(added, ["devbox", "nope"])
  const get = await handle(new Request(`servef-app://app${SSH_PATH}?alias=devbox`))
  assert.equal(get.status, 405)
  assert.deepEqual(added, ["devbox", "nope"])
})

test("hostPage lists folders, servers, and Browse folders, all escaped", async () => {
  const response = hostPage(ready, instructions)
  assert.equal(response.status, 200)
  assert.match(response.headers.get("content-security-policy") ?? "", /form-action 'self'/)
  const html = await response.text()
  assert.ok(html.includes("<title>servef - devbox</title>"))
  assert.ok(!html.includes("<script"))
  assert.ok(html.includes("<a href=\"servef-app://ssh-devbox/?project=%2Fsrv%2Fa%20b\">/srv/a b</a>"))
  assert.ok(html.includes("<a href=\"servef-app://ssh-devbox/?project=%2Fsrv%2F%3Cx%3E\">/srv/&lt;x&gt;</a>"))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-devbox/browse?path=">Browse folders</a>`))
  assert.ok(html.includes(`action="${SSH_PATH}/ssh-devbox/server"><input type="hidden" name="origin" value="http://10.0.0.5:7971"><button class="link">10.0.0.5:7971</button>`))
  assert.ok(html.includes("127.0.0.1:7972</button></form><span class=\"tag\">loopback only</span>"))
  const empty = await hostPage({ ...ready, roots: [], servers: { ok: false, detail: "<boom>" } }, instructions).text()
  assert.match(empty, /No folders yet\./)
  assert.match(empty, /Could not list servef servers: &lt;boom&gt;/)
})

test("hostPage shows failures with instructions, and after forwardDenied the directly reachable servers", async () => {
  const denied: HostView = {
    kind: "failed",
    id: "ssh-devbox",
    destination: "devbox",
    failure: { kind: "forwardDenied", destination: "devbox", detail: "refused <forward>" },
    servers: { ok: true, servers: [{ origin: "http://10.0.0.5:7971", label: "10.0.0.5:7971", direct: true }] },
  }
  const response = hostPage(denied, instructions)
  assert.equal(response.status, 503)
  const html = await response.text()
  assert.match(html, /devbox refused the socket forward/)
  assert.match(html, /refused &lt;forward&gt;/)
  assert.match(html, /reachable without the forward/)
  assert.ok(html.includes("value=\"http://10.0.0.5:7971\""))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-devbox">Retry</a>`))

  const mismatch = await hostPage({ ...denied, failure: { kind: "mismatch", destination: "devbox", detail: "" }, servers: null }, instructions).text()
  assert.ok(mismatch.includes(`action="${SSH_PATH}/ssh-devbox/stop-and-retry"`))
  assert.doesNotMatch(mismatch, /reachable without the forward/)

  const connecting = hostPage({ kind: "connecting", id: "ssh-devbox", destination: "devbox", detail: "timed out" }, instructions)
  assert.equal(connecting.status, 503)
  assert.match(await connecting.text(), new RegExp(`content="3; url=${SSH_PATH}/ssh-devbox"`))
  assert.equal(hostPage({ kind: "unknown" }, instructions).status, 404)
})

test("browsePage links folders, the parent, hidden folders, and an Open this folder form", async () => {
  const view: BrowseView = {
    kind: "listing",
    id: "ssh-devbox",
    destination: "devbox",
    hidden: false,
    listing: {
      path: "/home/dee",
      parent: "/home",
      truncated: true,
      entries: [
        { name: "a&b", path: "/home/dee/a&b", repository: true, hidden: false },
        { name: ".config", path: "/home/dee/.config", repository: false, hidden: true },
      ],
    },
  }
  const html = await browsePage(view, instructions).text()
  assert.ok(html.includes("<title>servef - Browse devbox</title>"))
  assert.ok(!html.includes("<script"))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-devbox/browse?path=%2Fhome">..</a>`))
  assert.ok(html.includes(`<a href="${SSH_PATH}/ssh-devbox/browse?path=%2Fhome%2Fdee%2Fa%26b">a&amp;b</a><span class="tag">repository</span>`))
  assert.ok(!html.includes(".config"))
  assert.ok(html.includes(`action="${SSH_PATH}/ssh-devbox/open"><input type="hidden" name="path" value="/home/dee"><button>Open this folder</button>`))
  assert.ok(html.includes("browse?path=%2Fhome%2Fdee&amp;hidden=1\">Show hidden folders"))
  assert.match(html, /Only the first folders are listed/)

  const shown = await browsePage({ ...view, hidden: true, listing: { ...view.listing, parent: "", truncated: false } }, instructions).text()
  assert.ok(shown.includes(".config"))
  assert.ok(shown.includes("hidden=1\">.config</a>"))
  assert.ok(!shown.includes(">..</a>"), "the filesystem root has no parent link")
  assert.match(shown, /Hide hidden folders/)
})

test("browsePage explains errors and an old servef on the host", async () => {
  const old = browsePage({ kind: "error", id: "ssh-devbox", destination: "devbox", path: "", error: "Unknown daemon endpoint.", upgrade: true }, instructions)
  assert.equal(old.status, 501)
  assert.match(await old.text(), /servef on devbox cannot list folders\. Upgrade servef there/)
  const missing = await browsePage({ kind: "error", id: "ssh-devbox", destination: "devbox", path: "/gone<", error: "path not found", upgrade: false }, instructions).text()
  assert.match(missing, /path not found/)
  assert.ok(missing.includes("<code>/gone&lt;</code>"))
})

test("host routes: GET pages, POST actions, and their outcomes", async () => {
  const { handle, hosts, stopped } = pages()
  assert.equal((await handle(new Request(`servef-app://app${SSH_PATH}/ssh-devbox`))).status, 200)
  assert.equal((await handle(new Request(`servef-app://app${SSH_PATH}/ssh-devbox/browse?path=%2Fsrv&hidden=1`))).status, 200)
  const opened = await handle(post(`${SSH_PATH}/ssh-devbox/open`, "path=%2Fsrv%2Fa"))
  assert.equal(opened.status, 303)
  assert.equal(opened.headers.get("location"), `${SSH_PATH}/ssh-devbox`)
  const server = await handle(post(`${SSH_PATH}/ssh-devbox/server`, `origin=${encodeURIComponent("http://10.0.0.5:7971")}`))
  assert.equal(server.status, 303)
  const stop = await handle(post(`${SSH_PATH}/ssh-devbox/stop-and-retry`, ""))
  assert.equal(stop.headers.get("location"), `${SSH_PATH}/ssh-devbox`)
  assert.deepEqual(stopped, ["ssh-devbox"])
  assert.deepEqual(hosts.calls, ["host ssh-devbox", "browse ssh-devbox /srv true", "open ssh-devbox /srv/a", "server ssh-devbox http://10.0.0.5:7971"])

  hosts.outcome = { ok: false, error: "No servef server at <x> is running." }
  const failedServer = await handle(post(`${SSH_PATH}/ssh-devbox/server`, "origin=x"))
  assert.equal(failedServer.status, 200)
  assert.match(await failedServer.text(), /role="alert">No servef server at &lt;x&gt; is running\./)
  const failedOpen = await handle(post(`${SSH_PATH}/ssh-devbox/open`, "path=%2Fnope"))
  assert.match(await failedOpen.text(), /role="alert">No servef server/)
  assert.equal(hosts.calls.at(-1), "browse ssh-devbox /nope false")
})

test("host routes refuse wrong methods, bodies, and malformed ids", async () => {
  const { handle, hosts } = pages()
  assert.equal((await handle(new Request(`servef-app://app${SSH_PATH}/ssh-devbox/open?path=%2F`))).status, 405)
  assert.equal((await handle(new Request(`servef-app://app${SSH_PATH}/ssh-devbox/server`))).status, 405)
  assert.equal((await handle(post(`${SSH_PATH}/ssh-devbox`, ""))).status, 405)
  assert.equal((await handle(post(`${SSH_PATH}/ssh-devbox/browse`, ""))).status, 405)
  const json = new Request(`servef-app://app${SSH_PATH}/ssh-devbox/open`, { method: "POST", body: "{}", headers: { "content-type": "application/json" } })
  assert.equal((await handle(json)).status, 415)
  for (const path of [`${SSH_PATH}/ssh-Dev`, `${SSH_PATH}/http-x-1`, `${SSH_PATH}/ssh-devbox/other`, `${SSH_PATH}/ssh-devbox/browse/x`]) {
    assert.equal((await handle(new Request(`servef-app://app${path}`))).status, 404, path)
  }
  assert.deepEqual(hosts.calls, [])
})
