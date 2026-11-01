// Main process composition (decision 0067): lifecycle, routing, menu, and wiring.
import { app, dialog, ipcMain, Menu, shell, type MenuItemConstructorOptions, type MessageBoxOptions, type OpenDialogOptions } from "electron"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { findOnPath, locateServef } from "./binary.ts"
import { APP_SCHEME, isAppFrame, OPEN_FOLDER_CHANNEL } from "./bridge.ts"
import { createConnections, LOCAL_CONNECTION, type ConnectionLookup } from "./connections.ts"
import { EXPECTED_PROTOCOL } from "./daemon-contract.ts"
import { devFlags, ensureDaemon, nodeExecFile, openPath, stopDaemon, type EnsureResult } from "./daemon.ts"
import { createNotifier } from "./electron-notify.ts"
import { handleAppScheme, registerAppScheme } from "./electron-protocol.ts"
import { createWindowManager, type WindowManager } from "./electron-windows.ts"
import { createForwarder } from "./forward.ts"
import {
  createNoticeGate,
  createSettleTracker,
  headNotice,
  NOTICE_INTERVAL_MS,
  settledNotice,
  SETTLE_QUIET_MS,
  walkthroughNotice,
  type Notice,
} from "./notifications.ts"
import { createProxy } from "./proxy.ts"
import { routeSchemeURL, schemeArgument } from "./scheme.ts"
import { createSubscriptions, type SubscriptionEvent } from "./subscriptions.ts"
import { systemTimers } from "./timers.ts"

const ORIGIN = `${APP_SCHEME}://${LOCAL_CONNECTION}`
const HOME = `${ORIGIN}/`
const DOCS_URL = "https://github.com/flexdinesh/servef"

process.on("unhandledRejection", (reason) => console.error("servef: unhandled rejection:", reason))

function main(): void {
  // The single-instance lock is per userData directory: set it first.
  const userData = process.env.SERVEF_DESKTOP_USER_DATA
  // The default <appData>/servef is the CLI's config directory (daemon roots.json).
  app.setPath("userData", userData !== undefined && userData !== "" ? path.resolve(userData) : path.join(app.getPath("appData"), "servef-desktop"))
  if (!app.requestSingleInstanceLock()) {
    app.quit()
    return
  }
  registerAppScheme()

  // macOS delivers servef:// links as open-url, possibly before ready.
  const early: string[] = []
  let deliver: ((url: string) => void) | null = null
  app.on("open-url", (event, url) => {
    event.preventDefault()
    if (deliver === null) early.push(url)
    else deliver(url)
  })

  app.whenReady().then(() => {
    const argument = schemeArgument(process.argv)
    const launch = early.length > 0 ? early.splice(0) : argument === null ? [] : [argument]
    deliver = start(launch)
  }).catch((error: unknown) => {
    console.error("servef: startup failed:", error)
    app.exit(1)
  })
}

