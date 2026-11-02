import { isIP } from "node:net"
import { cliId, cliPort, dialHost } from "./cli-servers.ts"
import type { CliList, CliServer } from "./cli-servers.ts"
import { assignId, assignServerId } from "./connection-store.ts"
import type { ConnectionStore, KnownConnection, KnownServer } from "./connection-store.ts"
import type { DaemonStatus } from "./daemon-contract.ts"
import type { EnsureFailure, EnsureResult } from "./daemon.ts"
import type { SshConnector, SshReady, SshSession, SshState } from "./ssh-connection.ts"
import type { SshFailure } from "./ssh-errors.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export const LOCAL_CONNECTION = "local"
// APP_HOST is the app pages host (servef-app://app/connect); no upstream.
export const APP_HOST = "app"
// RELEASE_GRACE_MS: an ssh connection no window holds closes after this.
export const RELEASE_GRACE_MS = 30_000

export type ConnectionKind = "daemon" | "ssh" | "cli" | "http" | "ssh-port"

// DaemonConnection is a daemon reached over a Unix socket (local or ssh).
export interface DaemonConnection {
  id: string
  kind: "daemon" | "ssh"
  // origin is the renderer origin, servef-app://<id>.
  origin: string
  // socket is the daemon socket (local) or the local forward socket (ssh).
  socket: string
  status: DaemonStatus
  // label is "local" or the ssh destination.
  label: string
}

// CliConnection is a running CLI server (servef browser) on loopback TCP
// (decision 0071).
export interface CliConnection {
  id: string
  kind: "cli"
  origin: string
  // host is the loopback host to dial; port its port.
  host: string
  port: number
  // label is "localhost:<port>".
  label: string
}

// HttpConnection is a servef server on another computer over TCP (decision
// 0075), confirmed or chosen by the user.
export interface HttpConnection {
  id: string
  kind: "http"
  origin: string
  // host is the dial host (IPv6 without brackets); urlHost the URL hostname.
  host: string
  urlHost: string
  port: number
  // label is "<urlHost>:<port>".
  label: string
}

// PortConnection is a remote TCP server reached through an ssh host's
// master (decision 0075): socket is the local p-*.sock forward.
export interface PortConnection {
  id: string
  kind: "ssh-port"
  origin: string
  socket: string
  port: number
  // label is "<ssh destination>:<port>".
  label: string
}

export type Connection = DaemonConnection | CliConnection | HttpConnection | PortConnection

// upstreamHost is the Host header the proxy sends: localhost for daemons;
// localhost:<port> for CLI servers and forwarded remote servers; for http
// servers <host>:<port> when host is an IP literal, else localhost:<port>
// (servef's Host check refuses other names, and the app is no browser that
// DNS rebinding could steer).
export function upstreamHost(connection: Connection): string {
  switch (connection.kind) {
    case "daemon":
    case "ssh":
      return "localhost"
    case "cli":
    case "ssh-port":
      return `localhost:${connection.port}`
    case "http":
      return isIP(connection.host) === 0 ? `localhost:${connection.port}` : `${connection.urlHost}:${connection.port}`
  }
}

export type ConnectionLookup =
  | { kind: "ready"; connection: Connection }
  // connecting: an ssh connection is reconnecting after a transient failure.
  | { kind: "connecting"; label: string; failure: SshFailure }
  | { kind: "failed"; connection: "daemon"; failure: EnsureFailure; label: string }
  | { kind: "failed"; connection: "ssh"; failure: SshFailure; label: string }
  | { kind: "unknown" }

export type ConnectionState = "idle" | "connecting" | "ready" | "reconnecting" | "failed"

// ConnectionSummary feeds titles and the Connect page.
export interface ConnectionSummary {
  id: string
  kind: ConnectionKind
  label: string
  state: ConnectionState
  // detail is the failure detail when reconnecting or failed, else "".
  detail: string
}

