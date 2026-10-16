import { formatBytes } from "./format-bytes.ts"
import type { LanguageServerStatus, NavigationStatus } from "./navigation-contract.ts"

// LANGUAGE_SERVER_HEADER opts a navigation request into language servers. A
// header, not a query parameter, so other sites cannot start servers.
export const LANGUAGE_SERVER_HEADER = "X-Servef-Language-Servers"

export function navigationStatusQuery(project: string, path: string, refresh: boolean): string {
  const query = new URLSearchParams({ project, path })
  if (refresh) query.set("refresh", "1")
  return query.toString()
}

// providerLabel is the status bar's short name for who answers navigation.
export function providerLabel(status: NavigationStatus): string {
  if (status.provider === "language-server" && status.server) {
    return status.server.state === "starting" ? `${status.server.name}…` : status.server.name
  }
  return "Syntax"
}

// serverSummary explains a language server's state in a sentence.
export function serverSummary(server: LanguageServerStatus, enabled: boolean): string {
  switch (server.state) {
    case "missing":
      return `${server.name} is not installed.`
    case "stopped":
      return enabled ? `${server.name} starts on the next navigation.` : `${server.name} is installed.`
    case "starting":
      return `${server.name} is starting; syntax navigation answers meanwhile.`
    case "ready":
      return server.memory > 0 ? `${server.name} is running, using ${formatBytes(server.memory)}.` : `${server.name} is running.`
    case "failed":
      return server.message ? `${server.name} failed: ${server.message}` : `${server.name} failed.`
  }
}

// pollsServer reports whether a status changes on its own: a running
// server's memory and idle stop, or a start in progress.
export function pollsServer(status: NavigationStatus | null): boolean {
  return status !== null && status.languageServers && (status.server?.state === "starting" || status.server?.state === "ready")
}
