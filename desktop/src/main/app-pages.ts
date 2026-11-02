// App pages on servef-app://app (decisions 0070, 0071, 0075): the Connect
// page, ssh host pages and the folder picker, their POST actions, and the
// "Open a link" form. Static HTML, no scripts.
import { connectionOrigin } from "./connections.ts"
import type { ConnectionState, ConnectionSummary } from "./connections.ts"
import type { Instructions } from "./instructions.ts"
import { PASTE_LIMIT } from "./location.ts"
import { projectLanding } from "./open-paths.ts"
import { escapeHTML, failureContent, jsonError, PAGE_STYLE, PAGE_THEME_COLOR, reconnectingPage } from "./pages.ts"
import type { ActionOutcome, BrowseView, HostView, RemoteHosts, ServersView } from "./remote-hosts.ts"
import type { SshFailure } from "./ssh-errors.ts"

export const CONNECT_PATH = "/connect"
export const FORGET_PATH = "/forget"
export const OPEN_PATH = "/open"
// SSH_PATH: POST alias=<config alias> stores the host and opens its page,
// SSH_PATH/<id> (host page), /<id>/browse?path= (picker), and POST
// /<id>/open (path=), /<id>/server (origin=), /<id>/stop-and-retry.
export const SSH_PATH = "/ssh"

export interface ConnectModel {
  // connections: local first, then known ssh hosts, known http servers,
  // then running CLI servers (Connections.summaries()); CLI servers render
  // as their own section.
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

const PAGE_HEADERS = {
  "content-type": "text/html; charset=utf-8",
  "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'",
  "cache-control": "no-store",
}

// FORM_LIMIT bounds a POST body (an id, alias, origin, or remote path).
const FORM_LIMIT = 8192

const APP_STYLE = `
main { margin-top: 10vh; padding-bottom: 3rem; }
h2 { font-size: 1rem; margin: 2rem 0 0.5rem; }
ul { list-style: none; margin: 0; padding: 0; }
li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0.25rem 0.75rem; padding: 0.5rem 0; border-top: 1px solid var(--code); }
li .state, li .tag { color: var(--muted); }
li .detail { flex-basis: 100%; margin: 0; font-size: 0.875rem; white-space: pre-wrap; overflow-wrap: anywhere; }
li form { margin-left: auto; }
li form.host { margin-left: 0; }
button.link { border: none; padding: 0; text-decoration: underline; }
.open { display: flex; flex-wrap: wrap; gap: 0.5rem; margin: 0.5rem 0; }
.open input { flex: 1 1 20rem; font: inherit; color: var(--fg); background: var(--code); border: 1px solid var(--muted); border-radius: 6px; padding: 0.25rem 0.5rem; }
.path { overflow-wrap: anywhere; }
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

function document(title: string, body: string, status = 200): Response {
  const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8">${PAGE_THEME_COLOR}<title>${escapeHTML(title)}</title><style>${PAGE_STYLE}${APP_STYLE}</style></head>
<body><main>
${body}
</main></body>
</html>
`
  return new Response(html, { status, headers: PAGE_HEADERS })
}

function alert(error: string): string {
  return error === "" ? "" : `<p class="error" role="alert">${escapeHTML(error)}</p>\n`
}

// postButton is a POST form with hidden fields and one button.
function postButton(action: string, fields: Record<string, string>, label: string, className = ""): string {
  const inputs = Object.entries(fields).map(([name, value]) => `<input type="hidden" name="${escapeHTML(name)}" value="${escapeHTML(value)}">`).join("")
  const form = className === "" ? "" : ` class="${className}"`
  const button = className === "" ? "<button>" : "<button class=\"link\">"
  return `<form${form} method="post" action="${escapeHTML(action)}">${inputs}${button}${escapeHTML(label)}</button></form>`
}

function hostPath(id: string, rest = ""): string {
  return `${SSH_PATH}/${id}${rest}`
}

