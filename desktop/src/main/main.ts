// Main process composition (decisions 0067, 0070, 0072): lifecycle, routing, menu, and wiring.
import { app, BrowserWindow, dialog, ipcMain, Menu, shell, type MenuItemConstructorOptions, type MessageBoxOptions, type OpenDialogOptions } from "electron"
import { existsSync } from "node:fs"
import os from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { CONNECT_PATH, createAppPages } from "./app-pages.ts"
import { findOnPath, locateServef } from "./binary.ts"
import { APP_MENU_CHANNEL, APP_SCHEME, isLocalFrame, OPEN_FOLDER_CHANNEL } from "./bridge.ts"
import { cliId, cliPort, listCliServers, type CliList } from "./cli-servers.ts"
import { createFileStore } from "./connection-store.ts"
import { APP_HOST, connectionOrigin, createConnections, LOCAL_CONNECTION, type ConnectionLookup, type DaemonConnection } from "./connections.ts"
import { EXPECTED_PROTOCOL } from "./daemon-contract.ts"
import { devFlags, ensureDaemon, nodeExecFile, openPath, probeDaemon, stopDaemon, type EnsureResult } from "./daemon.ts"
import { confirmConnect, showReport, type ConnectChoice } from "./electron-dialogs.ts"
import { createLoginItem, showLoginApproval } from "./electron-login.ts"
import { createNotifier } from "./electron-notify.ts"
import { handleAppScheme, registerAppScheme } from "./electron-protocol.ts"
import { createWindowManager, type WindowManager } from "./electron-windows.ts"
import { createFolderOpener } from "./folder-opener.ts"
import { createForwarder } from "./forward.ts"
import { releaseOf } from "./instructions.ts"
import { NOT_A_SERVEF_LINK, parsePastedLocation } from "./location.ts"
import {
  createNoticeGate,
  createSettleTracker,
  headNotice,
  NOTICE_INTERVAL_MS,
  settledNotice,
  SETTLE_QUIET_MS,
  walkthroughNotice,
  type Notice,
  type NoticeSource,
} from "./notifications.ts"
import { openPathsOf, parseOpenPaths, type InstanceData } from "./open-paths.ts"
import { createProxy } from "./proxy.ts"
import { chooseRuntimeDir, prepareRuntimeDir, sweepRuntimeDir } from "./runtime-dir.ts"
import { routeSchemeURL, schemeArgument } from "./scheme.ts"
import { muxControlArgs } from "./ssh-args.ts"
import { listHostAliases, nodeConfigReader, resolveAliases } from "./ssh-config.ts"
import { createSshConnector, EXIT_TIMEOUT_MS, nodeSpawn, type SshTarget } from "./ssh-connection.ts"
import { resolveSshLink, sshTargetURL, type ResolvedAlias, type SshLink } from "./ssh-links.ts"
import { createSubscriptions, type SubscriptionEvent, type Subscriptions } from "./subscriptions.ts"
import { systemTimers } from "./timers.ts"
import { parseMenuPoint } from "./titlebar.ts"
import { windowTitle } from "./titles.ts"
import { connectionOfOrigin, targetOfURL } from "./windows.ts"

const ORIGIN = `${APP_SCHEME}://${LOCAL_CONNECTION}`
const HOME = `${ORIGIN}/`
const CONNECT_URL = `${APP_SCHEME}://${APP_HOST}${CONNECT_PATH}`
const DOCS_URL = "https://github.com/flexdinesh/servef"

process.on("unhandledRejection", (reason) => console.error("servef: unhandled rejection:", reason))

// DROP_COLLECT_MS collects open-file events from one drop.
const DROP_COLLECT_MS = 100

// LaunchItem is a servef:// link or folders the OS handed the app.
type LaunchItem = { kind: "link"; url: string } | { kind: "paths"; paths: string[] }

