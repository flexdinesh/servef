import type { DaemonStatus } from "./daemon-contract.ts"
import type { EnsureFailure, EnsureResult } from "./daemon.ts"

export const LOCAL_CONNECTION = "local"

export interface Connection {
  id: string
  kind: "daemon"
  // origin is the renderer origin, servef-app://<id>.
  origin: string
  socket: string
  status: DaemonStatus
}

export type ConnectionLookup =
  | { kind: "ready"; connection: Connection }
  | { kind: "failed"; failure: EnsureFailure }
  | { kind: "unknown" }

export interface Connections {
  // lookup awaits an in-flight ensure; it starts the first ensure on demand.
  lookup(id: string): Promise<ConnectionLookup>
  // reensure runs ensure again (single-flight per id); true when ready after.
  reensure(id: string): Promise<boolean>
  subscribe(listener: (id: string, lookup: ConnectionLookup) => void): () => void
}

// connectionOrigin is the renderer origin of connection id.
export function connectionOrigin(id: string): string {
  return `servef-app://${id}`
}

// createConnections registers only the local daemon connection in M3.
// Listeners hear each ensure whose outcome differs from the last one.
export function createConnections(ensure: () => Promise<EnsureResult>): Connections {
  let current: ConnectionLookup | null = null
  let inflight: Promise<ConnectionLookup> | null = null
  const listeners = new Set<(id: string, lookup: ConnectionLookup) => void>()

  const run = (): Promise<ConnectionLookup> => {
    if (inflight !== null) return inflight
    const started = ensure().then(toLookup, (error: unknown): ConnectionLookup => ({
      kind: "failed",
      failure: { ok: false, kind: "failed", detail: error instanceof Error ? error.message : String(error) },
    })).then((lookup) => {
      const changed = current === null || !sameLookup(current, lookup)
      current = lookup
      inflight = null
      if (changed) for (const listener of listeners) listener(LOCAL_CONNECTION, lookup)
      return lookup
    })
    inflight = started
    return started
  }

  return {
    async lookup(id) {
      if (id !== LOCAL_CONNECTION) return { kind: "unknown" }
      if (inflight !== null) return inflight
      return current ?? run()
    },
    async reensure(id) {
      if (id !== LOCAL_CONNECTION) return false
      return (await run()).kind === "ready"
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
  }
}

function toLookup(result: EnsureResult): ConnectionLookup {
  if (!result.ok) return { kind: "failed", failure: result }
  return {
    kind: "ready",
    connection: { id: LOCAL_CONNECTION, kind: "daemon", origin: connectionOrigin(LOCAL_CONNECTION), socket: result.status.socket, status: result.status },
  }
}

function sameLookup(a: ConnectionLookup, b: ConnectionLookup): boolean {
  if (a.kind === "ready" && b.kind === "ready") {
    const x = a.connection.status
    const y = b.connection.status
    return x.pid === y.pid && x.socket === y.socket && x.protocol === y.protocol && x.version === y.version
  }
  if (a.kind === "failed" && b.kind === "failed") return a.failure.kind === b.failure.kind && a.failure.detail === b.failure.detail
  return a.kind === b.kind
}
