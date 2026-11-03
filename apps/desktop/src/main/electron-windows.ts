// BrowserWindow adapter: creation, security handlers, origin and project
// tracking, titles, title band colors, and routing (decisions 0067, 0070, 0075).
import { BrowserWindow, nativeTheme, session, shell } from "electron"
import { readFileSync, writeFileSync } from "node:fs"
import { navigationPolicy, permissionAllowed } from "./navigation.ts"
import { DEFAULT_CANVAS, overlayFor, parseThemeColor, windowChrome } from "./titlebar.ts"
import { parseWindowState, serializeWindowState, type WindowBounds } from "./window-state.ts"
import { connectionOfOrigin, routeWindow, targetOfURL, type WindowInfo } from "./windows.ts"

export interface WindowManagerOptions {
  // preload is the absolute path of build/preload.cjs.
  preload: string
  // stateFile is userData/window-state.json.
  stateFile: string
  // onScheme receives servef:// links the renderer navigates to.
  onScheme(url: string): void
  // onConnection receives servef-app:// links on another origin; main routes
  // them only for known connections.
  onConnection(url: string): void
  // title computes a window title from its origin and page title.
  title(origin: string, page: string): string
}

export interface WindowManager {
  infos(): WindowInfo[]
  // route applies routeWindow to a servef-app:// target.
  route(target: string): void
  newWindow(url: string): void
  focusedWindow(): BrowserWindow | null
  // focused is the focused window's connection id and project.
  focused(): { connection: string; project: string | null } | null
  // shownProjects lists the distinct projects windows of origin show.
  shownProjects(origin: string): string[]
  // shownOrigins lists the distinct origins with at least one window.
  shownOrigins(): string[]
  // onWindowsChanged fires when a window opens, closes, or changes project.
  onWindowsChanged(listener: () => void): () => void
  // retitle recomputes the titles of origin's windows from their last page titles.
  retitle(origin: string): void
}

interface Tracked {
  window: BrowserWindow
  // origin is the origin of the window's first URL, kept for life.
  origin: string
  project: string | null
  // page is the last page title; null before the first one.
  page: string | null
  focused: boolean
  focusedAt: number
}

export function createWindowManager(options: WindowManagerOptions): WindowManager {
  const tracked = new Map<number, Tracked>()
  const listeners = new Set<() => void>()
  let bounds = readBounds(options.stateFile)
  // bandColor is the last theme color any page reported; new windows start
  // with it so they open in the current theme's canvas.
  let bandColor: string | null = null

  const windowsChanged = (): void => {
    for (const listener of listeners) listener()
  }

  session.defaultSession.setPermissionRequestHandler((_contents, permission, callback) => {
    callback(permissionAllowed(permission))
  })
  session.defaultSession.setPermissionCheckHandler((_contents, permission) => permissionAllowed(permission))

  // decide runs the navigation policy; true when the navigation may proceed.
  // Only main-frame user navigations act on scheme and external links.
  const decide = (origin: string, url: string, act: boolean): boolean => {
    const decision = navigationPolicy(url, origin)
    if (decision.kind === "allow") return true
    if (!act) return false
    if (decision.kind === "scheme") options.onScheme(decision.url)
    if (decision.kind === "connection") options.onConnection(decision.url)
    if (decision.kind === "external") {
      shell.openExternal(decision.url).catch((error: unknown) => console.error("servef: open external:", error))
    }
    return false
  }

  const newWindow = (url: string): void => {
    const first = targetOfURL(url)
    if (first === null) {
      console.error(`servef: refused window for ${url}`)
      return
    }
    const origin = first.origin
    // A window beside open ones takes the size only; the OS places it.
    const placed: WindowBounds = tracked.size === 0 ? bounds : { width: bounds.width, height: bounds.height }
    const window = new BrowserWindow({
      ...placed,
      ...windowChrome(process.platform, bandColor ?? (nativeTheme.shouldUseDarkColors ? DEFAULT_CANVAS.dark : DEFAULT_CANVAS.light)),
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
    const entry: Tracked = { window, origin, project: null, page: null, focused: false, focusedAt: 0 }
    tracked.set(id, entry)
    const contents = window.webContents

    const navigated = (target: string): void => {
      // Navigation never leaves the window's origin; ignore anything else.
      const next = targetOfURL(target)
      if (next === null || next.origin !== origin || next.project === entry.project) return
      entry.project = next.project
      windowsChanged()
    }
    // The window's own event: preventDefault stops Electron's setTitle.
    window.on("page-title-updated", (event, page) => {
      event.preventDefault()
      entry.page = page
      window.setTitle(options.title(origin, page))
    })
    // The page's theme-color meta is its canvas (decision 0075); color the
    // window background and, outside macOS, the overlay controls with it.
    contents.on("did-change-theme-color", (_event, reported) => {
      const color = parseThemeColor(reported)
      if (color === null) return
      bandColor = color
      window.setBackgroundColor(color)
      if (process.platform !== "darwin") window.setTitleBarOverlay(overlayFor(color))
    })
    contents.on("did-navigate", (_event, target) => navigated(target))
    contents.on("did-navigate-in-page", (_event, target, isMainFrame) => {
      if (isMainFrame) navigated(target)
    })
    contents.on("will-navigate", (event) => {
      if (!decide(origin, event.url, true)) event.preventDefault()
    })
    contents.on("will-frame-navigate", (event) => {
      // will-navigate already handled the main frame.
      if (!event.isMainFrame && !decide(origin, event.url, false)) event.preventDefault()
    })
    contents.on("will-redirect", (event) => {
      if (!decide(origin, event.url, false)) event.preventDefault()
    })
    contents.setWindowOpenHandler(({ url: target }) => {
      // Same-origin opens go through window routing, never a new renderer.
      if (decide(origin, target, true)) route(target)
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
      windowsChanged()
    })

    windowsChanged()
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
      origin: entry.origin,
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
    focused: () => {
      const entry = focused()
      if (entry === undefined) return null
      const connection = connectionOfOrigin(entry.origin)
      return connection === null ? null : { connection, project: entry.project }
    },
    shownProjects: (origin) => [...new Set([...tracked.values()].flatMap((entry) => (entry.origin !== origin || entry.project === null ? [] : [entry.project])))],
    shownOrigins: () => [...new Set([...tracked.values()].map((entry) => entry.origin))],
    onWindowsChanged(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    retitle(origin) {
      for (const entry of tracked.values()) {
        if (entry.origin !== origin || entry.page === null || entry.window.isDestroyed()) continue
        entry.window.setTitle(options.title(origin, entry.page))
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