function browsePath(id: string, path: string, hidden: boolean): string {
  const query = new URLSearchParams({ path })
  if (hidden) query.set("hidden", "1")
  return hostPath(id, `/browse?${query.toString()}`)
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
  const aliases = model.aliases.length === 0
    ? "<p>No Host aliases found in your SSH config.</p>"
    : `<ul>\n${model.aliases.map((alias) => `<li>${postButton(SSH_PATH, { alias }, alias, "host")}</li>`).join("\n")}\n</ul>`
  return document("servef - Connect", `<h1>Connect</h1>
<h2>Connections</h2>
<ul>
${rows}
</ul>
<h2>SSH hosts from your config</h2>
${aliases}
<h2>Servers on this computer</h2>
${serverList}
<h2>Open a link</h2>
<form class="open" method="get" action="${OPEN_PATH}"><input type="text" name="url" aria-label="servef link" placeholder="servef://… or http://192.168.1.20:7971/…" value="${escapeHTML(pasted?.value ?? "")}" required><button>Open</button></form>
${alert(pasted?.error ?? "")}<p class="note">The app uses your SSH configuration in batch mode: keys from your agent and a verified host key.</p>
<p class="note">Notifications from SSH hosts arrive only while a window for that host is open.</p>`)
}

function connectionRow(summary: ConnectionSummary): string {
  const href = summary.kind === "ssh"
    ? hostPath(summary.id)
    : summary.kind === "http" ? `${connectionOrigin(summary.id)}/` : "servef://local/"
  const parts = [`<a href="${escapeHTML(href)}">${escapeHTML(summary.label)}</a>`]
  if (summary.kind === "http") parts.push("<span class=\"tag\">http</span>")
  else parts.push(`<span class="state">${STATE_TEXT[summary.state]}</span>`)
  if (summary.kind === "ssh" || summary.kind === "http") parts.push(postButton(FORGET_PATH, { id: summary.id }, "Forget"))
  if (summary.detail !== "") parts.push(`<p class="detail">${escapeHTML(summary.detail)}</p>`)
  return `<li>${parts.join("")}</li>`
}

function serverList(id: string, servers: ServersView, empty: string): string {
  if (!servers.ok) return `<p>${escapeHTML(`Could not list servef servers: ${servers.detail}`)}</p>`
  if (servers.servers.length === 0) return `<p>${escapeHTML(empty)}</p>`
  const rows = servers.servers.map((server) => {
    const tag = server.direct ? "" : "<span class=\"tag\">loopback only</span>"
    return `<li>${postButton(hostPath(id, "/server"), { origin: server.origin }, server.label, "host")}${tag}</li>`
  })
  return `<ul>\n${rows.join("\n")}\n</ul>`
}

// hostPage renders an ssh host's page (decision 0075): served folders,
// running servers, and Browse folders; a failure shows its instructions.
export function hostPage(view: HostView, instructions: (failure: SshFailure) => Instructions, error = ""): Response {
  switch (view.kind) {
    case "unknown":
      return document("servef - Unknown host", `<h1>Unknown SSH host</h1>\n<p><a href="${CONNECT_PATH}">Connect</a></p>`, 404)
    case "connecting":
      return reconnectingPage(`Reconnecting to ${view.destination}`, view.detail, hostPath(view.id))
    case "failed": {
      const steps = instructions(view.failure)
      const servers = view.servers === null
        ? ""
        : `<h2>Servers on ${escapeHTML(view.destination)} reachable without the forward</h2>\n${serverList(view.id, view.servers, "No servef server there listens on an address this computer can reach.")}`
      const stop = steps.stopAndRetry ? postButton(hostPath(view.id, "/stop-and-retry"), {}, "Stop the running daemon and retry") : ""
      return document(`servef - ${steps.title}`, `<h1>${escapeHTML(steps.title)}</h1>
${alert(error)}${failureContent(steps, servers)}
<div class="actions"><a href="${escapeHTML(hostPath(view.id))}">Retry</a>${stop}<a href="${CONNECT_PATH}">Connect</a></div>`, 503)
    }
    case "ready": {
      const origin = connectionOrigin(view.id)
      const roots = view.roots.length === 0
        ? "<p>No folders yet.</p>"
        : `<ul>\n${view.roots.map((root) => `<li><a href="${escapeHTML(projectLanding(origin, root))}">${escapeHTML(root)}</a></li>`).join("\n")}\n</ul>`
      return document(`servef - ${view.destination}`, `<h1>${escapeHTML(view.destination)}</h1>
${alert(error)}<h2>Folders</h2>
${roots}
<p><a href="${escapeHTML(browsePath(view.id, "", false))}">Browse folders</a></p>
<h2>Servers</h2>
${serverList(view.id, view.servers, "No servef servers are running there.")}
<p class="note"><a href="${CONNECT_PATH}">Connect</a></p>`)
    }
  }
}

