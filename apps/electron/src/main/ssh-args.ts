// Pure ssh argv builders and remote command constants (decision 0070).
// Every ssh call is argv only (never a local shell); `--` always precedes the
// destination, and nothing from links, aliases, or remote output is ever
// interpolated into a remote command.
import { isIPv4, isIPv6 } from "node:net"
import type { SshTarget } from "./ssh-connection.ts"

// REMOTE_PATH_DIRS are appended to the remote PATH (existing entries win).
export const REMOTE_PATH_DIRS: readonly string[] = [
  "$HOME/.local/bin", "${GOPATH:-$HOME/go}/bin", "$HOME/bin", "/opt/homebrew/bin", "/home/linuxbrew/.linuxbrew/bin", "/usr/local/bin",
]

function remoteCommand(servefArgs: string): string {
  const script = `PATH="$PATH:${REMOTE_PATH_DIRS.join(":")}"; export PATH; `
    + `command -v servef >/dev/null 2>&1 || { echo "servef-missing $(uname -s) $(uname -m)" >&2; exit 127; }; `
    + `exec servef ${servefArgs}`
  return `sh -c '${script}'`
}

// REMOTE_ENSURE runs `servef daemon ensure --json` on the remote; a missing
// binary prints `servef-missing <uname -s> <uname -m>` to stderr, exit 127.
// No single quote or `!` inside the sh -c string.
export const REMOTE_ENSURE: string = remoteCommand("daemon ensure --json")
// REMOTE_STOP is REMOTE_ENSURE with `exec servef daemon stop`.
export const REMOTE_STOP: string = remoteCommand("daemon stop")
// REMOTE_LIST is REMOTE_ENSURE with `exec servef control list --json`: the
// running CLI servers on the host (decision 0076).
export const REMOTE_LIST: string = remoteCommand("control list --json")

const USER_PART = /^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$/
const HOST_PART = /^[A-Za-z0-9._][A-Za-z0-9._-]{0,252}$/

// validDestination accepts an ssh config alias or user@host (plan grammar):
// no leading `-`, no `%`, whitespace, quotes, or `/`; IPv6 literals allowed.
export function validDestination(destination: string): boolean {
  const at = destination.indexOf("@")
  const user = at === -1 ? null : destination.slice(0, at)
  const host = at === -1 ? destination : destination.slice(at + 1)
  if (user !== null && !USER_PART.test(user)) return false
  return HOST_PART.test(host) || validIPv6Literal(host)
}

// validIPv6Literal accepts an unbracketed IPv6 address without a zone index.
function validIPv6Literal(host: string): boolean {
  return host.includes(":") && !host.includes("%") && /^[0-9A-Fa-f:.]+$/.test(host) && isIPv6(host)
}

const MAX_SOCKET_BYTES = 107

// validRemoteSocket: absolute, ≤ 107 bytes, no `:`, whitespace, control
// characters, or `%`.
export function validRemoteSocket(path: string): boolean {
  if (!path.startsWith("/") || Buffer.byteLength(path, "utf8") > MAX_SOCKET_BYTES) return false
  for (const char of path) {
    const code = char.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f || char === ":" || char === "%" || /\s/u.test(char)) return false
  }
  return true
}

// portForwardSpec is the `<host>:<port>` side of `-L <socket>:<host>:<port>`
// for a remote TCP server (decision 0076): host must be an IPv4 or IPv6
// literal (brackets added) or localhost, port an integer 1-65535; null
// otherwise, so nothing else reaches ssh's argv.
export function portForwardSpec(host: string, port: number): string | null {
  if (!Number.isInteger(port) || port < 1 || port > 65_535) return null
  const bare = host.replace(/^\[(.*)\]$/, "$1")
  if (bare === "localhost" || isIPv4(bare)) return `${bare}:${port}`
  if (validIPv6Literal(bare)) return `[${bare}]:${port}`
  return null
}

function portArgs(target: SshTarget): string[] {
  return target.port === null ? [] : ["-p", String(target.port)]
}

