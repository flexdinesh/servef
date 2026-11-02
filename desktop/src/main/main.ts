// Main process composition (decisions 0067, 0070): lifecycle, routing, menu, and wiring.
import { app, dialog, ipcMain, Menu, shell, type MenuItemConstructorOptions, type MessageBoxOptions, type OpenDialogOptions } from "electron"
import { existsSync } from "node:fs"
import os from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { CONNECT_PATH, createAppPages } from "./app-pages.ts"
import { findOnPath, locateServef } from "./binary.ts"
import { APP_SCHEME, isLocalFrame, OPEN_FOLDER_CHANNEL } from "./bridge.ts"
import { cliId, cliPort, listCliServers, type CliList } from "./cli-servers.ts"
import { createFileStore } from "./connection-store.ts"
import { APP_HOST, connectionOrigin, createConnections, LOCAL_CONNECTION, type ConnectionLookup, type DaemonConnection } from "./connections.ts"
import { EXPECTED_PROTOCOL } from "./daemon-contract.ts"
import { devFlags, ensureDaemon, nodeExecFile, openPath, probeDaemon, stopDaemon, type EnsureResult } from "./daemon.ts"
import { confirmConnect, type ConnectChoice } from "./electron-dialogs.ts"
import { createNotifier } from "./electron-notify.ts"
import { handleAppScheme, registerAppScheme } from "./electron-protocol.ts"
import { createWindowManager, type WindowManager } from "./electron-windows.ts"
import { createForwarder } from "./forward.ts"
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
import { createProxy } from "./proxy.ts"
import { chooseRuntimeDir, prepareRuntimeDir, sweepRuntimeDir } from "./runtime-dir.ts"
import { routeSchemeURL, schemeArgument } from "./scheme.ts"
import { muxControlArgs } from "./ssh-args.ts"
import { listHostAliases, nodeConfigReader, resolveAliases } from "./ssh-config.ts"
import { createSshConnector, EXIT_TIMEOUT_MS, nodeSpawn, type SshTarget } from "./ssh-connection.ts"
import { resolveSshLink, sshTargetURL, type ResolvedAlias, type SshLink } from "./ssh-links.ts"
import { createSubscriptions, type SubscriptionEvent, type Subscriptions } from "./subscriptions.ts"
import { systemTimers } from "./timers.ts"
import { windowTitle } from "./titles.ts"
import { connectionOfOrigin, targetOfURL } from "./windows.ts"

const ORIGIN = `${APP_SCHEME}://${LOCAL_CONNECTION}`
const HOME = `${ORIGIN}/`
const CONNECT_URL = `${APP_SCHEME}://${APP_HOST}${CONNECT_PATH}`
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
    release: packaged ? app.getVersion() : null,
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

  const openFolder = async (): Promise<void> => {
    const parent = windows.focusedWindow()
    const options: OpenDialogOptions = { properties: ["openDirectory"] }
    const picked = parent === null ? await dialog.showOpenDialog(options) : await dialog.showOpenDialog(parent, options)
    const [folder] = picked.filePaths
    if (picked.canceled || folder === undefined) return
    const local = daemonConnection(await connections.lookup(LOCAL_CONNECTION))
    if (local === null) {
      dialog.showErrorBox("Could not open folder", "The servef daemon is not running.")
      return
    }
    const outcome = await openPath(forwarder.forward, local.socket, folder)
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
  // Only servef-app://local frames have the bridge (decision 0070).
  ipcMain.on(OPEN_FOLDER_CHANNEL, (event) => {
    if (isLocalFrame(event.senderFrame?.url ?? "")) runOpenFolder()
  })

  Menu.setApplicationMenu(Menu.buildFromTemplate(menuTemplate({
    newWindow: () => windows.newWindow(HOME),
    openFolder: runOpenFolder,
    connect: () => windows.route(CONNECT_URL),
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

  if (launch.length === 0) windows.route(HOME)
  for (const url of launch) openSchemeURL(url)
  return openSchemeURL
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

function menuTemplate(actions: { newWindow(): void; openFolder(): void; connect(): void }): MenuItemConstructorOptions[] {
  const mac = process.platform === "darwin"
  // No accelerators on keys the UI binds (P, F, Shift-F, C).
  const file: MenuItemConstructorOptions[] = [
    { label: "New Window", accelerator: "CmdOrCtrl+N", click: actions.newWindow },
    { label: "Open Folder…", accelerator: "CmdOrCtrl+O", click: actions.openFolder },
    { label: "Connect…", click: actions.connect },
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
