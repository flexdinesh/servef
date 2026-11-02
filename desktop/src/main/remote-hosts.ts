// ssh hosts from the Connect page (decision 0075): the host page model, the
// folder picker, and opening folders and remote servers. Pure over deps:
// paths travel only in HTTP requests over the forwarded daemon socket, never
// in remote commands.
import { isIP } from "node:net"
import { isLoopbackHost, originHost } from "./cli-servers.ts"
import type { CliList, CliServer } from "./cli-servers.ts"
import { serverHost } from "./connection-store.ts"
import { connectionOrigin } from "./connections.ts"
import type { ConnectionLookup } from "./connections.ts"
import type { DaemonStatus, DirectoryListing } from "./daemon-contract.ts"
import type { DirectoriesOutcome, OpenOutcome, ProbeOutcome } from "./daemon.ts"
import { projectLanding } from "./open-paths.ts"
import type { SshFailure } from "./ssh-errors.ts"

// ServerRow is a running servef server on the host.
export interface ServerRow {
  // origin identifies the server in the open form; label is <host>:<port>.
  origin: string
  label: string
  // direct: reachable without ssh (it does not listen on loopback only).
  direct: boolean
}

export type ServersView = { ok: true; servers: ServerRow[] } | { ok: false; detail: string }

export type HostView =
  | { kind: "unknown" }
  | { kind: "ready"; id: string; destination: string; roots: string[]; servers: ServersView }
  | { kind: "connecting"; id: string; destination: string; detail: string }
  // servers lists the directly reachable servers after forwardDenied, else null.
  | { kind: "failed"; id: string; destination: string; failure: SshFailure; servers: ServersView | null }

export type BrowseView =
  | { kind: "host"; view: HostView }
  | { kind: "listing"; id: string; destination: string; listing: DirectoryListing; hidden: boolean }
  // upgrade: servef on the host predates the directory listing.
  | { kind: "error"; id: string; destination: string; path: string; error: string; upgrade: boolean }

export type ActionOutcome = { ok: true } | { ok: false; error: string }

export interface RemoteHostsDeps {
  // destination is a known ssh host's destination, else null.
  destination(id: string): string | null
  lookup(id: string): Promise<ConnectionLookup>
  listServers(id: string): Promise<CliList>
  status(socket: string): Promise<DaemonStatus | null>
  directories(socket: string, path: string): Promise<DirectoriesOutcome>
  open(socket: string, path: string): Promise<OpenOutcome>
  registerPort(id: string, host: string, port: number): { ok: true; id: string } | { ok: false; detail: string }
  dropPort(id: string): Promise<void>
  // probe sends one request to a forwarded server: "gone" when no response
  // head arrived.
  probe(socket: string, port: number): Promise<ProbeOutcome>
  // sshHostname is the `ssh -G` hostname of ssh host id, else null.
  sshHostname(id: string): Promise<string | null>
  // rememberServer stores an http server the user chose and returns its id.
  rememberServer(host: string, port: number): string
  route(url: string): void
}

export interface RemoteHosts {
  host(id: string): Promise<HostView>
  browse(id: string, path: string, hidden: boolean): Promise<BrowseView>
  openFolder(id: string, path: string): Promise<ActionOutcome>
  // openServer opens the listed server with origin: through an ssh-port
  // forward, else directly over http when it is reachable.
  openServer(id: string, origin: string): Promise<ActionOutcome>
}

// forwardHost is where the master forwards to: the origin's host when it is
// an IP literal or localhost; null otherwise.
export function forwardHost(server: CliServer): string | null {
  const host = originHost(server)
  if (host === null) return null
  return host === "localhost" || isIP(host) !== 0 ? host : null
}

// directHost is the URL hostname to reach the server without ssh: its
// listed host unless that is loopback (none) or unspecified (sshHostname,
// the host's `ssh -G` hostname); null when there is none.
export function directHost(server: CliServer, sshHostname: string | null): string | null {
  if (!reachableDirectly(server)) return null
  const host = bareHost(server)
  if (bindsAll(server)) return sshHostname === null ? null : serverHost(bracketed(sshHostname))
  return serverHost(bracketed(host))
}

function bracketed(host: string): string {
  return host.includes(":") && !host.startsWith("[") ? `[${host}]` : host
}

// reachableDirectly: the server does not listen on loopback only.
export function reachableDirectly(server: CliServer): boolean {
  return !isLoopbackHost(bareHost(server))
}

// bindsAll: the server listens on every address (0.0.0.0 or ::).
function bindsAll(server: CliServer): boolean {
  const host = bareHost(server)
  return host === "" || host === "0.0.0.0" || host === "::"
}

function bareHost(server: CliServer): string {
  return server.host.replace(/^\[(.*)\]$/, "$1")
}

