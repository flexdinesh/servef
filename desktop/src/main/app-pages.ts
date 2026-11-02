// App pages on servef-app://app (decisions 0070, 0071): the Connect page,
// its POST actions, and the "Open a link" form. Static HTML, no scripts.
import { connectionOrigin } from "./connections.ts"
import type { ConnectionState, ConnectionSummary } from "./connections.ts"
import { PASTE_LIMIT } from "./location.ts"
import { escapeHTML, jsonError, PAGE_STYLE, PAGE_THEME_COLOR } from "./pages.ts"

export const CONNECT_PATH = "/connect"
export const FORGET_PATH = "/forget"
export const OPEN_PATH = "/open"

export interface ConnectModel {
  // connections: local first, then known ssh hosts, then running CLI servers
  // (Connections.summaries()); CLI servers render as their own section.
  connections: ConnectionSummary[]
  // aliases are Host aliases from the ssh config not already known.
  aliases: string[]
  // serversDetail is why CLI servers could not be listed; "" when listed.
  serversDetail: string
}

// PastedLink is the "Open a link" field after a failed open: the text and why.
export interface PastedLink {
  value: string
  error: string
}

// OpenOutcome: ok opened the link in a window; else reason is shown.
export type OpenOutcome = { ok: true } | { ok: false; reason: string }

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
.open { display: flex; flex-wrap: wrap; gap: 0.5rem; margin: 0.5rem 0; }
.open input { flex: 1 1 20rem; font: inherit; color: var(--fg); background: var(--code); border: 1px solid var(--muted); border-radius: 6px; padding: 0.25rem 0.5rem; }
.error { color: var(--fg); }
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
// pasted refills the "Open a link" field with its error.
export function connectPage(model: ConnectModel, pasted: PastedLink | null = null): Response {
  const rows = model.connections.filter((summary) => summary.kind !== "cli").map(connectionRow).join("\n")
  const servers = model.connections.filter((summary) => summary.kind === "cli")
  const serverList = servers.length === 0
    ? `<p>${escapeHTML(model.serversDetail === "" ? "No servef servers are running." : `Could not list servef servers: ${model.serversDetail}`)}</p>`
    : `<ul>\n${servers.map((summary) => `<li><a href="${escapeHTML(connectionOrigin(summary.id))}/">${escapeHTML(summary.label)}</a></li>`).join("\n")}\n</ul>`
  const error = pasted === null ? "" : `\n<p class="error" role="alert">${escapeHTML(pasted.error)}</p>`
  const aliases = model.aliases.length === 0
    ? "<p>No Host aliases found in your SSH config.</p>"
    : `<ul>\n${model.aliases.map((alias) => `<li><a href="${escapeHTML(`servef://ssh/${encodeURIComponent(alias)}/`)}">${escapeHTML(alias)}</a></li>`).join("\n")}\n</ul>`
  const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8">${PAGE_THEME_COLOR}<title>servef - Connect</title><style>${PAGE_STYLE}${CONNECT_STYLE}</style></head>
<body><main>
<h1>Connect</h1>
<h2>Connections</h2>
<ul>
${rows}
</ul>
<h2>SSH hosts from your config</h2>
${aliases}
<h2>Servers on this computer</h2>
${serverList}
<h2>Open a link</h2>
<form class="open" method="get" action="${OPEN_PATH}"><input type="text" name="url" aria-label="servef link" placeholder="servef://… or http://localhost:7971/…" value="${escapeHTML(pasted?.value ?? "")}" required><button>Open</button></form>${error}
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
  // open resolves a pasted link and opens it in a window (decision 0071).
  open(raw: string): Promise<OpenOutcome>
}

// createAppPages handles servef-app://app requests (the proxy applies the
// Origin guard first): GET /connect, POST /forget (form id=<ssh id>, 303 to
// /connect), GET /open?url=<pasted link> (opened: 303 to /connect; else the
// Connect page with the reason), else 404. Stored state changes are POST only;
// /open only opens windows, as a clicked servef:// link does.
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
      case OPEN_PATH: {
        if (!read) return notAllowed("GET, HEAD")
        if (request.method === "HEAD") return new Response(null, { status: 200, headers: CONNECT_HEADERS })
        const value = new URL(request.url).searchParams.get("url") ?? ""
        const outcome = await deps.open(value)
        if (outcome.ok) return seeOther(CONNECT_PATH)
        return connectPage(await deps.model(), { value: value.slice(0, PASTE_LIMIT), error: outcome.reason })
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