// start composes the app once ready, opens the launch links (or the home
// window), and returns the servef:// link handler.
function start(launch: readonly string[]): (url: string) => void {
  const env = process.env
  const located = locateServef({
    packaged: app.isPackaged,
    resourcesPath: process.resourcesPath,
    env,
    onPath: (name) => findOnPath(name, env),
  })
  const ensure = (): Promise<EnsureResult> => {
    if ("path" in located) return ensureDaemon(located.path, devFlags(env), nodeExecFile)
    return Promise.resolve({ ok: false, kind: "missing", detail: located.missing })
  }
  const forwarder = createForwarder()
  const connections = createConnections(ensure)
  const lookup = (id: string): Promise<ConnectionLookup> => connections.lookup(id)
  const reensure = (id: string): Promise<boolean> => connections.reensure(id)
  const stopAndRetry = async (id: string): Promise<boolean> => {
    const current = await connections.lookup(id)
    if (current.kind !== "failed" || current.failure.status === undefined) return false
    await stopDaemon(forwarder.forward, current.failure.status.socket)
    return connections.reensure(id)
  }
  handleAppScheme(createProxy({ packaged: app.isPackaged, lookup, forward: forwarder.forward, reensure, stopAndRetry }))

  const windows = createWindowManager({
    origin: ORIGIN,
    preload: fileURLToPath(new URL("preload.cjs", import.meta.url)),
    stateFile: path.join(app.getPath("userData"), "window-state.json"),
    onScheme: (url) => openSchemeURL(url),
  })

  const notifier = createNotifier({
    logFile: env.SERVEF_DESKTOP_NOTICE_LOG,
    onClick: (notice) => windows.route(notice.target),
  })
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, systemTimers)
  const notify = (notice: Notice): void => {
    if (gate.allow(notice, windows.focusedProject())) notifier.show(notice)
  }
  const settle = createSettleTracker(
    SETTLE_QUIET_MS,
    (project, files, overflow) => notify(settledNotice(project, files, overflow)),
    systemTimers,
  )
  const onEvent = (event: SubscriptionEvent): void => {
    switch (event.kind) {
      case "head": {
        const notice = headNotice(event.project, event.head)
        if (notice !== null) notify(notice)
        break
      }
      case "walkthrough":
        notify(walkthroughNotice(event.announcement))
        break
      case "change":
        if (event.lease === "open") settle.change(event.project, event.change)
        break
      case "control":
        if (event.status.protocol !== EXPECTED_PROTOCOL) {
          console.error(`servef: daemon control protocol ${event.status.protocol}, app expects ${EXPECTED_PROTOCOL}`)
        }
        break
      case "status":
        break
    }
  }
  const subs = createSubscriptions({
    connection: async () => {
      const local = await connections.lookup(LOCAL_CONNECTION)
      return local.kind === "ready" ? local.connection : null
    },
    reensure: () => connections.reensure(LOCAL_CONNECTION),
    forward: forwarder.forward,
    timers: systemTimers,
    onEvent,
  })
  let shown = new Set<string>()
  windows.onProjectsChanged(() => {
    const next = windows.shownProjects()
    // A project no window shows any more leaves its open lease: drop its burst.
    for (const project of shown) if (!next.includes(project)) settle.forget(project)
    shown = new Set(next)
    subs.shown(next)
  })

  // About shows the app and, once known, the daemon release.
  const appVersion = app.getVersion()
  const about = (daemonVersion: string | null): void => {
    let version = appVersion
    if (daemonVersion !== null) {
      version += daemonVersion === appVersion ? ` (daemon ${daemonVersion})` : ` (daemon ${daemonVersion}, different release)`
    }
    app.setAboutPanelOptions({ applicationName: "servef", applicationVersion: version })
  }
  about(null)
  connections.subscribe((id, current) => {
    if (id !== LOCAL_CONNECTION) return
    if (current.kind === "ready") about(current.connection.status.version)
    else if (current.kind === "failed" && current.failure.status !== undefined) about(current.failure.status.version)
  })

  const openSchemeURL = (raw: string): void => {
    const routed = routeSchemeURL(raw)
    if ("target" in routed) {
      windows.route(routed.target)
      return
    }
    if (routed.reject === "remote") {
      void message(windows, "Remote links arrive in a later servef", raw)
    } else {
      console.error(`servef: ignored invalid link ${raw}`)
    }
    // Never leave the app running with no window.
    if (windows.infos().length === 0) windows.route(HOME)
  }

  const openFolder = async (): Promise<void> => {
    const parent = windows.focusedWindow()
    const options: OpenDialogOptions = { properties: ["openDirectory"] }
    const picked = parent === null ? await dialog.showOpenDialog(options) : await dialog.showOpenDialog(parent, options)
    const [folder] = picked.filePaths
    if (picked.canceled || folder === undefined) return
    const local = await connections.lookup(LOCAL_CONNECTION)
    if (local.kind !== "ready") {
      dialog.showErrorBox("Could not open folder", "The servef daemon is not running.")
      return
    }
    const outcome = await openPath(forwarder.forward, local.connection.socket, folder)
    if (!outcome.ok) {
      dialog.showErrorBox("Could not open folder", outcome.error)
      return
    }
    windows.route(`${HOME}?project=${encodeURIComponent(outcome.result.project)}`)
  }
  const runOpenFolder = (): void => {
    openFolder().catch((error: unknown) => {
      console.error("servef: open folder:", error)
      dialog.showErrorBox("Could not open folder", String(error))
    })
  }
  ipcMain.on(OPEN_FOLDER_CHANNEL, (event) => {
    if (isAppFrame(event.senderFrame?.url ?? "")) runOpenFolder()
  })

  Menu.setApplicationMenu(Menu.buildFromTemplate(menuTemplate({
    newWindow: () => windows.newWindow(HOME),
    openFolder: runOpenFolder,
  })))

  app.on("second-instance", (_event, argv) => {
    const argument = schemeArgument(argv)
    // A launch without a link focuses the latest window.
    if (argument === null) windows.route(HOME)
    else openSchemeURL(argument)
  })
  app.on("window-all-closed", () => {
    if (process.platform !== "darwin") app.quit()
  })
  app.on("activate", () => {
    if (windows.infos().length === 0) windows.route(HOME)
  })
  let closed = false
  app.on("before-quit", () => {
    // The daemon outlives the app; only this process's streams close.
    if (closed) return
    closed = true
    settle.dispose()
    subs.close()
    forwarder.close()
  })

  if (launch.length === 0) windows.route(HOME)
  for (const url of launch) openSchemeURL(url)
  return openSchemeURL
}

function message(windows: WindowManager, text: string, detail: string): Promise<unknown> {
  const options: MessageBoxOptions = { type: "info", message: text, detail }
  const parent = windows.focusedWindow()
  return parent === null ? dialog.showMessageBox(options) : dialog.showMessageBox(parent, options)
}

function menuTemplate(actions: { newWindow(): void; openFolder(): void }): MenuItemConstructorOptions[] {
  const mac = process.platform === "darwin"
  // No accelerators on keys the UI binds (P, F, Shift-F, C).
  const file: MenuItemConstructorOptions[] = [
    { label: "New Window", accelerator: "CmdOrCtrl+N", click: actions.newWindow },
    { label: "Open Folder…", accelerator: "CmdOrCtrl+O", click: actions.openFolder },
    { type: "separator" },
    { label: "Close Window", accelerator: "CmdOrCtrl+W", role: "close" },
  ]
  if (!mac) file.push({ type: "separator" }, { role: "quit" })
  const template: MenuItemConstructorOptions[] = [
    { label: "File", submenu: file },
    { role: "editMenu" },
    {
      label: "View",
      submenu: [
        { role: "reload" },
        { role: "forceReload" },
        { role: "toggleDevTools" },
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "togglefullscreen" },
      ],
    },
    { role: "windowMenu" },
    {
      role: "help",
      submenu: [{
        label: "servef documentation",
        click: () => {
          shell.openExternal(DOCS_URL).catch((error: unknown) => console.error("servef: open docs:", error))
        },
      }],
    },
  ]
  if (mac) template.unshift({ role: "appMenu" })
  return template
}

main()
