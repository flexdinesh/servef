import { serverLocation } from "./location.ts"
import type { ServerLocation } from "./location.ts"

export type SchemeRoute = { kind: "local"; target: string } | ServerLocation | { kind: "invalid" }

// routeSchemeURL maps servef://local/<path>?<query>#<fragment> (path / or
// /view) to servef-app://local/<path>?<query>#<fragment> (query and fragment
// kept verbatim), and servef://<host>:<port>/<path>?… (decision 0075), the
// web URL http://<host>:<port>/<path>?… with the scheme swapped, to a server
// location: a CLI server for loopback hosts, else an http server. The port
// is required. Anything else (servef://ssh/… included) is invalid.
export function routeSchemeURL(raw: string): SchemeRoute {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return { kind: "invalid" }
  }
  if (url.protocol !== "servef:" || url.username !== "" || url.password !== "") return { kind: "invalid" }
  const path = url.pathname === "" ? "/" : url.pathname
  if (path !== "/" && path !== "/view") return { kind: "invalid" }
  if (url.host === "local") return { kind: "local", target: `servef-app://local${path}${url.search}${url.hash}` }
  // servef: is not a special scheme: its host is opaque and has no default
  // port. Parsing the same URL as http normalizes the host (case, IPv4).
  // servef://local:<port> is neither: local names the daemon only.
  if (url.port === "" || url.hostname === "local") return { kind: "invalid" }
  let web: URL
  try {
    web = new URL(`http://${url.host}${path}${url.search}${url.hash}`)
  } catch {
    return { kind: "invalid" }
  }
  return serverLocation(web, Number(url.port)) ?? { kind: "invalid" }
}

// schemeArgument returns the first argv entry starting with servef://.
export function schemeArgument(argv: readonly string[]): string | null {
  return argv.find((argument) => argument.startsWith("servef://")) ?? null
}
