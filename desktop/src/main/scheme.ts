import { parseSshLink } from "./ssh-links.ts"
import type { SshLink } from "./ssh-links.ts"

export type SchemeRoute = { kind: "local"; target: string } | { kind: "ssh"; link: SshLink } | { kind: "invalid" }

// routeSchemeURL maps servef://local/<path>?<query>#<fragment> (path / or /view)
// to servef-app://local/<path>?<query>#<fragment> (query and fragment kept
// verbatim) and parses servef://ssh/<host>/... links (decision 0070); the
// caller resolves ssh links against known connections.
export function routeSchemeURL(raw: string): SchemeRoute {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return { kind: "invalid" }
  }
  if (url.protocol !== "servef:" || url.username !== "" || url.password !== "") return { kind: "invalid" }
  if (url.host === "ssh") {
    const link = parseSshLink(url)
    return link === null ? { kind: "invalid" } : { kind: "ssh", link }
  }
  if (url.host !== "local") return { kind: "invalid" }
  const path = url.pathname === "" ? "/" : url.pathname
  if (path !== "/" && path !== "/view") return { kind: "invalid" }
  return { kind: "local", target: `servef-app://local${path}${url.search}${url.hash}` }
}

// schemeArgument returns the first argv entry starting with servef://.
export function schemeArgument(argv: readonly string[]): string | null {
  return argv.find((argument) => argument.startsWith("servef://")) ?? null
}
