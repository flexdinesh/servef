import assert from "node:assert/strict"
import { test } from "node:test"
import { connectPage, createAppPages, CONNECT_PATH, FORGET_PATH, OPEN_PATH } from "../src/main/app-pages.ts"
import type { ConnectModel, OpenOutcome } from "../src/main/app-pages.ts"

const model: ConnectModel = {
  connections: [
    { id: "local", kind: "daemon", label: "local", state: "ready", detail: "" },
    { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "reconnecting", detail: "ssh: connect to host devbox: <timed out> & \"gone\"" },
    { id: "ssh-dee-work", kind: "ssh", label: "dee@work", state: "idle", detail: "" },
    { id: "cli-7971", kind: "cli", label: "localhost:7971", state: "ready", detail: "" },
  ],
  aliases: ["build-1", "a b<c>"],
  serversDetail: "",
}

function pages(overrides: Partial<ConnectModel> = {}, open: (raw: string) => OpenOutcome = () => ({ ok: true })) {
  const forgotten: string[] = []
  const opened: string[] = []
  const handle = createAppPages({
    model: async () => ({ ...model, ...overrides }),
    forget: async (id) => { forgotten.push(id) },
    open: async (raw) => {
      opened.push(raw)
      return open(raw)
    },
  })
  return { handle, forgotten, opened }
}

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
  assert.ok(html.includes("<a href=\"servef://ssh/devbox/\">devbox</a><span class=\"state\">Reconnecting</span>"))
  assert.ok(html.includes("<a href=\"servef://ssh/dee-work/\">dee@work</a><span class=\"state\">Not connected</span>"))
  assert.ok(html.includes("ssh: connect to host devbox: &lt;timed out&gt; &amp; &quot;gone&quot;"))
  assert.ok(html.includes(`<form method="post" action="${FORGET_PATH}"><input type="hidden" name="id" value="ssh-devbox"><button>Forget</button></form>`))
  assert.ok(html.includes("value=\"ssh-dee-work\""))
  assert.equal(html.match(/<form method="post" action="\/forget">/g)?.length, 2, "the local row has no Forget form")
})

test("connectPage links config aliases and carries the notes", async () => {
  const html = await connectPage(model).text()
  assert.ok(html.includes("<h2>SSH hosts from your config</h2>"))
  assert.ok(html.includes("<a href=\"servef://ssh/build-1/\">build-1</a>"))
  assert.ok(html.includes("<a href=\"servef://ssh/a%20b%3Cc%3E/\">a b&lt;c&gt;</a>"))
  assert.ok(html.includes("Notifications from SSH hosts arrive only while a window for that host is open."))
  assert.match(html, /first connection to a host asks you to confirm/)
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

test("POST /forget forgets ssh ids only and redirects to Connect", async () => {
  const { handle, forgotten } = pages()
  const response = await handle(form("id=ssh-devbox"))
  assert.equal(response.status, 303)
  assert.equal(response.headers.get("location"), CONNECT_PATH)
  for (const body of ["id=local", "id=app", "id=", "other=ssh-x", ""]) {
    assert.equal((await handle(form(body))).status, 303, body)
  }
  assert.equal((await handle(form("id=ssh-work", "application/x-www-form-urlencoded; charset=UTF-8"))).status, 303)
  assert.deepEqual(forgotten, ["ssh-devbox", "ssh-work"])
})

test("forget requires a form body; other methods are 405 and unknown paths 404", async () => {
  const { handle, forgotten } = pages()
  assert.equal((await handle(form("{\"id\":\"ssh-devbox\"}", "application/json"))).status, 415)
  assert.equal((await handle(form(`id=ssh-${"x".repeat(5000)}`))).status, 413)
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
