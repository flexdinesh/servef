// Pure ssh failure classification (decision 0070).
import type { DaemonStatus } from "./daemon-contract.ts"
import { parseVersion } from "./ssh-args.ts"

export type SshFailureKind =
  | "sshMissing" | "sshTooOld" | "hostKey" | "auth" | "unreachable" | "servefMissing" | "servefTooOld"
  | "mismatch" | "forwardDenied" | "runtimeDir" | "failed"

export interface SshFailure {
  kind: SshFailureKind
  destination: string
  // detail is user-facing text: the stderr tail (≤ 2 KB) or a summary.
  detail: string
  // os and arch come from `servef-missing <os> <arch>` (servefMissing only).
  os?: string
  arch?: string
  // status is the remote daemon status (mismatch, failed after ensure).
  status?: DaemonStatus
}

export type SshPhase = "preflight" | "master" | "ensure" | "forward" | "probe"

const HOST_KEY = [/Host key verification failed/, /REMOTE HOST IDENTIFICATION HAS CHANGED/, /No \S+ host key is known/]
const AUTH = [/Permission denied \(/, /Too many authentication failures/]
const UNREACHABLE = [/Could not resolve hostname/, /Connection refused/, /Connection timed out/, /No route to host/, /Network is unreachable/]
// Only while the master connects: the server dropped the connection.
const MASTER_DROPPED = [/Connection closed by/, /Connection reset/]
const SERVEF_TOO_OLD = [/unknown command "daemon"/, /unknown flag: --json/, /unknown shorthand flag/]

function matches(text: string, patterns: readonly RegExp[]): boolean {
  return patterns.some((pattern) => pattern.test(text))
}

// connectionKind classifies stderr of the master connection (also a mux
// client's own stderr): host key, auth, then reachability.
function connectionKind(text: string, master: boolean): SshFailureKind | null {
  if (matches(text, HOST_KEY)) return "hostKey"
  if (matches(text, AUTH)) return "auth"
  if (matches(text, UNREACHABLE)) return "unreachable"
  if (master && matches(text, MASTER_DROPPED)) return "unreachable"
  return null
}

// classifySsh maps a failed step to a kind (plan table, spike corrections:
// host keys match `No \S+ host key is known` / `Host key verification
// failed`; forwardDenied is never decided from stderr alone, the state
// machine infers it from repeated probe resets).
//
// Input conventions:
// - preflight: code null means `ssh -V` could not run (ENOENT, EACCES):
//   sshMissing. Otherwise stderr is the `ssh -V` output: OpenSSH older than
//   6.7 is sshTooOld; anything else is failed.
// - master: stderr is the master's stderr (masterStderr the same). Code null
//   with no recognized stderr means the ControlPath wait timed out (the caller
//   killed the master) or the master died by a signal: unreachable.
// - ensure, forward, probe: stderr is the mux client's; masterStderr is
//   consulted for connection kinds when stderr has none (the master died).
export function classifySsh(input: { phase: SshPhase; code: number | null; stderr: string; masterStderr: string }): SshFailureKind {
  const { phase, code, stderr, masterStderr } = input
  if (phase === "preflight") {
    if (code === null) return "sshMissing"
    const version = parseVersion(stderr)
    if (version.kind === "openssh" && !atLeast(version.major, version.minor, 6, 7)) return "sshTooOld"
    return "failed"
  }
  const master = phase === "master"
  const own = connectionKind(stderr, master)
  if (own !== null) return own
  if (code === 127 && parseServefMissing(stderr) !== null) return "servefMissing"
  if (code !== 0 && code !== null && matches(stderr, SERVEF_TOO_OLD)) return "servefTooOld"
  if (master) return code === null && stderr.trim() === "" ? "unreachable" : "failed"
  const fromMaster = connectionKind(masterStderr, true)
  if (fromMaster !== null) return fromMaster
  return "failed"
}

function atLeast(major: number, minor: number, wantMajor: number, wantMinor: number): boolean {
  return major > wantMajor || (major === wantMajor && minor >= wantMinor)
}

const PERMANENT: ReadonlySet<SshFailureKind> = new Set<SshFailureKind>([
  "sshMissing", "sshTooOld", "hostKey", "auth", "servefMissing", "servefTooOld", "mismatch", "forwardDenied", "runtimeDir",
])

// permanent kinds stop and wait for Retry; the rest are retried with backoff.
export function permanent(kind: SshFailureKind): boolean {
  return PERMANENT.has(kind)
}

// parseServefMissing reads `servef-missing <os> <arch>` from stderr.
export function parseServefMissing(stderr: string): { os: string; arch: string } | null {
  const match = /(?:^|\n)\s*servef-missing ([A-Za-z0-9._-]{1,64}) ([A-Za-z0-9._-]{1,64})\s*(?:\r?\n|$)/.exec(stderr)
  if (match === null) return null
  return { os: match[1] ?? "", arch: match[2] ?? "" }
}
