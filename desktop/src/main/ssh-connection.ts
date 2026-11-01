// Per-connection ssh state machine (decision 0070): app-owned ControlMaster,
// ensure over the mux, Unix-socket forward, probe, backoff, teardown.
import { spawn } from "node:child_process"
import type { DaemonStatus } from "./daemon-contract.ts"
import { interpretEnsure } from "./daemon.ts"
import type { ExecFile, ExecOutcome, ProbeOutcome } from "./daemon.ts"
import { unlinkQuietly } from "./runtime-dir.ts"
import type { RuntimeDirResult } from "./runtime-dir.ts"
import { masterArgs, muxCommandArgs, muxControlArgs, parseVersion, REMOTE_ENSURE, REMOTE_STOP, validRemoteSocket } from "./ssh-args.ts"
import { classifySsh, parseServefMissing, permanent } from "./ssh-errors.ts"
import type { SshFailure, SshFailureKind, SshPhase } from "./ssh-errors.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export interface SshTarget {
  // id is ssh-<slug>; "" for unconfirmed candidates.
  id: string
  destination: string
  port: number | null
}

export interface ChildHandle {
  readonly pid: number | undefined
  onExit(listener: (code: number | null, signal: string | null) => void): void
  onStderr(listener: (chunk: string) => void): void
  kill(signal: "SIGTERM" | "SIGKILL"): void
  exited(): boolean
}

// Spawn starts a long-lived process: argv only, shell false, stdin and stdout
// ignored, stderr piped.
export type Spawn = (file: string, args: readonly string[], env: NodeJS.ProcessEnv) => ChildHandle

// nodeSpawn adapts node:child_process.spawn. It never throws: a process that
// cannot start (ENOENT, invalid arguments) reports its error as a stderr
// chunk and then exits with (null, null). onExit fires after stderr closed.
export const nodeSpawn: Spawn = (file, args, env) => {
  const exitListeners: ((code: number | null, signal: string | null) => void)[] = []
  const stderrListeners: ((chunk: string) => void)[] = []
  let exit: { code: number | null; signal: string | null } | null = null
  const emitStderr = (chunk: string): void => {
    for (const listener of stderrListeners) listener(chunk)
  }
  const finish = (code: number | null, signal: string | null): void => {
    if (exit !== null) return
    exit = { code, signal }
    for (const listener of exitListeners.splice(0)) listener(code, signal)
  }
  let child: ReturnType<typeof spawn> | null = null
  try {
    child = spawn(file, [...args], { env, stdio: ["ignore", "ignore", "pipe"], shell: false, windowsHide: true })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    setImmediate(() => {
      emitStderr(`Cannot run ${file}: ${message}\n`)
      finish(null, null)
    })
  }
  if (child !== null) {
    const started = child
    started.stderr?.setEncoding("utf8")
    started.stderr?.on("data", (chunk: string) => emitStderr(chunk))
    started.on("error", (error) => {
      if (started.pid !== undefined) return
      emitStderr(`Cannot run ${file}: ${error.message}\n`)
      finish(null, null)
    })
    started.on("close", (code, signal) => finish(code, signal))
  }
  return {
    get pid() {
      return child?.pid
    },
    onExit(listener) {
      const done = exit
      if (done === null) exitListeners.push(listener)
      else queueMicrotask(() => listener(done.code, done.signal))
    },
    onStderr(listener) {
      stderrListeners.push(listener)
    },
    kill(signal) {
      if (exit !== null || child === null) return
      try {
        child.kill(signal)
      } catch {
        // Already gone.
      }
    },
    exited: () => exit !== null,
  }
}

export type SshReady = { ok: true; socket: string; status: DaemonStatus } | { ok: false; failure: SshFailure }

export type SshState =
  | { kind: "connecting" }
  | { kind: "ready"; socket: string; status: DaemonStatus }
  | { kind: "reconnecting"; failure: SshFailure; delayMs: number }
  | { kind: "failed"; failure: SshFailure }