function main(): void {
  // The single-instance lock is per userData directory: set it first.
  const userData = process.env.SERVEF_DESKTOP_USER_DATA
  // The default <appData>/servef is the CLI's config directory (daemon roots.json).
  app.setPath("userData", userData !== undefined && userData !== "" ? path.resolve(userData) : path.join(app.getPath("appData"), "servef-desktop"))
  // A second instance hands its own parsed folders over; its argv may arrive reordered.
  const openPaths = parseOpenPaths(process.argv, process.cwd())
  if (!app.requestSingleInstanceLock({ openPaths } satisfies InstanceData)) {
    app.quit()
    return
  }
  registerAppScheme()

  // macOS delivers servef:// links as open-url and folders (Dock or app icon
  // drops, `open -a servef <dir>`) as open-file, possibly before ready.
  const early: LaunchItem[] = []
  let deliver: ((item: LaunchItem) => void) | null = null
  // Folders dropped together arrive as one open-file each: once running,
  // collect them briefly into one request so the 8-folder bound applies.
  let dropped: string[] = []
  const flushDropped = (): void => {
    const paths = dropped
    dropped = []
    if (deliver !== null) deliver({ kind: "paths", paths })
  }
  const receive = (item: LaunchItem): void => {
    if (deliver === null) early.push(item)
    else if (item.kind === "link") deliver(item)
    else {
      if (dropped.length === 0) setTimeout(flushDropped, DROP_COLLECT_MS)
      dropped.push(...item.paths)
    }
  }
  app.on("open-url", (event, url) => {
    event.preventDefault()
    receive({ kind: "link", url })
  })
  app.on("open-file", (event, file) => {
    event.preventDefault()
    receive({ kind: "paths", paths: [file] })
  })

  app.whenReady().then(() => {
    const launch = early.splice(0)
    const argument = schemeArgument(process.argv)
    if (argument !== null && !launch.some((item) => item.kind === "link")) launch.push({ kind: "link", url: argument })
    if (openPaths.length > 0) launch.push({ kind: "paths", paths: openPaths })
    deliver = start(launch)
  }).catch((error: unknown) => {
    console.error("servef: startup failed:", error)
    app.exit(1)
  })
}

