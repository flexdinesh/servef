export interface WindowInfo {
  id: number
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

// routeWindow picks the window for a servef-app:// target URL: a window on the
// target's project loads it; a target without a project focuses the latest window.
export function routeWindow(windows: readonly WindowInfo[], target: string): WindowRoute {
  const project = projectOfURL(target)
  const candidates = project === null ? windows : windows.filter((window) => window.project === project)
  const latest = [...candidates].sort(byRecency)[0]
  if (latest === undefined) return { kind: "new", url: target }
  return project === null ? { kind: "focus", id: latest.id } : { kind: "load", id: latest.id, url: target }
}

function byRecency(a: WindowInfo, b: WindowInfo): number {
  if (a.focused !== b.focused) return a.focused ? -1 : 1
  return b.focusedAt - a.focusedAt
}

// projectOfURL returns the project query parameter of a servef-app:// URL.
export function projectOfURL(url: string): string | null {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }
  if (parsed.protocol !== "servef-app:") return null
  const project = parsed.searchParams.get("project")
  return project === null || project === "" ? null : project
}