// State semantics:
// - connecting: the first attempt (or a Retry from failed) is running.
// - ready: the forward answers; socket is the local forward socket.
// - reconnecting: a transient failure (or the master closed after ready);
//   the next attempt starts after delayMs. The state stays reconnecting while
//   that attempt runs and is replaced by ready, failed, or a new reconnecting.
// - failed: a permanent failure; nothing retries until reensure().
export interface SshSession {
  // state is the current state (synchronous).
  state(): SshState
  // ready resolves when the current attempt settles: ready (socket is the
  // local forward socket) or that attempt's failure. A transient failure then
  // moves to reconnecting (backoff 1 s doubling to 30 s); a permanent one to failed.
  // While waiting out a backoff (and after the master closed), it resolves
  // with the failure that caused it; it never rejects.
  ready(): Promise<SshReady>
  // reensure: ready → ensure over the mux again (remote socket changed →
  // cancel + forward), probe; failed or reconnecting → a new attempt now.
  // Single flight. Resolves true when ready after.
  reensure(): Promise<boolean>
  // stopRemoteDaemon runs REMOTE_STOP over the mux (mismatch page). It needs
  // a live master: a mismatch failure keeps the master for this; false
  // without one.
  stopRemoteDaemon(): Promise<boolean>
  onState(listener: (state: SshState) => void): () => void
  // close: -O exit (2 s), SIGTERM the master if alive, unlink f-* and c-*.
  close(): Promise<void>
  // closeSync (will-quit): SIGTERM the master, unlink synchronously.
  closeSync(): void
}

export interface SshConnector {
  connect(target: SshTarget): SshSession
}

export interface SshConnectorDeps {
  // ssh is the binary (SERVEF_DESKTOP_SSH, else ssh on PATH, else /usr/bin/ssh).
  ssh: string
  // config is SERVEF_DESKTOP_SSH_CONFIG: master and alias lister only.
  config: string | null
  runtime: RuntimeDirResult
  // env is the environment for ssh children (process.env plus SSH_ASKPASS_REQUIRE=never).
  env: NodeJS.ProcessEnv
  spawn: Spawn
  run: ExecFile
  // probe GETs /_daemon/status over the local forward socket.
  probe(socket: string): Promise<ProbeOutcome>
  timers: Timers
  // exists reports a path exists (ControlPath wait).
  exists(path: string): boolean
}

export const CONTROL_WAIT_MS = 25_000
export const ENSURE_SSH_TIMEOUT_MS = 20_000
export const STOP_SSH_TIMEOUT_MS = 10_000
export const MUX_TIMEOUT_MS = 10_000
export const EXIT_TIMEOUT_MS = 2_000
// CONTROL_POLL_MS is the ControlPath stat interval.
export const CONTROL_POLL_MS = 100
// INITIAL_BACKOFF_MS doubles per transient failure up to MAX_BACKOFF_MS; ready resets it.
export const INITIAL_BACKOFF_MS = 1_000
export const MAX_BACKOFF_MS = 30_000
// MASTER_STDERR_MAX bounds the master stderr ring buffer.
export const MASTER_STDERR_MAX = 8_192

const MAX_DETAIL = 2_000

type Preflight = { ok: true } | { ok: false; kind: SshFailureKind; detail: string }

// createSshConnector runs `ssh -V` once (first connect) for preflight. A
// passing preflight is cached for the connector's life; a failing one is
// re-run by the next attempt (Retry after installing OpenSSH works).
export function createSshConnector(deps: SshConnectorDeps): SshConnector {
  let passed = false
  let flight: Promise<Preflight> | null = null
  const preflight = (): Promise<Preflight> => {
    if (passed) return Promise.resolve({ ok: true })
    if (flight !== null) return flight
    const current = runPreflight(deps).then((result) => {
      flight = null
      if (result.ok) passed = true
      return result
    })
    flight = current
    return current
  }
  return { connect: (target) => createSession(deps, target, preflight) }
}

async function runPreflight(deps: SshConnectorDeps): Promise<Preflight> {
  const outcome = await runSafe(deps, ["-V"], MUX_TIMEOUT_MS)
  const error = outcome.error
  if (error?.timedOut) return { ok: false, kind: "failed", detail: `${deps.ssh} -V did not finish within ${MUX_TIMEOUT_MS / 1000} s.` }
  if (error !== undefined) {
    return { ok: false, kind: classifySsh({ phase: "preflight", code: null, stderr: "", masterStderr: "" }), detail: `Cannot run ${deps.ssh}: ${error.message}` }
  }
  const text = `${outcome.stderr}\n${outcome.stdout}`
  if (parseVersion(text).kind === "openssh" && classifySsh({ phase: "preflight", code: outcome.code ?? 0, stderr: text, masterStderr: "" }) === "sshTooOld") {
    const line = text.trim().split("\n")[0] ?? ""
    return { ok: false, kind: "sshTooOld", detail: `${line.trim()} is too old: servef needs OpenSSH 6.7 or later.` }
  }
  return { ok: true }
}