// browsePage renders the folder picker for one listing.
export function browsePage(view: BrowseView, instructions: (failure: SshFailure) => Instructions, error = ""): Response {
  switch (view.kind) {
    case "host":
      return hostPage(view.view, instructions, error)
    case "error": {
      const text = view.upgrade
        ? `servef on ${view.destination} cannot list folders. Upgrade servef there, then Retry.`
        : view.error
      return document(`servef - Browse ${view.destination}`, `<h1>Browse ${escapeHTML(view.destination)}</h1>
${view.path === "" ? "" : `<p class="path"><code>${escapeHTML(view.path)}</code></p>\n`}${alert(text)}<div class="actions"><a href="${escapeHTML(browsePath(view.id, view.path, false))}">Retry</a><a href="${escapeHTML(browsePath(view.id, "", false))}">Home folder</a><a href="${escapeHTML(hostPath(view.id))}">${escapeHTML(view.destination)}</a></div>`, view.upgrade ? 501 : 200)
    }
    case "listing": {
      const { id, listing, hidden } = view
      const entries = listing.entries.filter((entry) => hidden || !entry.hidden)
      const rows = [
        ...(listing.parent === "" ? [] : [`<li><a href="${escapeHTML(browsePath(id, listing.parent, hidden))}">..</a></li>`]),
        ...entries.map((entry) => {
          const tag = entry.repository ? "<span class=\"tag\">repository</span>" : ""
          return `<li><a href="${escapeHTML(browsePath(id, entry.path, hidden))}">${escapeHTML(entry.name)}</a>${tag}</li>`
        }),
      ]
      const toggle = hidden
        ? `<a href="${escapeHTML(browsePath(id, listing.path, false))}">Hide hidden folders</a>`
        : `<a href="${escapeHTML(browsePath(id, listing.path, true))}">Show hidden folders</a>`
      const truncated = listing.truncated ? "<p class=\"note\">Only the first folders are listed.</p>\n" : ""
      return document(`servef - Browse ${view.destination}`, `<h1>Browse ${escapeHTML(view.destination)}</h1>
<p class="path"><code>${escapeHTML(listing.path)}</code></p>
${alert(error)}${postButton(hostPath(id, "/open"), { path: listing.path }, "Open this folder")}
<ul>
${rows.join("\n")}
</ul>
${truncated}<div class="actions">${toggle}<a href="${escapeHTML(hostPath(id))}">${escapeHTML(view.destination)}</a></div>`)
    }
  }
}

export interface AppPagesDeps {
  model(): Promise<ConnectModel>
  // forget releases an ssh host or http server and removes it from connections.json.
  forget(id: string): Promise<void>
  // open resolves a pasted link and opens it in a window (decision 0071).
  open(raw: string): Promise<OpenOutcome>
  // addSsh stores a config alias the user chose; its id, or null when the
  // alias is not in the config.
  addSsh(alias: string): string | null
  hosts: RemoteHosts
  // stopAndRetry stops an ssh host's mismatched daemon and connects again.
  stopAndRetry(id: string): Promise<void>
  instructions(failure: SshFailure): Instructions
}

const SSH_ID = "ssh-[a-z0-9-]{1,45}"
const HOST_ROUTE = new RegExp(`^${SSH_PATH}/(${SSH_ID})(/browse|/open|/server|/stop-and-retry)?$`)

