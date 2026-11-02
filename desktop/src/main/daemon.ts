import { execFile } from "node:child_process"
import { EXPECTED_PROTOCOL, isControlErrorBody, isDaemonStatus, isOpenResult } from "./daemon-contract.ts"
import type { DaemonStatus, OpenResult } from "./daemon-contract.ts"
import { isDaemonGone, type Forward } from "./forward.ts"

// ExecFile runs a binary and resolves with its exit outcome; it never rejects
// for a non-zero exit. error is set when the process could not run or timed out.
export type ExecFile = (file: string, args: readonly string[], options: { timeoutMs: number }) => Promise<ExecOutcome>

export interface ExecOutcome {
  code: number | null
  stdout: string
  stderr: string
  error?: { code?: string; message: string; timedOut: boolean }
}

export type EnsureFailure = { ok: false; kind: "missing" | "mismatch" | "failed"; detail: string; status?: DaemonStatus }
export type EnsureResult = { ok: true; status: DaemonStatus } | EnsureFailure

export const ENSURE_TIMEOUT_MS = 15_000

// MAX_DETAIL bounds the stderr tail a failure carries.
const MAX_DETAIL = 2_000

// devFlags maps SERVEF_DESKTOP_SOCKET and SERVEF_DESKTOP_ROOTS_FILE to
// `--socket` and `--roots-file`; nothing else.
export function devFlags(env: NodeJS.ProcessEnv): string[] {
  const flags: string[] = []
  const socket = env.SERVEF_DESKTOP_SOCKET
  if (socket !== undefined && socket !== "") flags.push("--socket", socket)
  const rootsFile = env.SERVEF_DESKTOP_ROOTS_FILE
  if (rootsFile !== undefined && rootsFile !== "") flags.push("--roots-file", rootsFile)
  return flags
}

// ensureDaemon runs `servef daemon ensure --json ...flags` and validates its status.
// ensure prints the status even when it exits non-zero on a protocol mismatch.
export async function ensureDaemon(binary: string, flags: readonly string[], run: ExecFile): Promise<EnsureResult> {
  const outcome = await run(binary, ["daemon", "ensure", "--json", ...flags], { timeoutMs: ENSURE_TIMEOUT_MS })
  const error = outcome.error
  if (error?.timedOut) return { ok: false, kind: "failed", detail: `servef daemon ensure timed out after ${ENSURE_TIMEOUT_MS / 1000} s.` }
  if (error?.code === "ENOENT" || error?.code === "EACCES") {
    return { ok: false, kind: "missing", detail: `Cannot run ${binary}: ${error.message}` }
  }
  const status = lastJSONLine(outcome.stdout)
  if (isDaemonStatus(status)) {
    if (status.protocol !== EXPECTED_PROTOCOL) {
      return {
        ok: false,
        kind: "mismatch",
        detail: `The running daemon speaks protocol ${status.protocol}; this app needs protocol ${EXPECTED_PROTOCOL}.`,
        status,
      }
    }
    if (outcome.code === 0 && error === undefined) return { ok: true, status }
  }
  const failure: EnsureFailure = { ok: false, kind: "failed", detail: failureDetail(outcome) }
  if (isDaemonStatus(status)) failure.status = status
  return failure
}

function lastJSONLine(stdout: string): unknown {
  const lines = stdout.split("\n").map((line) => line.trim()).filter((line) => line !== "")
  const last = lines.at(-1)
  if (last === undefined) return null
  try {
    return JSON.parse(last)
  } catch {
    return null
  }
}

function failureDetail(outcome: ExecOutcome): string {
  const stderr = outcome.stderr.trim()
  if (stderr !== "") return stderr.length > MAX_DETAIL ? `…${stderr.slice(-MAX_DETAIL)}` : stderr
  if (outcome.error !== undefined) return outcome.error.message
  return `servef daemon ensure exited with code ${outcome.code ?? "unknown"} and no usable status.`
}

// nodeExecFile adapts node:child_process.execFile to ExecFile.
export const nodeExecFile: ExecFile = (file, args, options) => new Promise((resolve) => {
  execFile(file, [...args], { timeout: options.timeoutMs, maxBuffer: 1 << 20, encoding: "utf8", windowsHide: true }, (error, stdout, stderr) => {
    if (error === null) {
      resolve({ code: 0, stdout, stderr })
      return
    }
    // A number code is the exit status; a string code is a spawn or buffer
    // failure; neither means the child was killed by a signal.
    if (typeof error.code === "number") {
      resolve({ code: error.code, stdout, stderr })
      return
    }
    if (typeof error.code === "string") {
      resolve({ code: null, stdout, stderr, error: { code: error.code, message: error.message, timedOut: false } })
      return
    }
    resolve({ code: null, stdout, stderr, error: { message: error.message, timedOut: error.killed === true } })
  })
})

export type OpenOutcome = { ok: true; result: OpenResult } | { ok: false; status: number; code: string; error: string }

// Control requests send Host: localhost and never Origin or Sec-Fetch-Site,
// which the daemon refuses as browser requests.
const CONTROL_HOST = "localhost"

// openPath posts /_daemon/open {path} over the socket. A daemon that does not
// answer yields status 0 and code "daemonUnavailable".
export async function openPath(forward: Forward, socket: string, path: string): Promise<OpenOutcome> {
  const payload = new TextEncoder().encode(JSON.stringify({ path }))
  let response: Response
  try {
    response = await forward({
      socket,
      method: "POST",
      path: "/_daemon/open",
      headers: { host: CONTROL_HOST, "content-type": "application/json", "content-length": String(payload.byteLength) },
      body: bytesStream(payload),
      stream: false,
    })
  } catch (error) {
    return { ok: false, status: 0, code: "daemonUnavailable", error: `The servef daemon is not answering: ${errorMessage(error)}` }
  }
  const body = await readJSON(response)
  if ((response.status === 200 || response.status === 201) && isOpenResult(body)) return { ok: true, result: body }
  if (isControlErrorBody(body)) return { ok: false, status: response.status, code: body.code ?? "", error: body.error }
  return { ok: false, status: response.status, code: "invalidResponse", error: `The servef daemon sent an unexpected answer (HTTP ${response.status}).` }
}

// STOP_WAIT_MS bounds how long stopDaemon waits for the daemon to go away.
export const STOP_WAIT_MS = 5_000

// stopDaemon posts /_daemon/stop, then resolves true once the socket stops
// answering (so a following ensure starts a new daemon), false otherwise.
export async function stopDaemon(forward: Forward, socket: string, waitMs = STOP_WAIT_MS): Promise<boolean> {
  try {
    const response = await forward({ socket, method: "POST", path: "/_daemon/stop", headers: { host: CONTROL_HOST }, body: null, stream: false })
    await response.arrayBuffer()
    if (response.status !== 202) return false
  } catch {
    return false
  }
  const deadline = Date.now() + waitMs
  while (Date.now() < deadline) {
    try {
      const status = await forward({ socket, method: "GET", path: "/_daemon/status", headers: { host: CONTROL_HOST }, body: null, stream: false })
      await status.arrayBuffer()
    } catch (error) {
      if (isDaemonGone(error)) return true
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  return false
}

function bytesStream(bytes: Uint8Array): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(bytes)
      controller.close()
    },
  })
}

async function readJSON(response: Response): Promise<unknown> {
  try {
    return JSON.parse(await response.text())
  } catch {
    return null
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
