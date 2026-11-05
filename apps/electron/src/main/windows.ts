export interface WindowInfo {
  id: number
  // origin is the servef-app://<host> origin the window keeps for life.
  origin: string
  // project is the window URL's project query value, else null.
  project: string | null
  focused: boolean
  // focusedAt is the last focus time; 0 when never focused.
  focusedAt: number
}

export type WindowRoute =
  | { kind: "load"; id: number; url: string }
  | { kind: "focus"; id: number }
  | { kind: "new"; url: string }

// routeWindow picks the window for a servef-app:// target URL among windows on
// the target's origin: a window on the target's project loads it; a target
// without a project focuses the latest window. Other origins never match.
export function routeWindow(windows: readonly WindowInfo[], target: string): WindowRoute {
  const parsed = targetOfURL(target)
  if (parsed === null) return { kind: "new", url: target }
  const { origin, project } = parsed
  const sameOrigin = windows.filter((window) => window.origin === origin)
  const candidates = project === null ? sameOrigin : sameOrigin.filter((window) => window.project === project)
  const latest = [...candidates].sort(byRecency)[0]
  if (latest === undefined) return { kind: "new", url: target }
  return project === null ? { kind: "focus", id: latest.id } : { kind: "load", id: latest.id, url: target }
}

function byRecency(a: WindowInfo, b: WindowInfo): number {
  if (a.focused !== b.focused) return a.focused ? -1 : 1
  return b.focusedAt - a.focusedAt
}

// targetOfURL returns the origin (servef-app://<host>) and project of a
// servef-app:// URL without credentials; null otherwise.
export function targetOfURL(url: string): { origin: string; project: string | null } | null {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }
  if (parsed.protocol !== "servef-app:" || parsed.host === "" || parsed.username !== "" || parsed.password !== "") return null
  const project = parsed.searchParams.get("project")
  return { origin: `servef-app://${parsed.host}`, project: project === null || project === "" ? null : project }
}

// connectionOfOrigin returns the connection id (host) of a servef-app://<host>
// origin; null otherwise.
export function connectionOfOrigin(origin: string): string | null {
  const target = targetOfURL(origin)
  return target === null || target.origin !== origin ? null : origin.slice("servef-app://".length)
}

// projectOfURL returns the project query parameter of a servef-app:// URL.
export function projectOfURL(url: string): string | null {
  return targetOfURL(url)?.project ?? null
}