export interface Connections {
  // lookup: local ensures on demand (as M3); a known ssh id awaits an
  // in-flight connect or starts one; a known http id is ready (nothing is
  // dialed); a registered ssh-port id looks its host up and forwards the
  // port; unknown ids (and APP_HOST) are unknown.
  lookup(id: string): Promise<ConnectionLookup>
  // peek returns the current lookup without starting anything; null when
  // there is no result or session yet.
  peek(id: string): ConnectionLookup | null
  // reensure: local runs ensure again; ssh calls session.reensure() (failed
  // or reconnecting → new attempt now); false for unknown ids or ssh ids
  // without a session. Single flight per id. True when ready after.
  reensure(id: string): Promise<boolean>
  // stopAndRetry stops a mismatched daemon (local stopLocal, ssh REMOTE_STOP)
  // and re-ensures.
  stopAndRetry(id: string): Promise<boolean>
  // hold and release count windows showing an origin; an ssh session with no
  // holds closes RELEASE_GRACE_MS after its last release (or its start when
  // never held). Holding a known idle id does not connect by itself.
  hold(id: string): void
  release(id: string): void
  known(): KnownConnection[]
  // remember stores a destination the user chose and returns its entry; an
  // existing destination+port reuses its entry.
  remember(target: { destination: string; port: number | null }): KnownConnection
  // servers lists the known http servers in store order.
  servers(): KnownServer[]
  // findServer is the known http server at host (URL hostname) and port.
  findServer(host: string, port: number): KnownServer | null
  // rememberServer stores an http server (confirmed or chosen) and returns
  // its entry; an existing host+port reuses its entry.
  rememberServer(host: string, port: number): KnownServer
  // registerPort names the remote TCP server host:port of ssh host sshId as
  // ssh-port id <sshId>-<port> (in memory only); an id a known ssh host has
  // is refused.
  registerPort(sshId: string, host: string, port: number): { ok: true; id: string } | { ok: false; detail: string }
  // dropPort cancels an ssh-port forward and unregisters the id.
  dropPort(id: string): Promise<void>
  // listServers lists the running servef servers on ssh host id through its
  // session (none: not open).
  listServers(id: string): Promise<CliList>
  // forget closes an ssh session (no grace) and removes the entry and its
  // ssh-port ids, or removes an http server.
  forget(id: string): Promise<void>
  // describe summarizes local, known ssh and http, registered ssh-port, and
  // registered cli ids; null otherwise.
  describe(id: string): ConnectionSummary | null
  // summaries lists local first, then known ssh hosts in store order, known
  // http servers in store order, then registered CLI servers by port.
  summaries(): ConnectionSummary[]
  // refreshCli lists the running CLI servers and registers exactly those
  // (single flight); a failed list keeps the registrations.
  refreshCli(): Promise<{ ok: true } | { ok: false; detail: string }>
  // dropCli unregisters a CLI server that stopped answering; the next lookup
  // lists again.
  dropCli(id: string): void
  // Listeners hear each lookup change; null when an ssh session closed.
  subscribe(listener: (id: string, lookup: ConnectionLookup | null) => void): () => void
  close(): Promise<void>
  closeSync(): void
}

export interface ConnectionsDeps {
  ensureLocal(): Promise<EnsureResult>
  // stopLocal stops a mismatched local daemon at socket; true once gone.
  stopLocal(socket: string): Promise<boolean>
  ssh: SshConnector
  store: ConnectionStore
  timers: Timers
  // listCli runs `servef control list --json` (decision 0071).
  listCli(): Promise<CliList>
}

// connectionOrigin is the renderer origin of connection id.
export function connectionOrigin(id: string): string {
  return `servef-app://${id}`
}