// start composes the app once ready, opens the launch links and folders (or
// the home window), and returns the handler for later ones.
function start(launch: readonly LaunchItem[]): (item: LaunchItem) => void {
  const env = process.env
  const packaged = app.isPackaged
  const userData = app.getPath("userData")
  const located = locateServef({
    packaged,
    resourcesPath: process.resourcesPath,
    env,
    onPath: (name) => findOnPath(name, env),
  })
  const ensure = (): Promise<EnsureResult> => {
    if ("path" in located) return ensureDaemon(located.path, devFlags(env), nodeExecFile)
    return Promise.resolve({ ok: false, kind: "missing", detail: located.missing })
  }
  const listCli = (): Promise<CliList> => {
    if ("path" in located) return listCliServers(located.path, nodeExecFile)
    return Promise.resolve({ ok: false, detail: located.missing })
  }
  const forwarder = createForwarder()

  // ssh (decision 0070). Dev seams are honored only in unpackaged builds.
  const ssh = (packaged ? null : nonEmpty(env.SERVEF_DESKTOP_SSH)) ?? findOnPath("ssh", env) ?? "/usr/bin/ssh"
  const config = packaged ? null : nonEmpty(env.SERVEF_DESKTOP_SSH_CONFIG)
  const sshEnv: NodeJS.ProcessEnv = { ...env, SSH_ASKPASS_REQUIRE: "never" }
  const runtimeEnv: NodeJS.ProcessEnv = { ...env }
  if (packaged) delete runtimeEnv.SERVEF_DESKTOP_RUNTIME_DIR
  const runtime = prepareRuntimeDir(chooseRuntimeDir({
    platform: process.platform,
    env: runtimeEnv,
    tmpdir: os.tmpdir(),
    userData,
    uid: process.getuid?.() ?? 0,
  }))
  if (!runtime.ok) console.error(`servef: runtime directory ${runtime.dir}: ${runtime.detail}`)
  // Single instance: every entry left in the runtime dir is stale. ssh
  // connections wait for the sweep (see gated) so it never touches live files.
  const swept: Promise<void> = runtime.ok
    ? sweepRuntimeDir(runtime.runtime, async (controlPath) => {
      await nodeExecFile(ssh, muxControlArgs({ id: "", destination: "servef-sweep", port: null }, controlPath, { op: "exit" }), { timeoutMs: EXIT_TIMEOUT_MS, env: sshEnv })
    }).catch((error: unknown) => console.error("servef: runtime sweep:", error))
    : Promise.resolve()
  const connector = createSshConnector({
    ssh,
    config,
    runtime,
    env: sshEnv,
    spawn: nodeSpawn,
    run: nodeExecFile,
    probe: (socket) => probeDaemon(forwarder.forward, socket),
    timers: systemTimers,
    exists: existsSync,
  })
  const connections = createConnections({
    ensureLocal: ensure,
    stopLocal: (socket) => stopDaemon(forwarder.forward, socket),
    ssh: connector,
    store: createFileStore(path.join(userData, "connections.json")),
    timers: systemTimers,
    listCli,
  })
  const isKnown = (id: string): boolean => connections.known().some((entry) => entry.id === id)
  // listAliases reads Host aliases from the ssh config on each call.
  const listAliases = (): string[] => {
    try {
      return listHostAliases(config ?? path.join(os.homedir(), ".ssh", "config"), nodeConfigReader(os.homedir()))
    } catch (error) {
      console.error("servef: ssh config:", error)
      return []
    }
  }

  // Only ssh connections wait for the startup sweep; local and CLI servers never do.
  const gated = async <T>(id: string, run: () => Promise<T>): Promise<T> => {
    if (id !== LOCAL_CONNECTION && cliPort(id) === null) await swept
    return run()
  }
  const lookup = (id: string): Promise<ConnectionLookup> => gated(id, () => connections.lookup(id))
  const reensure = (id: string): Promise<boolean> => gated(id, () => connections.reensure(id))
  const stopAndRetry = (id: string): Promise<boolean> => gated(id, () => connections.stopAndRetry(id))
  const appPages = createAppPages({
    model: async () => {
      const destinations = new Set(connections.known().map((entry) => entry.destination.toLowerCase()))
      const listed = await connections.refreshCli()
      return {
        connections: connections.summaries(),
        aliases: listAliases().filter((alias) => !destinations.has(alias.toLowerCase())),
        serversDetail: listed.ok ? "" : listed.detail,
      }
    },
    forget: (id) => connections.forget(id),
    open: (raw) => openPasted(raw),
  })
  handleAppScheme(createProxy({
    packaged,
    release: releaseOf(app.getVersion(), packaged),
    lookup,
    forward: forwarder.forward,
    reensure,
    stopAndRetry,
    appPages,
    dropCli: (id) => connections.dropCli(id),
  }))

  const title = (origin: string, page: string): string => {
    const id = connectionOfOrigin(origin)
    if (id === null) return page
    if (id === APP_HOST) return windowTitle(page, { kind: "app", label: "", ready: true })
    const summary = connections.describe(id)
    if (summary === null) return page
    if (summary.kind === "daemon") return windowTitle(page, { kind: "local", label: summary.label, ready: summary.state === "ready" })
    if (summary.kind === "cli") return windowTitle(page, { kind: "cli", label: summary.label, ready: true })
    return windowTitle(page, { kind: "ssh", label: summary.label, ready: summary.state === "ready" })
  }

  // openConnectionURL routes a servef-app:// link on another origin: local,
  // app pages, known ssh hosts, and running CLI servers only.
  const openConnectionURL = (url: string): void => {
    const target = targetOfURL(url)
    const id = target === null ? null : connectionOfOrigin(target.origin)
    if (id !== null && (id === LOCAL_CONNECTION || id === APP_HOST || isKnown(id))) {
      windows.route(url)
      return
    }
    if (id !== null && cliPort(id) !== null) {
      connections.lookup(id).then((current) => {
        if (current.kind === "ready") windows.route(url)
        else console.error(`servef: denied navigation to a stopped servef server ${url}`)
      }).catch((error: unknown) => console.error(`servef: ${url}:`, error))
      return
    }
    console.error(`servef: denied navigation to unknown connection ${url}`)
  }

  const windows = createWindowManager({
    preload: fileURLToPath(new URL("preload.cjs", import.meta.url)),
    stateFile: path.join(userData, "window-state.json"),
    onScheme: (url) => openSchemeURL(url),
    onConnection: (url) => openConnectionURL(url),
    title,
  })

  const notifier = createNotifier({
    logFile: env.SERVEF_DESKTOP_NOTICE_LOG,
    onClick: (notice) => windows.route(notice.target),
  })
  const gate = createNoticeGate(NOTICE_INTERVAL_MS, systemTimers)
  const notify = (notice: Notice): void => {
    if (gate.allow(notice, windows.focused())) notifier.show(notice)
  }
  // sources maps a connection id to its notice source (label "" for local).
  const sources = new Map<string, NoticeSource>([[LOCAL_CONNECTION, { connection: LOCAL_CONNECTION, label: "" }]])
  const settle = createSettleTracker(
    SETTLE_QUIET_MS,
    (connection, project, files, overflow) => {
      const source = sources.get(connection)
      if (source !== undefined) notify(settledNotice(source, project, files, overflow))
    },
    systemTimers,
  )
  const onEvent = (source: NoticeSource, event: SubscriptionEvent): void => {
    switch (event.kind) {
      case "head": {
        const notice = headNotice(source, event.project, event.head)
        if (notice !== null) notify(notice)
        break
      }
      case "walkthrough":
        notify(walkthroughNotice(source, event.announcement))
        break
      case "change":
        if (event.lease === "open") settle.change(source.connection, event.project, event.change)
        break
      case "control":
        if (event.status.protocol !== EXPECTED_PROTOCOL) {
          console.error(`servef: ${source.connection} daemon control protocol ${event.status.protocol}, app expects ${EXPECTED_PROTOCOL}`)
        }
        break
      case "status":
        break
    }
  }

  // Feeds: one subscription set per daemon-like connection (local, each ready
  // ssh connection), fed the projects its origin's windows show.
  interface Feed {
    subs: Subscriptions
    shown: Set<string>
  }
  const feeds = new Map<string, Feed>()
  const feedShown = (id: string, feed: Feed): void => {
    const next = windows.shownProjects(connectionOrigin(id))
    // A project no window shows any more leaves its open lease: drop its burst.
    for (const project of feed.shown) if (!next.includes(project)) settle.forget(id, project)
    feed.shown = new Set(next)
    feed.subs.shown(next)
  }
  const closeFeed = (id: string): void => {
    const feed = feeds.get(id)
    if (feed === undefined) return
    feeds.delete(id)
    feed.subs.close()
    for (const project of feed.shown) settle.forget(id, project)
  }
  const localSource: NoticeSource = { connection: LOCAL_CONNECTION, label: "" }
  feeds.set(LOCAL_CONNECTION, {
    subs: createSubscriptions({
      connection: async () => daemonConnection(await connections.lookup(LOCAL_CONNECTION)),
      reensure: () => connections.reensure(LOCAL_CONNECTION),
      forward: forwarder.forward,
      timers: systemTimers,
      onEvent: (event) => onEvent(localSource, event),
    }),
    shown: new Set(),
  })
  const openSshFeed = (id: string, label: string): void => {
    const source: NoticeSource = { connection: id, label }
    sources.set(id, source)
    const feed: Feed = {
      subs: createSubscriptions({
        // Streams never start a connection: windows hold it.
        connection: async () => {
          const current = connections.peek(id)
          return current === null ? null : daemonConnection(current)
        },
        // Re-ensure only a ready session whose forward refused; a reconnecting
        // or failed session keeps its own backoff and waits for Retry.
        reensure: async () => connections.peek(id)?.kind === "ready" ? connections.reensure(id) : false,
        forward: forwarder.forward,
        timers: systemTimers,
        onEvent: (event) => onEvent(source, event),
      }),
      shown: new Set(),
    }
    feeds.set(id, feed)
    feedShown(id, feed)
  }

  // Holds: an ssh connection is held while any window shows its origin.
  const held = new Set<string>()
  const syncHolds = (): void => {
    const shown = new Set<string>()
    for (const origin of windows.shownOrigins()) {
      const id = connectionOfOrigin(origin)
      if (id !== null && connections.describe(id)?.kind === "ssh") shown.add(id)
    }
    for (const id of shown) {
      if (held.has(id)) continue
      held.add(id)
      connections.hold(id)
    }
    for (const id of held) {
      if (shown.has(id)) continue
      held.delete(id)
      connections.release(id)
    }
  }
  windows.onWindowsChanged(() => {
    syncHolds()
    for (const [id, feed] of feeds) feedShown(id, feed)
  })

  // About shows the app and, once known, the local daemon release.
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
    windows.retitle(connectionOrigin(id))
    if (id === LOCAL_CONNECTION) {
      if (current?.kind === "ready" && current.connection.kind !== "cli") about(current.connection.status.version)
      else if (current?.kind === "failed" && current.connection === "daemon" && current.failure.status !== undefined) about(current.failure.status.version)
      return
    }
    if (current === null) {
      closeFeed(id)
      sources.delete(id)
      return
    }
    // CLI servers have no control stream (decision 0071): no feed, no notices.
    if (current.kind === "ready" && current.connection.kind === "ssh" && !feeds.has(id)) openSshFeed(id, current.connection.label)
  })

  // ensureWindow never leaves the app running with no window.
  const ensureWindow = (): void => {
    if (windows.infos().length === 0) windows.route(HOME)
  }

  // openSshLink resolves a servef://ssh/ link: known hosts open at once; first
  // contact is confirmed and remembered.
  const openSshLink = async (link: SshLink): Promise<void> => {
    const known = connections.known()
    let resolution = resolveSshLink(link, known, [])
    let aliases: ResolvedAlias[] = []
    if (resolution.kind !== "known") {
      aliases = await resolveAliases(listAliases(), ssh, config, nodeExecFile)
      resolution = resolveSshLink(link, connections.known(), aliases)
    }
    if (resolution.kind === "known") {
      windows.route(sshTargetURL(resolution.id, link))
      return
    }
    const targets = resolution.candidates.length > 0 ? resolution.candidates : resolution.fallbacks
    if (targets.length === 0) {
      await message(windows, "error", `No SSH host for ${link.host}`,
        `Add a Host entry for ${link.host} to your SSH config, or run servef remote --alias <alias> on the host so its links name an alias you can reach.`)
      return
    }
    const choices = targets.slice(0, 4)
    const prompt = { host: link.host, choices: choices.map((target) => choiceOf(target, aliases)) }
    const chosen = await confirmConnect(windows.focusedWindow(), prompt, { packaged, env })
    const target = chosen === null ? undefined : choices[chosen]
    if (target === undefined) return
    const entry = connections.remember({ destination: target.destination, port: target.port }, link.hints)
    windows.route(sshTargetURL(entry.id, link))
  }

  // openPasted opens a link pasted on the Connect page (decision 0071):
  // servef:// links as the OS handler would, and localhost URLs of running
  // CLI servers in a servef-app://cli-<port> window.
  const openPasted = async (raw: string): Promise<{ ok: true } | { ok: false; reason: string }> => {
    const location = parsePastedLocation(raw)
    switch (location.kind) {
      case "invalid":
        return { ok: false, reason: location.reason }
      case "scheme":
        if (routeSchemeURL(location.url).kind === "invalid") return { ok: false, reason: NOT_A_SERVEF_LINK }
        openSchemeURL(location.url)
        return { ok: true }
      case "cli": {
        const listed = await connections.refreshCli()
        if (!listed.ok) return { ok: false, reason: `Could not list servef servers: ${listed.detail}` }
        // peek: the list just ran; a lookup would list again.
        const current = connections.peek(cliId(location.port))
        if (current?.kind !== "ready") return { ok: false, reason: `No servef server is running on port ${location.port}.` }
        windows.route(location.target)
        return { ok: true }
      }
    }
  }

  const openSchemeURL = (raw: string): void => {
    const routed = routeSchemeURL(raw)
    switch (routed.kind) {
      case "local":
        windows.route(routed.target)
        return
      case "ssh":
        openSshLink(routed.link)
          .catch((error: unknown) => console.error(`servef: ssh link ${raw}:`, error))
          .finally(ensureWindow)
        return
      case "invalid":
        console.error(`servef: ignored invalid link ${raw}`)
        ensureWindow()
    }
  }

  const report = (text: string, detail: string): void => {
    showReport(windows.focusedWindow(), { message: text, detail }, { packaged, env })
      .catch((error: unknown) => console.error("servef: report:", error))
  }
  // folders opens folders from the dialog, the OS, and second instances (decision 0072).
  const folders = createFolderOpener({
    socket: async () => daemonConnection(await connections.lookup(LOCAL_CONNECTION))?.socket ?? null,
    open: (socket, folder) => openPath(forwarder.forward, socket, folder),
    route: (url) => windows.route(url),
    windowCount: () => windows.infos().length,
    origin: ORIGIN,
    report,
  })
  const openFolders = (paths: readonly string[]): void => {
    folders.open(paths).catch((error: unknown) => {
      console.error("servef: open folders:", error)
      report("Could not open folder", String(error))
      ensureWindow()
    })
  }
  const openFolder = async (): Promise<void> => {
    const parent = windows.focusedWindow()
    const options: OpenDialogOptions = { properties: ["openDirectory"] }
    const picked = parent === null ? await dialog.showOpenDialog(options) : await dialog.showOpenDialog(parent, options)
    const [folder] = picked.filePaths
    if (picked.canceled || folder === undefined) return
    openFolders([folder])
  }
  const runOpenFolder = (): void => {
    openFolder().catch((error: unknown) => {
      console.error("servef: open folder:", error)
      report("Could not open folder", String(error))
    })
  }
  // Only servef-app://local frames have the bridge (decision 0070).
  ipcMain.on(OPEN_FOLDER_CHANNEL, (event) => {
    if (isLocalFrame(event.senderFrame?.url ?? "")) runOpenFolder()
  })
  // The title band's menu button (decision 0075): pop the application menu
  // under it, at a point validated against the sender's window.
  ipcMain.on(APP_MENU_CHANNEL, (event, point: unknown) => {
    if (!isLocalFrame(event.senderFrame?.url ?? "")) return
    const window = BrowserWindow.fromWebContents(event.sender)
    if (window === null) return
    const [width, height] = window.getContentSize()
    const at = parseMenuPoint(point, { width: width ?? 0, height: height ?? 0 })
    if (at !== null) Menu.getApplicationMenu()?.popup({ window, ...at })
  })

  // Open at Login (decision 0072): the checkbox shows the OS state; only a click changes it.
  const loginItem = createLoginItem({ platform: process.platform, packaged, env, home: os.homedir(), execPath: process.execPath, appPath: app.getAppPath() })
  const loginEnabled = (): boolean => {
    try {
      return loginItem?.enabled() ?? false
    } catch (error) {
      console.error("servef: open at login:", error)
      return false
    }
  }
  let loginShown = false
  const buildMenu = (): void => {
    loginShown = loginEnabled()
    Menu.setApplicationMenu(Menu.buildFromTemplate(menuTemplate({
      newWindow: () => windows.newWindow(HOME),
      openFolder: runOpenFolder,
      connect: () => windows.route(CONNECT_URL),
      login: loginItem === null ? null : { checked: loginShown, toggle: toggleLogin },
    })))
  }
  const toggleLogin = (): void => {
    if (loginItem === null) return
    const on = !loginEnabled()
    loginItem.set(on)
      .then((outcome) => outcome === "approval" ? showLoginApproval(windows.focusedWindow()) : undefined)
      .catch((error: unknown) => {
        console.error("servef: open at login:", error)
        report("Could not change Open at Login", `${error instanceof Error ? error.message : String(error)}\n\nChange it in ${loginItem.manual}.`)
      })
      .finally(buildMenu)
  }
  buildMenu()
  // Users may change it in OS settings: refresh on focus when it differs.
  app.on("browser-window-focus", () => {
    if (loginItem !== null && loginEnabled() !== loginShown) buildMenu()
  })

  app.on("second-instance", (_event, argv, _cwd, data) => {
    const paths = openPathsOf(data)
    if (paths.length > 0) {
      openFolders(paths)
      return
    }
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
    // The local daemon outlives the app; only this process's streams close.
    if (closed) return
    closed = true
    settle.dispose()
    for (const feed of feeds.values()) feed.subs.close()
    feeds.clear()
    forwarder.close()
  })
  // ssh masters and runtime files go with the app (synchronous on quit).
  app.on("will-quit", () => connections.closeSync())

  const deliver = (item: LaunchItem): void => {
    if (item.kind === "link") openSchemeURL(item.url)
    else openFolders(item.paths)
  }
  if (launch.length === 0) windows.route(HOME)
  // One request for all launch folders, so the 8-folder bound applies once.
  const launchPaths = launch.flatMap((item) => item.kind === "paths" ? item.paths : [])
  if (launchPaths.length > 0) openFolders([...new Set(launchPaths)])
  for (const item of launch) if (item.kind === "link") openSchemeURL(item.url)
  return deliver
}

