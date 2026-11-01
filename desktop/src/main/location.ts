// Pasted links (decision 0071): pure parsing of the Connect page's "Open a
// link" field. Pasted text is untrusted; the caller verifies cli ports
// against `servef control list`.
import { cliId } from "./cli-servers.ts"

export type PastedLocation =
  | { kind: "scheme"; url: string }
  // target is servef-app://cli-<port>/<path>?<query>#<fragment>.
  | { kind: "cli"; port: number; target: string }
  | { kind: "invalid"; reason: string }

export const NOT_A_SERVEF_LINK = "Not a servef link."

// PASTE_LIMIT bounds a pasted link.
export const PASTE_LIMIT = 8192

const LOOPBACK_HOSTS: ReadonlySet<string> = new Set(["localhost", "127.0.0.1", "[::1]"])

// parsePastedLocation accepts servef:// links (opened as the OS handler
// would) and http://localhost|127.0.0.1|[::1]:<port> URLs on / or /view, any
// query and fragment, no credentials. A missing port is 80.
export function parsePastedLocation(raw: string): PastedLocation {
  const text = raw.trim()
  if (text === "" || text.length > PASTE_LIMIT) return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  let url: URL
  try {
    url = new URL(text)
  } catch {
    return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  }
  if (url.username !== "" || url.password !== "") return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  if (url.protocol === "servef:") return { kind: "scheme", url: text }
  if (url.protocol !== "http:" || !LOOPBACK_HOSTS.has(url.hostname)) return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  if (url.pathname !== "/" && url.pathname !== "/view") return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  const port = url.port === "" ? 80 : Number(url.port)
  if (port < 1) return { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  return { kind: "cli", port, target: `servef-app://${cliId(port)}${url.pathname}${url.search}${url.hash}` }
}
