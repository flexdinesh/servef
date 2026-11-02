// BrowserWindow adapter: creation, security handlers, project tracking, and routing.
import { BrowserWindow, session, shell } from "electron"
import { readFileSync, writeFileSync } from "node:fs"
import { navigationPolicy, permissionAllowed } from "./navigation.ts"
import { parseWindowState, serializeWindowState, type WindowBounds } from "./window-state.ts"
import { projectOfURL, routeWindow, type WindowInfo } from "./windows.ts"

export interface WindowManagerOptions {
  // origin is the connection origin windows load, servef-app://local.
  origin: string
  // preload is the absolute path of build/preload.cjs.
  preload: string
  // stateFile is userData/window-state.json.
  stateFile: string
  // onScheme receives servef:// links the renderer navigates to.
  onScheme(url: string): void
}

export interface WindowManager {
  infos(): WindowInfo[]
  // route applies routeWindow to a servef-app:// target.
  route(target: string): void
  newWindow(url: string): void
  focusedWindow(): BrowserWindow | null
  focusedProject(): string | null
  // shownProjects lists the distinct projects any window shows.
  shownProjects(): string[]
  onProjectsChanged(listener: () => void): () => void
}

interface Tracked {
  window: BrowserWindow
  project: string | null
  focused: boolean
  focusedAt: number
}

export function createWindowManager(options: WindowManagerOptions): WindowManager {
  const tracked = new Map<number, Tracked>()
  const listeners = new Set<() => void>()
  let bounds = readBounds(options.stateFile)

  const projectsChanged = (): void => {
    for (const listener of listeners) listener()
  }

  session.defaultSession.setPermissionRequestHandler((_contents, permission, callback) => {
    callback(permissionAllowed(permission))
  })
  session.defaultSession.setPermissionCheckHandler((_contents, permission) => permissionAllowed(permission))

  // decide runs the navigation policy; true when the navigation may proceed.
  // Only main-frame user navigations act on scheme and external links.
  const decide = (url: string, act: boolean): boolean => {
    const decision = navigationPolicy(url, options.origin)
    if (decision.kind === "allow") return true
    if (!act) return false
    if (decision.kind === "scheme") options.onScheme(decision.url)
    if (decision.kind === "external") {
      shell.openExternal(decision.url).catch((error: unknown) => console.error("servef: open external:", error))
    }
    return false
  }

  const newWindow = (url: string): void => {
    // A window beside open ones takes the size only; the OS places it.
    const placed: WindowBounds = tracked.size === 0 ? bounds : { width: bounds.width, height: bounds.height }
    const window = new BrowserWindow({
      ...placed,
      show: true,
      webPreferences: {
        contextIsolation: true,
        sandbox: true,
        nodeIntegration: false,
        webSecurity: true,
        preload: options.preload,
        backgroundThrottling: true,
      },
    })
    const id = window.id
    const entry: Tracked = { window, project: null, focused: false, focusedAt: 0 }
    tracked.set(id, entry)
    const contents = window.webContents

    const navigated = (target: string): void => {
      const project = projectOfURL(target)
      if (project === entry.project) return
      entry.project = project
      projectsChanged()
    }
    contents.on("did-navigate", (_event, target) => navigated(target))
    contents.on("did-navigate-in-page", (_event, target, isMainFrame) => {
      if (isMainFrame) navigated(target)
    })
    contents.on("will-navigate", (event) => {
      if (!decide(event.url, true)) event.preventDefault()
    })
    contents.on("will-frame-navigate", (event) => {
      // will-navigate already handled the main frame.
      if (!event.isMainFrame && !decide(event.url, false)) event.preventDefault()
    })
    contents.on("will-redirect", (event) => {
      if (!decide(event.url, false)) event.preventDefault()
    })
    contents.setWindowOpenHandler(({ url: target }) => {
      // Same-origin opens go through window routing, never a new renderer.
      if (decide(target, true)) route(target)
      return { action: "deny" }
    })
    contents.on("render-process-gone", (_event, details) => {
      console.error(`servef: window ${id} renderer gone: ${details.reason} (exit ${details.exitCode})`)
    })

    window.on("focus", () => {
      entry.focused = true
      entry.focusedAt = Date.now()
    })
    window.on("blur", () => {
      entry.focused = false
    })
    window.on("close", () => {
      bounds = window.getNormalBounds()
      try {
        writeFileSync(options.stateFile, serializeWindowState(bounds))
      } catch (error) {
        console.error("servef: save window state:", error)
      }
    })
    window.on("closed", () => {
      tracked.delete(id)
      if (entry.project !== null) projectsChanged()
    })

    load(window, url)
  }

  const route = (target: string): void => {
    const decision = routeWindow(infos(), target)
    if (decision.kind === "new") {
      newWindow(decision.url)
      return
    }
    const entry = tracked.get(decision.id)
    if (entry === undefined) {
      newWindow(target)
      return
    }
    if (decision.kind === "load") load(entry.window, decision.url)
    reveal(entry.window)
  }

  const infos = (): WindowInfo[] =>
    [...tracked.entries()].map(([id, entry]) => ({
      id,
      project: entry.project,
      focused: entry.focused,
      focusedAt: entry.focusedAt,
    }))

  const focused = (): Tracked | undefined => [...tracked.values()].find((entry) => entry.focused)

  return {
    infos,
    route,
    newWindow,
    focusedWindow: () => focused()?.window ?? null,
    focusedProject: () => focused()?.project ?? null,
    shownProjects: () => [...new Set([...tracked.values()].flatMap((entry) => (entry.project === null ? [] : [entry.project])))],
    onProjectsChanged(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
  }
}

function load(window: BrowserWindow, url: string): void {
  // loadURL rejects when a later navigation aborts this one; nothing to do then.
  window.loadURL(url).catch((error: unknown) => console.error(`servef: load ${url}:`, error))
}

function reveal(window: BrowserWindow): void {
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

function readBounds(file: string): WindowBounds {
  try {
    return parseWindowState(readFileSync(file, "utf8"))
  } catch {
    return parseWindowState(null)
  }
}