// choiceOf describes a confirmation choice: the destination and port, plus
// the alias's `ssh -G` user, hostname, and port when it came from an alias.
function choiceOf(target: SshTarget, aliases: readonly ResolvedAlias[]): ConnectChoice {
  const label = target.port === null ? target.destination : `${target.destination} (port ${target.port})`
  const alias = aliases.find((entry) => entry.alias === target.destination)
  const detail = alias === undefined
    ? `${label}: ssh ${target.destination}${target.port === null ? "" : ` -p ${target.port}`}`
    : `${label}: user ${alias.user}, hostname ${alias.hostname}, port ${target.port ?? alias.port}`
  return { label, detail }
}

// daemonConnection is a ready daemon-like (local or ssh) connection; null otherwise.
function daemonConnection(lookup: ConnectionLookup): DaemonConnection | null {
  return lookup.kind === "ready" && lookup.connection.kind !== "cli" ? lookup.connection : null
}

function nonEmpty(value: string | undefined): string | null {
  return value === undefined || value === "" ? null : value
}

function message(windows: WindowManager, type: "info" | "error", text: string, detail: string): Promise<unknown> {
  const options: MessageBoxOptions = { type, message: text, detail }
  const parent = windows.focusedWindow()
  return parent === null ? dialog.showMessageBox(options) : dialog.showMessageBox(parent, options)
}

