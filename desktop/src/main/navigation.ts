export type NavigationDecision =
  | { kind: "allow" }
  | { kind: "scheme"; url: string }
  // connection: a servef-app:// URL on another origin; main routes it to a
  // window of that origin only for known connections.
  | { kind: "connection"; url: string }
  | { kind: "external"; url: string }
  | { kind: "deny" }

// navigationPolicy decides a renderer navigation or window.open target.
// URL.origin is "null" for servef-app:, so origins compare by scheme and host.
export function navigationPolicy(url: string, windowOrigin: string): NavigationDecision {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return { kind: "deny" }
  }
  if (parsed.protocol === "servef-app:") {
    if (parsed.username !== "" || parsed.password !== "" || parsed.host === "") return { kind: "deny" }
    return `${parsed.protocol}//${parsed.host}` === windowOrigin ? { kind: "allow" } : { kind: "connection", url }
  }
  if (parsed.protocol === "servef:") return { kind: "scheme", url }
  if (parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "mailto:") return { kind: "external", url }
  return { kind: "deny" }
}

// permissionAllowed allows clipboard-sanitized-write only.
export function permissionAllowed(permission: string): boolean {
  return permission === "clipboard-sanitized-write"
}
