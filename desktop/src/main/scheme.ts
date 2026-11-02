export type SchemeRoute = { connection: "local"; target: string } | { reject: "remote" | "invalid" }

// routeSchemeURL maps servef://local/<path>?<query>#<fragment> (path / or /view)
// to servef-app://local/<path>?<query>#<fragment>. Query and fragment are kept verbatim.
export function routeSchemeURL(raw: string): SchemeRoute {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return { reject: "invalid" }
  }
  if (url.protocol !== "servef:" || url.username !== "" || url.password !== "") return { reject: "invalid" }
  if (url.host === "ssh") return { reject: "remote" }
  if (url.host !== "local") return { reject: "invalid" }
  const path = url.pathname === "" ? "/" : url.pathname
  if (path !== "/" && path !== "/view") return { reject: "invalid" }
  return { connection: "local", target: `servef-app://local${path}${url.search}${url.hash}` }
}

// schemeArgument returns the first argv entry starting with servef://.
export function schemeArgument(argv: readonly string[]): string | null {
  return argv.find((argument) => argument.startsWith("servef://")) ?? null
}