// createConnections composes the local daemon (ensured on demand, as M3) and
// the known ssh hosts (decision 0070). Local listeners hear each ensure whose
// outcome differs from the last; ssh listeners hear every session state change
// that maps to a lookup (ready, reconnecting, failed, and connecting after an
// earlier failure), and null when a session closes or its host is forgotten.
// CLI servers register on demand: a lookup of an unregistered cli-<port>
// lists the running servers once; listeners hear registrations and drops.
export function createConnections(deps: ConnectionsDeps): Connections {
  const listeners = new Set<(id: string, lookup: ConnectionLookup | null) => void>()
  const emit = (id: string, lookup: ConnectionLookup | null): void => {
    for (const listener of listeners) listener(id, lookup)
  }

  // Local daemon.
  let local: ConnectionLookup | null = null
  let localInflight: Promise<ConnectionLookup> | null = null
  const ensureLocal = (): Promise<ConnectionLookup> => {
    if (localInflight !== null) return localInflight
    const started = deps.ensureLocal().then(localLookup, (error: unknown): ConnectionLookup => ({
      kind: "failed",
      connection: "daemon",
      failure: { ok: false, kind: "failed", detail: errorMessage(error) },
      label: LOCAL_CONNECTION,
    })).then((lookup) => {
      const changed = local === null || !sameLocal(local, lookup)
      local = lookup
      localInflight = null
      if (changed) emit(LOCAL_CONNECTION, lookup)
      return lookup
    })
    localInflight = started
    return started
  }

  // Known ssh hosts and their sessions.
  const stored = deps.store.load()
  let known: KnownConnection[] = stored.ssh
  let servers: KnownServer[] = stored.http
  // ports maps registered ssh-port ids to their host and remote address.
  const ports = new Map<string, { sshId: string; host: string; port: number }>()
  const sessions = new Map<string, Live>()
  // closing holds sessions still tearing down; a new session for the same id
  // waits so two masters never share a ControlPath.
  const closing = new Map<string, Promise<void>>()
  const holds = new Map<string, number>()
  const graceTimers = new Map<string, TimerHandle>()

  const entry = (id: string): KnownConnection | undefined => known.find((candidate) => candidate.id === id)

  const clearGrace = (id: string): void => {
    const timer = graceTimers.get(id)
    if (timer === undefined) return
    deps.timers.clearTimeout(timer)
    graceTimers.delete(id)
  }

  const scheduleGrace = (id: string): void => {
    clearGrace(id)
    if (!sessions.has(id) || (holds.get(id) ?? 0) > 0) return
    graceTimers.set(id, deps.timers.setTimeout(() => {
      graceTimers.delete(id)
      void closeSession(id, true)
    }, RELEASE_GRACE_MS))
  }

  // closeSession drops the session at once (later lookups reconnect after the
  // teardown) and resolves once it closed.
  const closeSession = async (id: string, notify: boolean): Promise<void> => {
    clearGrace(id)
    const live = sessions.get(id)
    if (live === undefined) {
      await closing.get(id)
      return
    }
    sessions.delete(id)
    live.unsubscribe()
    const done = live.session.close().catch(() => {}).finally(() => {
      if (closing.get(id) === done) closing.delete(id)
    })
    closing.set(id, done)
    await done
    if (notify) emit(id, null)
  }

  const startSession = (target: KnownConnection): Live => {
    const session = deps.ssh.connect({ id: target.id, destination: target.destination, port: target.port })
    const live: Live = { session, label: target.destination, failure: null, unsubscribe: () => {} }
    live.unsubscribe = session.onState((state) => {
      if (sessions.get(target.id) !== live) return
      if (state.kind === "reconnecting" || state.kind === "failed") live.failure = state.failure
      const mapped = sshLookup(target.id, live, state)
      if (mapped !== null) emit(target.id, mapped)
    })
    sessions.set(target.id, live)
    scheduleGrace(target.id)
    return live
  }

  const lookupSsh = async (id: string): Promise<ConnectionLookup> => {
    const pending = closing.get(id)
    if (pending !== undefined && !sessions.has(id)) await pending
    let live = sessions.get(id)
    if (live === undefined) {
      const current = entry(id)
      if (current === undefined) return { kind: "unknown" }
      live = startSession(current)
    }
    const state = live.session.state()
    if (state.kind !== "connecting") return sshLookup(id, live, state) ?? { kind: "unknown" }
    return settled(id, live, await live.session.ready())
  }

  const save = (): void => deps.store.save({ ssh: known, http: servers })
  const server = (id: string): KnownServer | undefined => servers.find((candidate) => candidate.id === id)

  const portFailure = (destination: string, label: string, detail: string): ConnectionLookup => ({
    kind: "failed", connection: "ssh", label, failure: { kind: "failed", destination, detail },
  })
  const lookupPort = async (id: string, registered: { sshId: string; host: string; port: number }): Promise<ConnectionLookup> => {
    const parent = await lookupSsh(registered.sshId)
    const live = sessions.get(registered.sshId)
    if (live === undefined) return parent
    const usable = parent.kind === "ready" || (parent.kind === "failed" && parent.connection === "ssh" && parent.failure.kind === "forwardDenied")
    if (!usable) return parent
    const label = portLabel(live.label, registered.port)
    const forwarded = await live.session.forwardPort(id, registered.host, registered.port)
    if (ports.get(id) !== registered) return { kind: "unknown" }
    if (!forwarded.ok) return portFailure(live.label, label, forwarded.detail)
    return { kind: "ready", connection: { id, kind: "ssh-port", origin: connectionOrigin(id), socket: forwarded.socket, port: registered.port, label } }
  }
  const describePort = (id: string, registered: { sshId: string; port: number }): ConnectionSummary | null => {
    const parent = entry(registered.sshId)
    if (parent === undefined) return null
    const summary = describeSsh(parent)
    return { ...summary, id, kind: "ssh-port", label: portLabel(parent.destination, registered.port) }
  }
  const describeServer = (current: KnownServer): ConnectionSummary => ({ id: current.id, kind: "http", label: serverLabel(current.host, current.port), state: "ready", detail: "" })

  // Registered CLI servers by id.
  const cli = new Map<string, CliServer>()
  let cliInflight: Promise<{ ok: true } | { ok: false; detail: string }> | null = null
  const refreshCli = (): Promise<{ ok: true } | { ok: false; detail: string }> => {
    if (cliInflight !== null) return cliInflight
    const started = deps.listCli().catch((error: unknown): CliList => ({ ok: false, detail: errorMessage(error) })).then((result) => {
      cliInflight = null
      if (!result.ok) return { ok: false as const, detail: result.detail }
      const next = new Map(result.servers.map((server) => [cliId(server.port), server]))
      for (const id of [...cli.keys()]) {
        if (next.has(id)) continue
        cli.delete(id)
        emit(id, null)
      }
      for (const [id, server] of next) {
        const previous = cli.get(id)
        if (previous !== undefined && previous.origin === server.origin) continue
        cli.set(id, server)
        const current = cliLookup(id, server)
        if (current !== null) emit(id, current)
      }
      return { ok: true as const }
    })
    cliInflight = started
    return started
  }
  const lookupCli = async (id: string): Promise<ConnectionLookup> => {
    const registered = cli.get(id)
    if (registered === undefined) await refreshCli()
    const server = cli.get(id)
    return (server === undefined ? null : cliLookup(id, server)) ?? { kind: "unknown" }
  }
  const describeCli = (id: string, server: CliServer): ConnectionSummary => ({ id, kind: "cli", label: cliLabel(server.port), state: "ready", detail: "" })

  const describeSsh = (current: KnownConnection): ConnectionSummary => {
    const summary = (state: ConnectionState, detail: string): ConnectionSummary => ({ id: current.id, kind: "ssh", label: current.destination, state, detail })
    const live = sessions.get(current.id)
    if (live === undefined) return summary("idle", "")
    const state = live.session.state()
    switch (state.kind) {
      case "connecting":
      case "ready":
        return summary(state.kind, "")
      case "reconnecting":
      case "failed":
        return summary(state.kind, state.failure.detail)
    }
  }

  const describeLocal = (): ConnectionSummary => {
    const summary = (state: ConnectionState, detail: string): ConnectionSummary => ({ id: LOCAL_CONNECTION, kind: "daemon", label: LOCAL_CONNECTION, state, detail })
    if (localInflight !== null) return summary("connecting", "")
    if (local === null) return summary("idle", "")
    if (local.kind === "failed") return summary("failed", local.failure.detail)
    return summary("ready", "")
  }

  const lookup = async (id: string): Promise<ConnectionLookup> => {
    if (id === LOCAL_CONNECTION) {
      if (localInflight !== null) return localInflight
      return local ?? ensureLocal()
    }
    if (cliPort(id) !== null) return lookupCli(id)
    const httpServer = server(id)
    if (httpServer !== undefined) return { kind: "ready", connection: httpConnection(httpServer) }
    const registered = ports.get(id)
    if (registered !== undefined) return lookupPort(id, registered)
    if (id === APP_HOST || entry(id) === undefined) return { kind: "unknown" }
    return lookupSsh(id)
  }

  const holdOne = (id: string): void => {
    holds.set(id, (holds.get(id) ?? 0) + 1)
    clearGrace(id)
  }
  const releaseOne = (id: string): number => {
    const count = (holds.get(id) ?? 0) - 1
    if (count > 0) {
      holds.set(id, count)
      return count
    }
    holds.delete(id)
    scheduleGrace(id)
    return 0
  }

  return {
    lookup,
    peek(id) {
      if (id === LOCAL_CONNECTION) return local
      if (cliPort(id) !== null) {
        const server = cli.get(id)
        return (server === undefined ? null : cliLookup(id, server)) ?? { kind: "unknown" }
      }
      const httpServer = server(id)
      if (httpServer !== undefined) return { kind: "ready", connection: httpConnection(httpServer) }
      if (ports.has(id)) return null
      if (id === APP_HOST || entry(id) === undefined) return { kind: "unknown" }
      const live = sessions.get(id)
      return live === undefined ? null : sshLookup(id, live, live.session.state())
    },
    async reensure(id) {
      if (id === LOCAL_CONNECTION) return (await ensureLocal()).kind === "ready"
      const registered = ports.get(id)
      if (registered !== undefined) {
        // A port forward that stopped answering is forwarded again; a host
        // that is not ready is re-ensured first.
        const parent = sessions.get(registered.sshId)
        if (parent === undefined) return false
        if (parent.session.state().kind === "ready") await parent.session.cancelPort(id)
        else if (!await parent.session.reensure()) return false
        return (await lookupPort(id, registered)).kind === "ready"
      }
      const live = sessions.get(id)
      return live === undefined ? false : live.session.reensure()
    },
    async stopAndRetry(id) {
      if (id === LOCAL_CONNECTION) {
        const current = await lookup(id)
        if (current.kind !== "failed" || current.connection !== "daemon" || current.failure.status === undefined) return false
        await deps.stopLocal(current.failure.status.socket)
        return (await ensureLocal()).kind === "ready"
      }
      const live = sessions.get(id)
      if (live === undefined) return false
      await live.session.stopRemoteDaemon()
      return live.session.reensure()
    },
    // An ssh-port window holds its ssh host too; its forward is cancelled
    // when the last one closes.
    hold(id) {
      const registered = ports.get(id)
      if (registered !== undefined) holdOne(registered.sshId)
      holdOne(id)
    },
    release(id) {
      const registered = ports.get(id)
      const left = releaseOne(id)
      if (registered === undefined) return
      if (left === 0) void sessions.get(registered.sshId)?.session.cancelPort(id)
      releaseOne(registered.sshId)
    },
    known: () => known.map((current) => ({ ...current })),
    remember(target) {
      const existing = known.find((candidate) => candidate.destination === target.destination && candidate.port === target.port)
      if (existing !== undefined) return { ...existing }
      const added: KnownConnection = {
        id: assignId(target.destination, new Set([...known.map((candidate) => candidate.id), ...ports.keys()])),
        destination: target.destination,
        port: target.port,
        addedAt: new Date(deps.timers.now()).toISOString(),
      }
      known = [...known, added]
      save()
      return { ...added }
    },
    servers: () => servers.map((current) => ({ ...current })),
    findServer(host, port) {
      const found = servers.find((candidate) => candidate.host === host && candidate.port === port)
      return found === undefined ? null : { ...found }
    },
    rememberServer(host, port) {
      const existing = servers.find((candidate) => candidate.host === host && candidate.port === port)
      if (existing !== undefined) return { ...existing }
      const added: KnownServer = {
        id: assignServerId(host, port, new Set(servers.map((candidate) => candidate.id))),
        host,
        port,
        addedAt: new Date(deps.timers.now()).toISOString(),
      }
      servers = [...servers, added]
      save()
      return { ...added }
    },
    registerPort(sshId, host, port) {
      const parent = entry(sshId)
      if (parent === undefined) return { ok: false, detail: "Unknown SSH host." }
      const id = `${sshId}-${port}`
      if (entry(id) !== undefined) return { ok: false, detail: `The known SSH host ${entry(id)?.destination ?? id} already uses the name ${id}.` }
      const previous = ports.get(id)
      if (previous !== undefined && previous.host === host) return { ok: true, id }
      if (previous !== undefined) void sessions.get(sshId)?.session.cancelPort(id)
      ports.set(id, { sshId, host, port })
      return { ok: true, id }
    },
    async dropPort(id) {
      const registered = ports.get(id)
      if (registered === undefined) return
      ports.delete(id)
      await sessions.get(registered.sshId)?.session.cancelPort(id)
      emit(id, null)
    },
    async listServers(id) {
      const live = sessions.get(id)
      if (live === undefined) return { ok: false, detail: "The SSH connection is not open." }
      return live.session.listServers()
    },
    async forget(id) {
      if (server(id) !== undefined) {
        servers = servers.filter((candidate) => candidate.id !== id)
        save()
        emit(id, null)
        return
      }
      if (entry(id) === undefined) return
      for (const [portId, registered] of [...ports]) {
        if (registered.sshId !== id) continue
        ports.delete(portId)
        emit(portId, null)
      }
      await closeSession(id, false)
      known = known.filter((candidate) => candidate.id !== id)
      save()
      emit(id, null)
    },
    describe(id) {
      if (id === LOCAL_CONNECTION) return describeLocal()
      const listed = cli.get(id)
      if (listed !== undefined) return describeCli(id, listed)
      const httpServer = server(id)
      if (httpServer !== undefined) return describeServer(httpServer)
      const registered = ports.get(id)
      if (registered !== undefined) return describePort(id, registered)
      const current = entry(id)
      return current === undefined ? null : describeSsh(current)
    },
    summaries: () => [
      describeLocal(),
      ...known.map(describeSsh),
      ...servers.map(describeServer),
      ...[...cli].sort(([, a], [, b]) => a.port - b.port).map(([id, listed]) => describeCli(id, listed)),
    ],
    refreshCli,
    dropCli(id) {
      if (!cli.delete(id)) return
      emit(id, null)
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    async close() {
      for (const timer of graceTimers.values()) deps.timers.clearTimeout(timer)
      graceTimers.clear()
      await Promise.all([...sessions.keys()].map((id) => closeSession(id, false)))
    },
    closeSync() {
      for (const timer of graceTimers.values()) deps.timers.clearTimeout(timer)
      graceTimers.clear()
      for (const live of sessions.values()) {
        live.unsubscribe()
        live.session.closeSync()
      }
      sessions.clear()
    },
  }
}

interface Live {
  session: SshSession
  label: string
  // failure is the session's last failure; it labels a connecting state that
  // follows one (a retry).
  failure: SshFailure | null
  unsubscribe: () => void
}

// sshLookup maps a session state; null for a first connect (no failure yet).
function sshLookup(id: string, live: Live, state: SshState): ConnectionLookup | null {
  switch (state.kind) {
    case "ready":
      return { kind: "ready", connection: sshConnection(id, live.label, state.socket, state.status) }
    case "reconnecting":
      return { kind: "connecting", label: live.label, failure: state.failure }
    case "failed":
      return { kind: "failed", connection: "ssh", failure: state.failure, label: live.label }
    case "connecting":
      return live.failure === null ? null : { kind: "connecting", label: live.label, failure: live.failure }
  }
}

// settled maps a finished attempt. The session's state after it (reconnecting
// or failed) decides the kind; a new attempt already under way is connecting.
function settled(id: string, live: Live, result: SshReady): ConnectionLookup {
  if (result.ok) return { kind: "ready", connection: sshConnection(id, live.label, result.socket, result.status) }
  const state = live.session.state()
  if (state.kind === "connecting") return { kind: "connecting", label: live.label, failure: result.failure }
  return sshLookup(id, live, state) ?? { kind: "failed", connection: "ssh", failure: result.failure, label: live.label }
}

// cliLookup is ready for a server with a loopback origin; null otherwise
// (parseCliList already drops those).
function cliLookup(id: string, server: CliServer): ConnectionLookup | null {
  const host = dialHost(server)
  if (host === null) return null
  return { kind: "ready", connection: { id, kind: "cli", origin: connectionOrigin(id), host, port: server.port, label: cliLabel(server.port) } }
}

// httpConnection is a known http server's connection (nothing is dialed).
function httpConnection(server: KnownServer): HttpConnection {
  return {
    id: server.id,
    kind: "http",
    origin: connectionOrigin(server.id),
    host: server.host.replace(/^\[(.*)\]$/, "$1"),
    urlHost: server.host,
    port: server.port,
    label: serverLabel(server.host, server.port),
  }
}

// serverLabel names an http server as its URLs do: <host>:<port>.
export function serverLabel(host: string, port: number): string {
  return `${host}:${port}`
}

// portLabel names a forwarded remote server: <ssh destination>:<port>.
export function portLabel(destination: string, port: number): string {
  return `${destination}:${port}`
}

// cliLabel names a CLI server as its pasted URLs do: localhost:<port>.
export function cliLabel(port: number): string {
  return `localhost:${port}`
}

function sshConnection(id: string, label: string, socket: string, status: DaemonStatus): Connection {
  return { id, kind: "ssh", origin: connectionOrigin(id), socket, status, label }
}

function localLookup(result: EnsureResult): ConnectionLookup {
  if (!result.ok) return { kind: "failed", connection: "daemon", failure: result, label: LOCAL_CONNECTION }
  return {
    kind: "ready",
    connection: { id: LOCAL_CONNECTION, kind: "daemon", origin: connectionOrigin(LOCAL_CONNECTION), socket: result.status.socket, status: result.status, label: LOCAL_CONNECTION },
  }
}

function sameLocal(a: ConnectionLookup, b: ConnectionLookup): boolean {
  if (a.kind === "ready" && b.kind === "ready" && (a.connection.kind === "daemon" || a.connection.kind === "ssh") && (b.connection.kind === "daemon" || b.connection.kind === "ssh")) {
    const x = a.connection.status
    const y = b.connection.status
    return x.pid === y.pid && x.socket === y.socket && x.protocol === y.protocol && x.version === y.version
  }
  if (a.kind === "failed" && b.kind === "failed") return a.failure.kind === b.failure.kind && a.failure.detail === b.failure.detail
  return a.kind === b.kind
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
