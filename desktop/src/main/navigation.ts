export type NavigationDecision =
  | { kind: "allow" }
  | { kind: "scheme"; url: string }
  | { kind: "external"; url: string }
  | { kind: "deny" }

// navigationPolicy decides a renderer navigation or window.open target.
// URL.origin is "null" for servef-app:, so origins compare by scheme and host.
export function navigationPolicy(url: string, connectionOrigin: string): NavigationDecision {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return { kind: "deny" }
  }
  if (parsed.protocol === "servef-app:" && parsed.username === "" && parsed.password === ""
    && `${parsed.protocol}//${parsed.host}` === connectionOrigin) return { kind: "allow" }
  if (parsed.protocol === "servef:") return { kind: "scheme", url }
  if (parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "mailto:") return { kind: "external", url }
  return { kind: "deny" }
}

// permissionAllowed allows clipboard-sanitized-write only.
export function permissionAllowed(permission: string): boolean {
  return permission === "clipboard-sanitized-write"
}