// masterArgs builds the app-owned ControlMaster argv (plan "ssh invocation"),
// with `-F config` when config is set and `-p port` when target.port is set.
export function masterArgs(target: SshTarget, controlPath: string, config: string | null): string[] {
  return [
    "-M", "-N",
    "-o", "ControlMaster=yes",
    "-o", `ControlPath=${controlPath}`,
    "-o", "ControlPersist=no",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=15",
    "-o", "ServerAliveInterval=15",
    "-o", "ServerAliveCountMax=3",
    "-o", "ClearAllForwardings=yes",
    "-o", "StreamLocalBindUnlink=yes",
    "-o", "StreamLocalBindMask=0177",
    "-o", "ForwardAgent=no",
    "-o", "ForwardX11=no",
    "-o", "PermitLocalCommand=no",
    "-o", "RemoteCommand=none",
    "-o", "RequestTTY=no",
    "-o", "LogLevel=ERROR",
    ...(config === null ? [] : ["-F", config]),
    ...portArgs(target),
    "--", target.destination,
  ]
}

export type MuxOperation =
  | { op: "check" }
  | { op: "exit" }
  | { op: "forward" | "cancel"; local: string; remote: string }

// Every mux client: -o ControlMaster=no -o ControlPath=<p> -o BatchMode=yes
// -o ProxyCommand=false -F none (spike B0: -F none keeps user LocalForwards
// out of the master; ProxyCommand=false stops a silent direct connection when
// the master is dead). The user's config never applies to mux clients.
function muxOptions(target: SshTarget, controlPath: string): string[] {
  return [
    "-o", "ControlMaster=no",
    "-o", `ControlPath=${controlPath}`,
    "-o", "BatchMode=yes",
    "-o", "ProxyCommand=false",
    "-F", "none",
    ...portArgs(target),
  ]
}

// muxControlArgs builds `ssh -O <op> [-L local:remote] <mux opts> -- <dest>`;
// remote is a validated remote socket or a portForwardSpec.
export function muxControlArgs(target: SshTarget, controlPath: string, operation: MuxOperation): string[] {
  const forward = operation.op === "forward" || operation.op === "cancel" ? ["-L", `${operation.local}:${operation.remote}`] : []
  return ["-O", operation.op, ...forward, ...muxOptions(target, controlPath), "--", target.destination]
}

// muxCommandArgs builds `ssh -n -T <mux opts> -- <dest> <command>`; command is
// REMOTE_ENSURE, REMOTE_STOP, or REMOTE_LIST only.
export function muxCommandArgs(target: SshTarget, controlPath: string, command: string): string[] {
  return ["-n", "-T", ...muxOptions(target, controlPath), "--", target.destination, command]
}

// sshGArgs builds `ssh -G [-F config] -- <alias>` for aliases read from config only.
export function sshGArgs(alias: string, config: string | null): string[] {
  return ["-G", ...(config === null ? [] : ["-F", config]), "--", alias]
}

export type SshVersion = { kind: "openssh"; major: number; minor: number } | { kind: "other" }

// parseVersion reads `ssh -V` stderr (OpenSSH_<major>.<minor>...).
export function parseVersion(stderr: string): SshVersion {
  const match = /\bOpenSSH_(?:for_Windows_)?(\d+)\.(\d+)/.exec(stderr)
  if (match === null) return { kind: "other" }
  return { kind: "openssh", major: Number(match[1]), minor: Number(match[2]) }
}

export interface SshGOutput {
  hostname: string
  user: string
  port: number
}

// parseSshG reads `ssh -G` stdout (lowercase `key value` lines); null when a
// field is missing or invalid.
export function parseSshG(stdout: string): SshGOutput | null {
  const values = new Map<string, string>()
  for (const line of stdout.split("\n")) {
    const match = /^(\S+)\s+(.*?)\s*$/.exec(line)
    if (match === null) continue
    const key = (match[1] ?? "").toLowerCase()
    if (!values.has(key)) values.set(key, match[2] ?? "")
  }
  const hostname = values.get("hostname") ?? ""
  const user = values.get("user") ?? ""
  const portText = values.get("port") ?? ""
  if (hostname === "" || user === "" || !/^\d{1,5}$/.test(portText)) return null
  const port = Number(portText)
  if (port < 1 || port > 65_535) return null
  return { hostname, user, port }
}