async function runSafe(deps: SshConnectorDeps, args: readonly string[], timeoutMs: number): Promise<ExecOutcome> {
  try {
    return await deps.run(deps.ssh, args, { timeoutMs, env: deps.env })
  } catch (error) {
    return { code: null, stdout: "", stderr: "", error: { message: errorMessage(error), timedOut: false } }
  }
}

function succeeded(outcome: ExecOutcome): boolean {
  return outcome.code === 0 && outcome.error === undefined
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

// tail bounds user-facing stderr to its last MAX_DETAIL characters.
function tail(text: string): string {
  const trimmed = text.trim()
  return trimmed.length > MAX_DETAIL ? `…${trimmed.slice(-MAX_DETAIL)}` : trimmed
}

function joinDetail(summary: string, stderr: string): string {
  const bounded = tail(stderr)
  return bounded === "" ? summary : `${summary}\n${bounded}`
}

function unlinkSafe(file: string): void {
  try {
    unlinkQuietly(file)
  } catch {
    // Best effort: a file we cannot remove is replaced by the next bind.
  }
}

interface Master {
  child: ChildHandle
  // stderr is the last MASTER_STDERR_MAX characters.
  stderr: string
  exit: { code: number | null; signal: string | null } | null
  exited: Promise<void>
}

interface Delay {
  // promise resolves true when the time elapsed, false when cancelled.
  promise: Promise<boolean>
  cancel(): void
}

type EnsureStep = { ok: true; status: DaemonStatus } | { ok: false; result: SshReady }

function createSession(deps: SshConnectorDeps, target: SshTarget, preflight: () => Promise<Preflight>): SshSession {
  const destination = target.destination
  const listeners = new Set<(state: SshState) => void>()
  const delays = new Map<number, { handle: TimerHandle; resolve: (elapsed: boolean) => void }>()
  const paths = deps.runtime.ok
    ? { controlPath: deps.runtime.runtime.controlPath(target.id), forwardSocket: deps.runtime.runtime.forwardSocket(target.id) }
    : null
  let state: SshState = { kind: "connecting" }
  let closed = false
  let closing: Promise<void> | null = null
  // epoch changes when an attempt starts, the master is lost after ready, or
  // the session closes; async steps of an older epoch stop.
  let epoch = 0
  let master: Master | null = null
  let stopping: Promise<void> = Promise.resolve()
  let forwardedRemote: string | null = null
  let attemptInFlight = false
  let retryTimer: TimerHandle | null = null
  let nextDelay = INITIAL_BACKOFF_MS
  let reensureFlight: Promise<boolean> | null = null
  let current: Promise<SshReady>

  const alive = (at: number): boolean => !closed && epoch === at
  const failure = (kind: SshFailureKind, detail: string, extra: Partial<SshFailure> = {}): SshFailure => ({ ...extra, kind, destination, detail })
  const fail = (kind: SshFailureKind, detail: string, extra: Partial<SshFailure> = {}): SshReady => ({ ok: false, failure: failure(kind, detail, extra) })
  const closedResult = (): SshReady => fail("failed", `The SSH connection to ${destination} was closed.`)

  const setState = (next: SshState): void => {
    state = next
    for (const listener of listeners) {
      try {
        listener(next)
      } catch (error) {
        console.error(`servef: ssh state listener failed: ${errorMessage(error)}`)
      }
    }
  }

  const delay = (ms: number): Delay => {
    if (closed) return { promise: Promise.resolve(false), cancel: () => {} }
    let settle: (elapsed: boolean) => void = () => {}
    const promise = new Promise<boolean>((resolve) => {
      settle = resolve
    })
    const handle = deps.timers.setTimeout(() => {
      delays.delete(handle.id)
      settle(true)
    }, ms)
    delays.set(handle.id, { handle, resolve: settle })
    return {
      promise,
      cancel() {
        if (!delays.delete(handle.id)) return
        deps.timers.clearTimeout(handle)
        settle(false)
      },
    }
  }

  const unlinkFiles = (): void => {
    if (paths === null) return
    unlinkSafe(paths.forwardSocket)
    unlinkSafe(paths.controlPath)
  }

  const terminate = async (m: Master | null): Promise<void> => {
    if (m === null || m.exit !== null) return
    m.child.kill("SIGTERM")
    const grace = delay(EXIT_TIMEOUT_MS)
    const exited = await Promise.race([m.exited.then(() => true), grace.promise.then(() => false)])
    grace.cancel()
    if (!exited && m.exit === null) m.child.kill("SIGKILL")
  }

  // stopMaster detaches the master, terminates it, and unlinks our files once
  // it is gone (so an exiting master cannot remove a new master's ControlPath).
  const stopMaster = (): Promise<void> => {
    const m = master
    master = null
    forwardedRemote = null
    const previous = stopping
    stopping = (async () => {
      await previous
      await terminate(m)
      unlinkFiles()
    })()
    return stopping
  }

  const scheduleRetry = (cause: SshFailure): void => {
    const delayMs = nextDelay
    nextDelay = Math.min(nextDelay * 2, MAX_BACKOFF_MS)
    setState({ kind: "reconnecting", failure: cause, delayMs })
    retryTimer = deps.timers.setTimeout(() => {
      retryTimer = null
      if (!closed) void startAttempt()
    }, delayMs)
  }

  // settleFailure tears the master down (kept for mismatch so the remote
  // daemon can be stopped over the mux), then fails or schedules a retry.
  const settleFailure = (cause: SshFailure): void => {
    if (cause.kind !== "mismatch") void stopMaster()
    if (permanent(cause.kind)) setState({ kind: "failed", failure: cause })
    else scheduleRetry(cause)
  }

  const lostAfterReady = (m: Master): void => {
    epoch += 1
    master = null
    forwardedRemote = null
    reensureFlight = null
    unlinkFiles()
    const cause = failure("unreachable", joinDetail(`The SSH connection to ${destination} closed.`, m.stderr))
    current = Promise.resolve({ ok: false, failure: cause })
    scheduleRetry(cause)
  }

  const spawnMaster = (controlPath: string): Master => {
    let resolveExit: () => void = () => {}
    const exited = new Promise<void>((resolve) => {
      resolveExit = resolve
    })
    const child = deps.spawn(deps.ssh, masterArgs(target, controlPath, deps.config), deps.env)
    const m: Master = { child, stderr: "", exit: null, exited }
    child.onStderr((chunk) => {
      m.stderr = (m.stderr + chunk).slice(-MASTER_STDERR_MAX)
    })
    child.onExit((code, signal) => {
      if (m.exit !== null) return
      m.exit = { code, signal }
      resolveExit()
      if (!closed && master === m && state.kind === "ready") lostAfterReady(m)
    })
    return m
  }

  const waitForControlPath = async (m: Master, controlPath: string): Promise<"ready" | "exited" | "timeout" | "closed"> => {
    const start = deps.timers.now()
    for (;;) {
      if (closed) return "closed"
      if (m.exit !== null) return "exited"
      if (deps.exists(controlPath)) return "ready"
      if (deps.timers.now() - start >= CONTROL_WAIT_MS) return "timeout"
      const tick = delay(CONTROL_POLL_MS)
      const exitedFirst = await Promise.race([tick.promise.then(() => false), m.exited.then(() => true)])
      if (exitedFirst) tick.cancel()
    }
  }

  const masterFailure = (m: Master): SshReady => {
    const code = m.exit?.code ?? null
    const kind = classifySsh({ phase: "master", code, stderr: m.stderr, masterStderr: m.stderr })
    const how = code === null ? `ssh ended (${m.exit?.signal ?? "no exit code"})` : `ssh exited with code ${code}`
    return fail(kind, tail(m.stderr) || `${how} before the connection to ${destination} was ready.`)
  }

  const stepFailure = (phase: SshPhase, outcome: ExecOutcome, m: Master, summary: string, status?: DaemonStatus): SshReady => {
    if (m.exit !== null) return masterFailure(m)
    const extra: Partial<SshFailure> = status === undefined ? {} : { status }
    if (outcome.error?.timedOut) return fail("failed", `${summary} (timed out)`, extra)
    const kind = classifySsh({ phase, code: outcome.code, stderr: outcome.stderr, masterStderr: m.stderr })
    return fail(kind, tail(outcome.stderr) || outcome.error?.message || summary, extra)
  }

  const ensure = async (at: number, m: Master, controlPath: string): Promise<EnsureStep> => {
    const outcome = await runSafe(deps, muxCommandArgs(target, controlPath, REMOTE_ENSURE), ENSURE_SSH_TIMEOUT_MS)
    if (!alive(at)) return { ok: false, result: closedResult() }
    if (outcome.error?.timedOut) {
      return { ok: false, result: fail("failed", `servef daemon ensure on ${destination} did not finish within ${ENSURE_SSH_TIMEOUT_MS / 1000} s.`) }
    }
    if (m.exit !== null) return { ok: false, result: masterFailure(m) }
    const missing = outcome.code === 127 ? parseServefMissing(outcome.stderr) : null
    if (missing !== null) {
      return { ok: false, result: fail("servefMissing", tail(outcome.stderr), { os: missing.os, arch: missing.arch }) }
    }
    const ensured = interpretEnsure(outcome)
    if (ensured.ok) {
      if (!validRemoteSocket(ensured.status.socket)) {
        return { ok: false, result: fail("failed", `Unsupported remote socket path: ${JSON.stringify(ensured.status.socket)}.`, { status: ensured.status }) }
      }
      return { ok: true, status: ensured.status }
    }
    const extra: Partial<SshFailure> = ensured.status === undefined ? {} : { status: ensured.status }
    if (ensured.kind === "mismatch") return { ok: false, result: fail("mismatch", ensured.detail, extra) }
    const kind = classifySsh({ phase: "ensure", code: outcome.code, stderr: outcome.stderr, masterStderr: m.stderr })
    return { ok: false, result: fail(kind, ensured.detail, extra) }
  }

  // forwardTo forwards the local socket to remote unless already forwarded,
  // cancelling a previous forward first. null on success.
  const forwardTo = async (at: number, m: Master, controlPath: string, forwardSocket: string, status: DaemonStatus): Promise<SshReady | null> => {
    const remote = status.socket
    if (forwardedRemote === remote) return null
    const previous = forwardedRemote
    if (previous !== null) {
      forwardedRemote = null
      await runSafe(deps, muxControlArgs(target, controlPath, { op: "cancel", local: forwardSocket, remote: previous }), MUX_TIMEOUT_MS)
      if (!alive(at)) return closedResult()
    }
    unlinkSafe(forwardSocket)
    const outcome = await runSafe(deps, muxControlArgs(target, controlPath, { op: "forward", local: forwardSocket, remote }), MUX_TIMEOUT_MS)
    if (!alive(at)) return closedResult()
    if (!succeeded(outcome)) return stepFailure("forward", outcome, m, `ssh could not forward ${forwardSocket} to ${remote} on ${destination}.`, status)
    forwardedRemote = remote
    return null
  }

  const probe = async (forwardSocket: string): Promise<ProbeOutcome> => {
    try {
      return await deps.probe(forwardSocket)
    } catch {
      return "failed"
    }
  }

  // establish: ensure, forward, probe. A probe without a response head
  // re-ensures once (a changed socket is re-forwarded); a second miss at the
  // same socket during connect means the server refuses the forward.
  const establish = async (at: number, m: Master, mode: "connect" | "reensure"): Promise<SshReady> => {
    if (paths === null) return closedResult()
    const { controlPath, forwardSocket } = paths
    const notAnswering = (status: DaemonStatus): SshReady =>
      fail("failed", `The servef daemon on ${destination} did not answer through the SSH forward.`, { status })
    const first = await ensure(at, m, controlPath)
    if (!first.ok) return first.result
    const forwarded = await forwardTo(at, m, controlPath, forwardSocket, first.status)
    if (forwarded !== null) return forwarded
    const answer = await probe(forwardSocket)
    if (!alive(at)) return closedResult()
    if (answer === "ok") return { ok: true, socket: forwardSocket, status: first.status }
    if (answer === "failed") return notAnswering(first.status)
    const second = await ensure(at, m, controlPath)
    if (!second.ok) return second.result
    const changed = second.status.socket !== first.status.socket
    const reforwarded = await forwardTo(at, m, controlPath, forwardSocket, second.status)
    if (reforwarded !== null) return reforwarded
    const again = await probe(forwardSocket)
    if (!alive(at)) return closedResult()
    if (again === "ok") return { ok: true, socket: forwardSocket, status: second.status }
    if (again === "gone" && !changed && mode === "connect") {
      return fail("forwardDenied", joinDetail(`${destination} did not forward connections to ${second.status.socket} although the servef daemon is running there.`, m.stderr), { status: second.status })
    }
    return notAnswering(second.status)
  }

  const connectOnce = async (at: number): Promise<SshReady> => {
    const checked = await preflight()
    if (!alive(at)) return closedResult()
    if (!checked.ok) return fail(checked.kind, checked.detail)
    if (!deps.runtime.ok || paths === null) {
      const detail = deps.runtime.ok ? "no runtime directory" : `Cannot use the private runtime directory ${deps.runtime.dir}: ${deps.runtime.detail}.`
      return fail("runtimeDir", detail)
    }
    const { controlPath } = paths
    await stopMaster()
    if (!alive(at)) return closedResult()
    const m = spawnMaster(controlPath)
    master = m
    const waited = await waitForControlPath(m, controlPath)
    if (!alive(at)) return closedResult()
    if (waited === "exited") return masterFailure(m)
    if (waited !== "ready") {
      const kind = classifySsh({ phase: "master", code: null, stderr: m.stderr, masterStderr: m.stderr })
      return fail(kind, joinDetail(`Timed out after ${CONTROL_WAIT_MS / 1000} s waiting for the SSH connection to ${destination}.`, m.stderr))
    }
    const check = await runSafe(deps, muxControlArgs(target, controlPath, { op: "check" }), MUX_TIMEOUT_MS)
    if (!alive(at)) return closedResult()
    if (!succeeded(check)) return stepFailure("master", check, m, `ssh -O check failed for ${destination}.`)
    const result = await establish(at, m, "connect")
    // A master that died while the forward was probed is not ready.
    return result.ok && m.exit !== null ? masterFailure(m) : result
  }

  const startAttempt = (): Promise<SshReady> => {
    if (retryTimer !== null) {
      deps.timers.clearTimeout(retryTimer)
      retryTimer = null
    }
    epoch += 1
    const at = epoch
    attemptInFlight = true
    reensureFlight = null
    if (state.kind === "failed") setState({ kind: "connecting" })
    const attempt = (async (): Promise<SshReady> => {
      let result: SshReady
      try {
        result = await connectOnce(at)
      } catch (error) {
        result = fail("failed", `Connecting to ${destination} failed: ${errorMessage(error)}`)
      }
      if (!alive(at)) return closed ? closedResult() : result
      attemptInFlight = false
      if (result.ok) {
        nextDelay = INITIAL_BACKOFF_MS
        setState({ kind: "ready", socket: result.socket, status: result.status })
      } else {
        settleFailure(result.failure)
      }
      return result
    })()
    current = attempt
    return attempt
  }

  const reensureReady = async (): Promise<boolean> => {
    const at = epoch
    const m = master
    if (m === null || m.exit !== null) return false
    let result: SshReady
    try {
      result = await establish(at, m, "reensure")
    } catch (error) {
      result = fail("failed", `Re-ensuring ${destination} failed: ${errorMessage(error)}`)
    }
    if (!alive(at)) return false
    if (result.ok) {
      setState({ kind: "ready", socket: result.socket, status: result.status })
      return true
    }
    epoch += 1
    current = Promise.resolve(result)
    settleFailure(result.failure)
    return false
  }

  const shutdown = (): Master | null => {
    closed = true
    epoch += 1
    reensureFlight = null
    if (retryTimer !== null) {
      deps.timers.clearTimeout(retryTimer)
      retryTimer = null
    }
    for (const pending of delays.values()) {
      deps.timers.clearTimeout(pending.handle)
      pending.resolve(false)
    }
    delays.clear()
    const m = master
    master = null
    forwardedRemote = null
    return m
  }

  current = startAttempt()

  return {
    state: () => state,
    ready: () => current,
    reensure() {
      if (closed) return Promise.resolve(false)
      if (attemptInFlight) return current.then((result) => result.ok)
      if (state.kind !== "ready") {
        if (state.kind === "failed") nextDelay = INITIAL_BACKOFF_MS
        return startAttempt().then((result) => result.ok)
      }
      if (reensureFlight !== null) return reensureFlight
      const flight = reensureReady().finally(() => {
        if (reensureFlight === flight) reensureFlight = null
      })
      reensureFlight = flight
      return flight
    },
    async stopRemoteDaemon() {
      const m = master
      if (closed || paths === null || m === null || m.exit !== null) return false
      return succeeded(await runSafe(deps, muxCommandArgs(target, paths.controlPath, REMOTE_STOP), STOP_SSH_TIMEOUT_MS))
    },
    onState(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    close() {
      if (closing !== null) return closing
      const m = shutdown()
      closing = (async () => {
        if (m !== null && m.exit === null && paths !== null) {
          await runSafe(deps, muxControlArgs(target, paths.controlPath, { op: "exit" }), EXIT_TIMEOUT_MS)
          if (m.exit === null) m.child.kill("SIGTERM")
        }
        unlinkFiles()
      })()
      return closing
    },
    closeSync() {
      const m = shutdown()
      if (m !== null && m.exit === null) m.child.kill("SIGTERM")
      unlinkFiles()
    },
  }
}
