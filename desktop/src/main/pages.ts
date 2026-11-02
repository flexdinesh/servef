import type { Instructions } from "./instructions.ts"

// STOP_AND_RETRY_PATH is the proxy-local POST the mismatch page's form targets.
export const STOP_AND_RETRY_PATH = "/_desktop/stop-and-retry"

const PAGE_HEADERS = {
  "content-type": "text/html; charset=utf-8",
  "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'",
  "cache-control": "no-store",
}

const STYLE = `
:root { color-scheme: light dark; --fg: #1f2328; --muted: #59636e; --bg: #ffffff; --code: #f6f8fa; --accent: #0969da; }
@media (prefers-color-scheme: dark) { :root { --fg: #e6edf3; --muted: #9198a1; --bg: #0d1117; --code: #161b22; --accent: #4493f8; } }
body { margin: 0; background: var(--bg); color: var(--fg); font: 15px/1.5 system-ui, sans-serif; }
main { max-width: 40rem; margin: 15vh auto 0; padding: 0 1.5rem; }
h1 { font-size: 1.25rem; margin: 0 0 1rem; }
p { color: var(--muted); margin: 0 0 0.75rem; }
pre { background: var(--code); padding: 0.75rem 1rem; border-radius: 6px; overflow-x: auto; }
.actions { display: flex; gap: 1rem; align-items: center; margin-top: 1.5rem; }
a, button { color: var(--accent); font: inherit; }
button { background: none; border: 1px solid currentColor; border-radius: 6px; padding: 0.25rem 0.75rem; cursor: pointer; }
form { margin: 0; }
`

function escapeHTML(text: string): string {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll("\"", "&quot;").replaceAll("'", "&#39;")
}

function page(title: string, content: string, actions: string): Response {
  const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>${escapeHTML(title)}</title><style>${STYLE}</style></head>
<body><main>
<h1>${escapeHTML(title)}</h1>
${content}
<div class="actions"><a href="">Retry</a>${actions}</div>
</main></body>
</html>
`
  return new Response(html, { status: 503, headers: PAGE_HEADERS })
}

// failurePage renders ensure-failure instructions as an HTML page (503) with
// its own CSP (default-src 'none'; style-src 'unsafe-inline'). Retry is a
// plain reload link; stopAndRetry adds a form POST to STOP_AND_RETRY_PATH.
export function failurePage(instructions: Instructions): Response {
  const paragraphs = instructions.paragraphs.map((text) => `<p>${escapeHTML(text)}</p>`).join("\n")
  const commands = instructions.commands.map((command) => `<pre><code>${escapeHTML(command)}</code></pre>`).join("\n")
  const stop = instructions.stopAndRetry
    ? `<form method="post" action="${STOP_AND_RETRY_PATH}"><button>Stop the running daemon and retry</button></form>`
    : ""
  return page(instructions.title, [paragraphs, commands].filter((part) => part !== "").join("\n"), stop)
}

// unavailablePage renders a daemon that stopped answering.
export function unavailablePage(detail: string): Response {
  return page("The servef daemon stopped answering", `<p>${escapeHTML(detail)}</p>`, "")
}

export function jsonError(status: number, code: string, error: string): Response {
  return new Response(JSON.stringify({ error, code }), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store" },
  })
}
