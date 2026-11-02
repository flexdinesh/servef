import type { Instructions } from "./instructions.ts"

// STOP_AND_RETRY_PATH is the proxy-local POST the mismatch page's form targets.
export const STOP_AND_RETRY_PATH = "/_desktop/stop-and-retry"

const PAGE_HEADERS = {
  "content-type": "text/html; charset=utf-8",
  "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'",
  "cache-control": "no-store",
}

// PAGE_STYLE is the shared stylesheet of app-rendered pages (no external assets).
export const PAGE_STYLE = `
:root { color-scheme: light dark; --fg: #1f2328; --muted: #59636e; --bg: #ffffff; --code: #f6f8fa; --accent: #0969da; }
@media (prefers-color-scheme: dark) { :root { --fg: #e6edf3; --muted: #9198a1; --bg: #0d1117; --code: #161b22; --accent: #4493f8; } }
body { margin: 0; padding-top: env(titlebar-area-height, 0px); background: var(--bg); color: var(--fg); font: 15px/1.5 system-ui, sans-serif; }
body::before { position: fixed; inset: 0 0 auto; height: env(titlebar-area-height, 0px); app-region: drag; content: ""; }
main { max-width: 40rem; margin: 15vh auto 0; padding: 0 1.5rem; }
h1 { font-size: 1.25rem; margin: 0 0 1rem; }
p { color: var(--muted); margin: 0 0 0.75rem; }
pre { background: var(--code); padding: 0.75rem 1rem; border-radius: 6px; overflow-x: auto; }
.actions { display: flex; gap: 1rem; align-items: center; margin-top: 1.5rem; }
a, button { color: var(--accent); font: inherit; }
button { background: none; border: 1px solid currentColor; border-radius: 6px; padding: 0.25rem 0.75rem; cursor: pointer; }
form { margin: 0; }
`

// PAGE_THEME_COLOR reports PAGE_STYLE's --bg as the theme color, so the
// desktop title band matches app-rendered pages (decision 0075).
export const PAGE_THEME_COLOR = `<meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)"><meta name="theme-color" content="#0d1117" media="(prefers-color-scheme: dark)">`

export function escapeHTML(text: string): string {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll("\"", "&quot;").replaceAll("'", "&#39;")
}

function page(title: string, content: string, actions: string, head = "", status = 503): Response {
  const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8">${PAGE_THEME_COLOR}${head}<title>${escapeHTML(title)}</title><style>${PAGE_STYLE}</style></head>
<body><main>
<h1>${escapeHTML(title)}</h1>
${content}
<div class="actions"><a href="">Retry</a>${actions}</div>
</main></body>
</html>
`
  return new Response(html, { status, headers: PAGE_HEADERS })
}

// failurePage renders ensure-failure instructions as an HTML page (503) with
// its own CSP (default-src 'none'; style-src 'unsafe-inline'). Retry is a
// plain reload link; stopAndRetry adds a form POST to STOP_AND_RETRY_PATH.
// extra is trusted HTML (already escaped) placed after the commands.
export function failurePage(instructions: Instructions, extra = ""): Response {
  return page(instructions.title, failureContent(instructions, extra), failureActions(instructions))
}

// failureContent is failurePage's body: paragraphs, commands, then extra.
export function failureContent(instructions: Instructions, extra = ""): string {
  const paragraphs = instructions.paragraphs.map((text) => `<p>${escapeHTML(text)}</p>`).join("\n")
  const commands = instructions.commands.map((command) => `<pre><code>${escapeHTML(command)}</code></pre>`).join("\n")
  return [paragraphs, commands, extra].filter((part) => part !== "").join("\n")
}

// failureActions is the stop-and-retry form for a mismatch, else "".
export function failureActions(instructions: Instructions): string {
  return instructions.stopAndRetry
    ? `<form method="post" action="${STOP_AND_RETRY_PATH}"><button>Stop the running daemon and retry</button></form>`
    : ""
}

// unavailablePage renders a daemon that stopped answering.
export function unavailablePage(detail: string): Response {
  return page("The servef daemon stopped answering", `<p>${escapeHTML(detail)}</p>`, "")
}

// noCliServerPage: no running servef server listens on port (404).
export function noCliServerPage(port: number): Response {
  return page(`No servef server is running on port ${port}`, `<p>Start one with <code>servef browser</code>, or open the address it prints.</p>`, "", "", 404)
}

// cliStoppedPage: the CLI server this window showed stopped (503). The app
// never starts CLI servers.
export function cliStoppedPage(port: number): Response {
  return page(`The servef server on port ${port} stopped`, `<p>Start it again in a terminal, then Retry:</p>\n<pre><code>servef browser --port ${port}</code></pre>`, "")
}

// serverUnreachablePage: an http or forwarded remote server did not answer
// (503). The app never starts remote servers.
export function serverUnreachablePage(label: string, detail: string): Response {
  return page(`Cannot reach the servef server at ${label}`, `<p>${escapeHTML(detail)}</p>\n<p>Check that servef still runs there and that this computer can reach it, then Retry.</p>`, "")
}

// RECONNECT_REFRESH_SECONDS is how often the reconnecting page reloads itself
// (a meta refresh; the page has no scripts).
export const RECONNECT_REFRESH_SECONDS = 3

// reconnectingPage renders an ssh connection waiting to reconnect (503): it
// reloads (or loads url) every RECONNECT_REFRESH_SECONDS and shows the last failure.
export function reconnectingPage(title: string, detail: string, url = ""): Response {
  const content = [
    detail === "" ? "" : `<p>${escapeHTML(detail)}</p>`,
    "<p>servef keeps trying while this window is open. This page reloads on its own.</p>",
  ].filter((part) => part !== "").join("\n")
  return page(title, content, "", `<meta http-equiv="refresh" content="${RECONNECT_REFRESH_SECONDS}${url === "" ? "" : `; url=${escapeHTML(url)}`}">`)
}

export function jsonError(status: number, code: string, error: string): Response {
  return new Response(JSON.stringify({ error, code }), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store" },
  })
}
