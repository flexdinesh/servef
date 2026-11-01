import assert from "node:assert/strict"
import { test } from "node:test"
import { connectPage, createAppPages, CONNECT_PATH, FORGET_PATH } from "../src/main/app-pages.ts"
import type { ConnectModel } from "../src/main/app-pages.ts"

const model: ConnectModel = {
  connections: [
    { id: "local", kind: "daemon", label: "local", state: "ready", detail: "" },
    { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "reconnecting", detail: "ssh: connect to host devbox: <timed out> & \"gone\"" },
    { id: "ssh-dee-work", kind: "ssh", label: "dee@work", state: "idle", detail: "" },
  ],
  aliases: ["build-1", "a b<c>"],
}

function pages(overrides: Partial<ConnectModel> = {}) {
  const forgotten: string[] = []
  const handle = createAppPages({
    model: async () => ({ ...model, ...overrides }),
    forget: async (id) => { forgotten.push(id) },
  })
  return { handle, forgotten }
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
  assert.equal(html.match(/<form /g)?.length, 2, "the local row has no Forget form")
})

test("connectPage links config aliases and carries the notes", async () => {
  const html = await connectPage(model).text()
  assert.ok(html.includes("<h2>SSH hosts from your config</h2>"))
  assert.ok(html.includes("<a href=\"servef://ssh/build-1/\">build-1</a>"))
  assert.ok(html.includes("<a href=\"servef://ssh/a%20b%3Cc%3E/\">a b&lt;c&gt;</a>"))
  assert.ok(html.includes("Notifications from SSH hosts arrive only while a window for that host is open."))
  assert.match(html, /first connection to a host asks you to confirm/)
  assert.match(html, /batch mode/)
  const empty = await connectPage({ connections: [], aliases: [] }).text()
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
