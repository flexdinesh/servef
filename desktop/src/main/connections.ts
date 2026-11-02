import { cliId, cliPort, dialHost } from "./cli-servers.ts"
import type { CliList, CliServer } from "./cli-servers.ts"
import { assignId } from "./connection-store.ts"
import type { ConnectionStore, KnownConnection } from "./connection-store.ts"
import type { DaemonStatus } from "./daemon-contract.ts"
import type { EnsureFailure, EnsureResult } from "./daemon.ts"
import type { SshConnector, SshReady, SshSession, SshState } from "./ssh-connection.ts"
import type { SshFailure } from "./ssh-errors.ts"
import type { SshHints } from "./ssh-links.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export const LOCAL_CONNECTION = "local"
// APP_HOST is the app pages host (servef-app://app/connect); no upstream.
export const APP_HOST = "app"
// RELEASE_GRACE_MS: an ssh connection no window holds closes after this.
export const RELEASE_GRACE_MS = 30_000

export type ConnectionKind = "daemon" | "ssh" | "cli"

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

export type Connection = DaemonConnection | CliConnection

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
  // in-flight connect or starts one; unknown ids (and APP_HOST) are unknown.
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
  // remember stores a confirmed destination (with the link's hints) and
  // returns its entry; an existing destination+port reuses its entry and
  // merges the hints.
  remember(target: { destination: string; port: number | null }, hints: SshHints): KnownConnection
  // forget closes the session (no grace) and removes the entry.
  forget(id: string): Promise<void>
  // describe summarizes local, known ssh, and registered cli ids; null otherwise.
  describe(id: string): ConnectionSummary | null
  // summaries lists local first, then known ssh hosts in store order, then
  // registered CLI servers by port.
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
  let known: KnownConnection[] = deps.store.load()
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

  const save = (): void => deps.store.save(known)

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
    if (id === APP_HOST || entry(id) === undefined) return { kind: "unknown" }
    return lookupSsh(id)
  }

  return {
    lookup,
    peek(id) {
      if (id === LOCAL_CONNECTION) return local
      if (cliPort(id) !== null) {
        const server = cli.get(id)
        return (server === undefined ? null : cliLookup(id, server)) ?? { kind: "unknown" }
      }
      if (id === APP_HOST || entry(id) === undefined) return { kind: "unknown" }
      const live = sessions.get(id)
      return live === undefined ? null : sshLookup(id, live, live.session.state())
    },
    async reensure(id) {
      if (id === LOCAL_CONNECTION) return (await ensureLocal()).kind === "ready"
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
    hold(id) {
      holds.set(id, (holds.get(id) ?? 0) + 1)
      clearGrace(id)
    },
    release(id) {
      const count = (holds.get(id) ?? 0) - 1
      if (count > 0) {
        holds.set(id, count)
        return
      }
      holds.delete(id)
      scheduleGrace(id)
    },
    known: () => known.map(copyKnown),
    remember(target, hints) {
      const existing = known.find((candidate) => candidate.destination === target.destination && candidate.port === target.port)
      const empty = Object.keys(hints).length === 0
      if (existing !== undefined) {
        const key = canonicalHints(hints)
        if (!empty && !existing.hints.some((hint) => canonicalHints(hint) === key)) {
          const merged = { ...existing, hints: [...existing.hints, { ...hints }] }
          known = known.map((candidate) => candidate === existing ? merged : candidate)
          save()
          return copyKnown(merged)
        }
        save()
        return copyKnown(existing)
      }
      const added: KnownConnection = {
        id: assignId(target.destination, new Set(known.map((candidate) => candidate.id))),
        destination: target.destination,
        port: target.port,
        hints: empty ? [] : [{ ...hints }],
        addedAt: new Date(deps.timers.now()).toISOString(),
      }
      known = [...known, added]
      save()
      return copyKnown(added)
    },
    async forget(id) {
      if (entry(id) === undefined) return
      await closeSession(id, false)
      known = known.filter((candidate) => candidate.id !== id)
      save()
      emit(id, null)
    },
    describe(id) {
      if (id === LOCAL_CONNECTION) return describeLocal()
      const server = cli.get(id)
      if (server !== undefined) return describeCli(id, server)
      const current = entry(id)
      return current === undefined ? null : describeSsh(current)
    },
    summaries: () => [
      describeLocal(),
      ...known.map(describeSsh),
      ...[...cli].sort(([, a], [, b]) => a.port - b.port).map(([id, server]) => describeCli(id, server)),
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
  if (a.kind === "ready" && b.kind === "ready" && a.connection.kind !== "cli" && b.connection.kind !== "cli") {
    const x = a.connection.status
    const y = b.connection.status
    return x.pid === y.pid && x.socket === y.socket && x.protocol === y.protocol && x.version === y.version
  }
  if (a.kind === "failed" && b.kind === "failed") return a.failure.kind === b.failure.kind && a.failure.detail === b.failure.detail
  return a.kind === b.kind
}

function copyKnown(known: KnownConnection): KnownConnection {
  return { ...known, hints: known.hints.map((hint) => ({ ...hint })) }
}

// canonicalHints is JSON with sorted keys, so equal hints compare equal.
function canonicalHints(hints: SshHints): string {
  return JSON.stringify(Object.entries(hints).filter(([, value]) => value !== undefined).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0))
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
