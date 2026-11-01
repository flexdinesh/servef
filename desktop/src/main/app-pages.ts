// App pages on servef-app://app (decision 0070): the Connect page and its
// POST actions. Static HTML, no scripts.
import type { ConnectionState, ConnectionSummary } from "./connections.ts"
import { escapeHTML, jsonError, PAGE_STYLE } from "./pages.ts"

export const CONNECT_PATH = "/connect"
export const FORGET_PATH = "/forget"

export interface ConnectModel {
  // connections: local first, then known ssh hosts (Connections.summaries()).
  connections: ConnectionSummary[]
  // aliases are Host aliases from the ssh config not already known.
  aliases: string[]
}

const CONNECT_HEADERS = {
  "content-type": "text/html; charset=utf-8",
  "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'",
  "cache-control": "no-store",
}

// FORM_LIMIT bounds a POST body (one ssh id).
const FORM_LIMIT = 4096

const CONNECT_STYLE = `
main { margin-top: 10vh; padding-bottom: 3rem; }
h2 { font-size: 1rem; margin: 2rem 0 0.5rem; }
ul { list-style: none; margin: 0; padding: 0; }
li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0.25rem 0.75rem; padding: 0.5rem 0; border-top: 1px solid var(--code); }
li .state { color: var(--muted); }
li .detail { flex-basis: 100%; margin: 0; font-size: 0.875rem; white-space: pre-wrap; overflow-wrap: anywhere; }
li form { margin-left: auto; }
.note { margin-top: 2rem; font-size: 0.875rem; }
`

const STATE_TEXT: Record<ConnectionState, string> = {
  idle: "Not connected",
  connecting: "Connecting",
  ready: "Connected",
  reconnecting: "Reconnecting",
  failed: "Failed",
}

// connectPage renders the Connect page (200) with CSP
// default-src 'none'; style-src 'unsafe-inline'; form-action 'self'.
export function connectPage(model: ConnectModel): Response {
  const rows = model.connections.map(connectionRow).join("\n")
  const aliases = model.aliases.length === 0
    ? "<p>No Host aliases found in your SSH config.</p>"
    : `<ul>\n${model.aliases.map((alias) => `<li><a href="${escapeHTML(`servef://ssh/${encodeURIComponent(alias)}/`)}">${escapeHTML(alias)}</a></li>`).join("\n")}\n</ul>`
  const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>servef - Connect</title><style>${PAGE_STYLE}${CONNECT_STYLE}</style></head>
<body><main>
<h1>Connect</h1>
<h2>Connections</h2>
<ul>
${rows}
</ul>
<h2>SSH hosts from your config</h2>
${aliases}
<p class="note">The first connection to a host asks you to confirm it. The app uses your SSH configuration in batch mode: keys from your agent and a verified host key.</p>
<p class="note">Notifications from SSH hosts arrive only while a window for that host is open.</p>
</main></body>
</html>
`
  return new Response(html, { status: 200, headers: CONNECT_HEADERS })
}

function connectionRow(summary: ConnectionSummary): string {
  const href = summary.kind === "ssh" ? `servef://ssh/${encodeURIComponent(summary.id.replace(/^ssh-/, ""))}/` : "servef://local/"
  const parts = [
    `<a href="${escapeHTML(href)}">${escapeHTML(summary.label)}</a>`,
    `<span class="state">${STATE_TEXT[summary.state]}</span>`,
  ]
  if (summary.kind === "ssh") {
    parts.push(`<form method="post" action="${FORGET_PATH}"><input type="hidden" name="id" value="${escapeHTML(summary.id)}"><button>Forget</button></form>`)
  }
  if (summary.detail !== "") parts.push(`<p class="detail">${escapeHTML(summary.detail)}</p>`)
  return `<li>${parts.join("")}</li>`
}

export interface AppPagesDeps {
  model(): Promise<ConnectModel>
  // forget releases the connection and removes it from connections.json.
  forget(id: string): Promise<void>
}

// createAppPages handles servef-app://app requests (the proxy applies the
// Origin guard first): GET /connect, POST /forget (form id=<ssh id>, 303 to
// /connect), else 404. State changes are POST only.
export function createAppPages(deps: AppPagesDeps): (request: Request) => Promise<Response> {
  return async (request) => {
    const path = new URL(request.url).pathname
    const read = request.method === "GET" || request.method === "HEAD"
    switch (path) {
      case "/":
        return read ? seeOther(CONNECT_PATH) : notAllowed("GET, HEAD")
      case CONNECT_PATH: {
        if (!read) return notAllowed("GET, HEAD")
        const page = connectPage(await deps.model())
        return request.method === "HEAD" ? new Response(null, { status: page.status, headers: page.headers }) : page
      }
      case FORGET_PATH: {
        if (request.method !== "POST") return notAllowed("POST")
        const type = (request.headers.get("content-type") ?? "").split(";")[0]?.trim().toLowerCase()
        if (type !== "application/x-www-form-urlencoded") return jsonError(415, "unsupportedMediaType", "Expected a form submission.")
        const body = await request.text()
        if (body.length > FORM_LIMIT) return jsonError(413, "tooLarge", "The form is too large.")
        const id = new URLSearchParams(body).get("id") ?? ""
        if (id.startsWith("ssh-")) await deps.forget(id)
        return seeOther(CONNECT_PATH)
      }
      default:
        return jsonError(404, "notFound", "Unknown app page.")
    }
  }
}

function seeOther(location: string): Response {
  return new Response(null, { status: 303, headers: { location, "cache-control": "no-store" } })
}

function notAllowed(allow: string): Response {
  const response = jsonError(405, "methodNotAllowed", "Method not allowed.")
  response.headers.set("allow", allow)
  return response
}