interface MenuActions {
  newWindow(): void
  openFolder(): void
  connect(): void
  // login is the Open at Login checkbox; null hides it.
  login: { checked: boolean; toggle(): void } | null
}

function menuTemplate(actions: MenuActions): MenuItemConstructorOptions[] {
  const mac = process.platform === "darwin"
  const login: MenuItemConstructorOptions[] = actions.login === null
    ? []
    : [{ id: "open-at-login", label: "Open at Login", type: "checkbox", checked: actions.login.checked, click: actions.login.toggle }]
  // No accelerators on keys the UI binds (P, F, Shift-F, C).
  const file: MenuItemConstructorOptions[] = [
    { label: "New Window", accelerator: "CmdOrCtrl+N", click: actions.newWindow },
    { label: "Open Folder…", accelerator: "CmdOrCtrl+O", click: actions.openFolder },
    { label: "Connect…", click: actions.connect },
    { type: "separator" },
    { label: "Close Window", accelerator: "CmdOrCtrl+W", role: "close" },
  ]
  if (!mac) {
    if (login.length > 0) file.push({ type: "separator" }, ...login)
    file.push({ type: "separator" }, { role: "quit" })
  }
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
  if (mac) {
    // The standard app menu, plus Open at Login.
    const appMenu: MenuItemConstructorOptions[] = [{ role: "about" }, { type: "separator" }]
    if (login.length > 0) appMenu.push(...login, { type: "separator" })
    appMenu.push(
      { role: "services" },
      { type: "separator" },
      { role: "hide" },
      { role: "hideOthers" },
      { role: "unhide" },
      { type: "separator" },
      { role: "quit" },
    )
    template.unshift({ label: app.name, submenu: appMenu })
  }
  return template
}

main()