// serverRows lists servers with a usable origin; direct marks those
// reachable without ssh.
export function serverRows(servers: readonly CliServer[]): ServerRow[] {
  const rows: ServerRow[] = []
  for (const server of servers) {
    const host = originHost(server)
    if (host === null) continue
    rows.push({
      origin: server.origin,
      label: `${bracketed(host)}:${server.port}`,
      direct: reachableDirectly(server),
    })
  }
  return rows
}

function serversView(list: CliList, directOnly: boolean): ServersView {
  if (!list.ok) return { ok: false, detail: list.detail }
  const rows = serverRows(list.servers)
  return { ok: true, servers: directOnly ? rows.filter((row) => row.direct) : rows }
}

export function createRemoteHosts(deps: RemoteHostsDeps): RemoteHosts {
  const host = async (id: string): Promise<HostView> => {
    const destination = deps.destination(id)
    if (destination === null) return { kind: "unknown" }
    const current = await deps.lookup(id)
    switch (current.kind) {
      case "unknown":
        return { kind: "unknown" }
      case "connecting":
        return { kind: "connecting", id, destination, detail: current.failure.detail }
      case "failed": {
        if (current.connection !== "ssh") return { kind: "unknown" }
        const servers = current.failure.kind === "forwardDenied" ? serversView(await deps.listServers(id), true) : null
        return { kind: "failed", id, destination, failure: current.failure, servers }
      }
      case "ready": {
        if (current.connection.kind !== "ssh") return { kind: "unknown" }
        const status = await deps.status(current.connection.socket)
        const roots = status?.roots ?? current.connection.status.roots
        return { kind: "ready", id, destination, roots, servers: serversView(await deps.listServers(id), false) }
      }
    }
  }

  // socketOf is the ready host's forwarded daemon socket, else its host view.
  const socketOf = async (id: string): Promise<{ socket: string; destination: string } | HostView> => {
    const destination = deps.destination(id)
    if (destination === null) return { kind: "unknown" }
    const current = await deps.lookup(id)
    if (current.kind === "ready" && current.connection.kind === "ssh") return { socket: current.connection.socket, destination }
    return host(id)
  }

  return {
    host,
    async browse(id, path, hidden) {
      const ready = await socketOf(id)
      if ("kind" in ready) return { kind: "host", view: ready }
      const outcome = await deps.directories(ready.socket, path)
      if (outcome.ok) return { kind: "listing", id, destination: ready.destination, listing: outcome.listing, hidden }
      return { kind: "error", id, destination: ready.destination, path, error: outcome.error, upgrade: outcome.upgrade }
    },
    async openFolder(id, path) {
      if (!path.startsWith("/")) return { ok: false, error: "Choose an absolute folder path." }
      const ready = await socketOf(id)
      if ("kind" in ready) return { ok: false, error: `The SSH connection to ${deps.destination(id) ?? id} is not ready.` }
      const outcome = await deps.open(ready.socket, path)
      if (!outcome.ok) return { ok: false, error: outcome.error }
      deps.route(projectLanding(connectionOrigin(id), outcome.result.project))
      return { ok: true }
    },
    async openServer(id, origin) {
      const destination = deps.destination(id)
      if (destination === null) return { ok: false, error: "Unknown SSH host." }
      // The form names a server; only one the host lists right now opens.
      const listed = await deps.listServers(id)
      if (!listed.ok) return { ok: false, error: `Could not list servef servers on ${destination}: ${listed.detail}` }
      const server = listed.servers.find((candidate) => candidate.origin === origin)
      if (server === undefined) return { ok: false, error: `No servef server at ${origin} is running on ${destination}.` }
      const target = forwardHost(server)
      let why = `${origin} is not an address servef forwards.`
      if (target !== null) {
        const registered = deps.registerPort(id, target, server.port)
        if (!registered.ok) why = registered.detail
        else {
          const current = await deps.lookup(registered.id)
          if (current.kind === "ready" && current.connection.kind === "ssh-port") {
            if (await deps.probe(current.connection.socket, server.port) !== "gone") {
              deps.route(`${connectionOrigin(registered.id)}/`)
              return { ok: true }
            }
            why = `${destination} did not forward connections to ${origin}.`
          } else if (current.kind === "failed") why = current.failure.detail
          else why = `The SSH connection to ${destination} is not ready.`
          await deps.dropPort(registered.id)
        }
      }
      const direct = directHost(server, bindsAll(server) ? await deps.sshHostname(id) : null)
      if (direct === null) return { ok: false, error: `${why} The server listens only on ${destination}'s loopback, so it cannot be reached without the forward.` }
      deps.route(`${connectionOrigin(deps.rememberServer(direct, server.port))}/`)
      return { ok: true }
    },
  }
}