// createAppPages handles servef-app://app requests (the proxy applies the
// Origin guard first): GET /connect, POST /forget (form id=<ssh or http
// id>), GET /open?url=<pasted link>, POST /ssh (alias=), and the host routes
// (SSH_PATH), else 404. Stored state changes and remote actions are POST
// only; GET /open only opens windows, as a clicked servef:// link does.
export function createAppPages(deps: AppPagesDeps): (request: Request) => Promise<Response> {
  const head = (request: Request, page: Response): Response =>
    request.method === "HEAD" ? new Response(null, { status: page.status, headers: page.headers }) : page

  return async (request) => {
    const url = new URL(request.url)
    const path = url.pathname
    const read = request.method === "GET" || request.method === "HEAD"
    const form = async (): Promise<URLSearchParams | Response> => {
      if (request.method !== "POST") return notAllowed("POST")
      const type = (request.headers.get("content-type") ?? "").split(";")[0]?.trim().toLowerCase()
      if (type !== "application/x-www-form-urlencoded") return jsonError(415, "unsupportedMediaType", "Expected a form submission.")
      const body = await request.text()
      if (body.length > FORM_LIMIT) return jsonError(413, "tooLarge", "The form is too large.")
      return new URLSearchParams(body)
    }
    const host = HOST_ROUTE.exec(path)
    if (host !== null) {
      const id = host[1] ?? ""
      const action = host[2] ?? ""
      if (action === "") return read ? head(request, hostPage(await deps.hosts.host(id), deps.instructions)) : notAllowed("GET, HEAD")
      if (action === "/browse") {
        if (!read) return notAllowed("GET, HEAD")
        const hidden = url.searchParams.get("hidden") === "1"
        return head(request, browsePage(await deps.hosts.browse(id, url.searchParams.get("path") ?? "", hidden), deps.instructions))
      }
      const fields = await form()
      if (fields instanceof Response) return fields
      if (action === "/stop-and-retry") {
        await deps.stopAndRetry(id)
        return seeOther(hostPath(id))
      }
      if (action === "/open") {
        const folder = fields.get("path") ?? ""
        const outcome = await deps.hosts.openFolder(id, folder)
        if (outcome.ok) return seeOther(hostPath(id))
        return browsePage(await deps.hosts.browse(id, folder, false), deps.instructions, failed(outcome))
      }
      const outcome = await deps.hosts.openServer(id, fields.get("origin") ?? "")
      if (outcome.ok) return seeOther(hostPath(id))
      return hostPage(await deps.hosts.host(id), deps.instructions, failed(outcome))
    }
    switch (path) {
      case "/":
        return read ? seeOther(CONNECT_PATH) : notAllowed("GET, HEAD")
      case CONNECT_PATH:
        return read ? head(request, connectPage(await deps.model())) : notAllowed("GET, HEAD")
      case OPEN_PATH: {
        if (!read) return notAllowed("GET, HEAD")
        if (request.method === "HEAD") return new Response(null, { status: 200, headers: PAGE_HEADERS })
        const value = url.searchParams.get("url") ?? ""
        const outcome = await deps.open(value)
        if (outcome.ok) return seeOther(CONNECT_PATH)
        return connectPage(await deps.model(), { value: value.slice(0, PASTE_LIMIT), error: outcome.reason })
      }
      case SSH_PATH: {
        const fields = await form()
        if (fields instanceof Response) return fields
        const id = deps.addSsh(fields.get("alias") ?? "")
        return seeOther(id === null ? CONNECT_PATH : hostPath(id))
      }
      case FORGET_PATH: {
        const fields = await form()
        if (fields instanceof Response) return fields
        const id = fields.get("id") ?? ""
        if (id.startsWith("ssh-") || id.startsWith("http-")) await deps.forget(id)
        return seeOther(CONNECT_PATH)
      }
      default:
        return jsonError(404, "notFound", "Unknown app page.")
    }
  }
}

function failed(outcome: ActionOutcome): string {
  return outcome.ok ? "" : outcome.error
}

function seeOther(location: string): Response {
  return new Response(null, { status: 303, headers: { location, "cache-control": "no-store" } })
}

function notAllowed(allow: string): Response {
  const response = jsonError(405, "methodNotAllowed", "Method not allowed.")
  response.headers.set("allow", allow)
  return response
}
