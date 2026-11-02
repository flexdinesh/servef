import assert from "node:assert/strict"
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { afterEach, test } from "node:test"
import { EXPECTED_PROTOCOL } from "../src/main/daemon-contract.ts"
import type { DaemonStatus } from "../src/main/daemon-contract.ts"
import type { ExecOutcome, ProbeOutcome } from "../src/main/daemon.ts"
import { runtimePaths } from "../src/main/runtime-dir.ts"
import type { RuntimeDirResult } from "../src/main/runtime-dir.ts"
import { masterArgs, muxCommandArgs, muxControlArgs, REMOTE_ENSURE, REMOTE_STOP } from "../src/main/ssh-args.ts"
import {
  CONTROL_WAIT_MS, createSshConnector, ENSURE_SSH_TIMEOUT_MS, EXIT_TIMEOUT_MS, MUX_TIMEOUT_MS, nodeSpawn, STOP_SSH_TIMEOUT_MS,
} from "../src/main/ssh-connection.ts"
import type { ChildHandle, SshConnectorDeps, SshReady, SshSession, SshState, SshTarget } from "../src/main/ssh-connection.ts"
import { createFakeTimers, flush } from "./fake-timers.ts"
import type { FakeTimers } from "./fake-timers.ts"

const SOCKET_A = "/home/dee/.cache/servef/daemon.sock"
const SOCKET_B = "/home/dee/.cache/servef/daemon-2.sock"
const HOST_KEY_UNKNOWN = "No ED25519 host key is known for [127.0.0.1]:47343 and you have requested strict checking.\r\nHost key verification failed.\r\n"
const NO_KEY = "dee@127.0.0.1: Permission denied (publickey).\r\n"
const REFUSED = "ssh: connect to host 127.0.0.1 port 35983: Connection refused\r\n"

const target: SshTarget = { id: "ssh-devbox", destination: "devbox", port: null }

function status(socket = SOCKET_A, protocol = EXPECTED_PROTOCOL): DaemonStatus {
  return { protocol, version: "1.0.0", pid: 7, socket, roots: [], started: true }
}

function ensured(socket = SOCKET_A): ExecOutcome {
  return { code: 0, stdout: `${JSON.stringify(status(socket))}\n`, stderr: "" }
}

const ok: ExecOutcome = { code: 0, stdout: "", stderr: "" }

interface FakeChild extends ChildHandle {
  args: readonly string[]
  signals: string[]
  emit(chunk: string): void
  exit(code: number | null, signal?: string | null): void
}

type RunKind = "version" | "check" | "exit" | "forward" | "cancel" | "ensure" | "stop" | "other"

function runKind(args: readonly string[]): RunKind {
  if (args[0] === "-V") return "version"
  if (args[0] === "-O") {
    const op = args[1]
    if (op === "check" || op === "exit" || op === "forward" || op === "cancel") return op
  }
  const last = args.at(-1)
  if (last === REMOTE_ENSURE) return "ensure"
  if (last === REMOTE_STOP) return "stop"
  return "other"
}

interface Harness {
  dir: string
  controlPath: string
  forwardSocket: string
  fake: FakeTimers
  deps: SshConnectorDeps
  children: FakeChild[]
  // events lists spawn, run kinds, and probe in call order.
  events: string[]
  calls: { file: string; args: readonly string[]; timeoutMs: number; env: NodeJS.ProcessEnv | undefined }[]
  spawned: { file: string; args: readonly string[]; env: NodeJS.ProcessEnv }[]
  // onSpawn runs synchronously inside spawn; default: the ControlPath appears.
  onSpawn: (child: FakeChild) => void
  handlers: Partial<Record<RunKind, (args: readonly string[]) => ExecOutcome | Promise<ExecOutcome>>>
  probes: ProbeOutcome[]
}

const dirs: string[] = []
afterEach(() => {
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true })
})

function harness(runtime?: RuntimeDirResult): Harness {
  const dir = mkdtempSync(join(tmpdir(), "servef-ssh-conn-"))
  dirs.push(dir)
  const paths = runtimePaths(dir)
  const controlPath = paths.controlPath(target.id)
  const forwardSocket = paths.forwardSocket(target.id)
  const fake = createFakeTimers()
  const h: Harness = {
    dir,
    controlPath,
    forwardSocket,
    fake,
    children: [],
    events: [],
    calls: [],
    spawned: [],
    onSpawn: (child) => writeFileSync(controlPathOf(child.args), ""),
    handlers: {},
    probes: [],
    deps: {
      ssh: "/usr/bin/ssh",
      config: "/cfg/ssh_config",
      runtime: runtime ?? { ok: true, runtime: paths },
      env: { SSH_ASKPASS_REQUIRE: "never" },
      spawn: (file, args, env) => {
        h.events.push("spawn")
        h.spawned.push({ file, args, env })
        const child = fakeChild(args, controlPathOf(args))
        h.children.push(child)
        h.onSpawn(child)
        return child
      },
      run: async (file, args, options) => {
        const kind = runKind(args)
        h.events.push(kind)
        h.calls.push({ file, args, timeoutMs: options.timeoutMs, env: options.env })
        const handler = h.handlers[kind]
        if (handler !== undefined) return handler(args)
        return defaultRun(h, kind, args)
      },
      probe: async (socket) => {
        h.events.push(`probe ${socket}`)
        return h.probes.shift() ?? "ok"
      },
      timers: fake.timers,
      exists: (path) => existsSync(path),
    },
  }
  return h
}

function controlPathOf(args: readonly string[]): string {
  const option = args.find((arg) => arg.startsWith("ControlPath="))
  assert.ok(option !== undefined)
  return option.slice("ControlPath=".length)
}

function liveChild(h: Harness): FakeChild | undefined {
  return h.children.findLast((child) => !child.exited())
}

function defaultRun(h: Harness, kind: RunKind, args: readonly string[]): ExecOutcome {
  switch (kind) {
    case "version": return { code: 0, stdout: "", stderr: "OpenSSH_9.2p1 Debian-2+deb12u3, OpenSSL 3.0.15 3 Sep 2024\n" }
    case "ensure": return ensured()
    case "forward": {
      // ssh binds the local socket (and leaves it behind after exit).
      const spec = args[args.indexOf("-L") + 1] ?? ""
      writeFileSync(spec.slice(0, spec.indexOf(":")), "")
      return ok
    }
    case "exit": {
      const child = liveChild(h)
      if (child === undefined || !existsSync(h.controlPath)) return { code: 255, stdout: "", stderr: `Control socket connect(${h.controlPath}): No such file or directory\r\n` }
      child.exit(0)
      return ok
    }
    default: return ok
  }
}

// fakeChild behaves like ssh: it removes its ControlPath when it exits and
// dies on SIGTERM/SIGKILL.
function fakeChild(args: readonly string[], controlPath: string): FakeChild {
  const exitListeners: ((code: number | null, signal: string | null) => void)[] = []
  const stderrListeners: ((chunk: string) => void)[] = []
  let done: { code: number | null; signal: string | null } | null = null
  const child: FakeChild = {
    args,
    signals: [],
    pid: 1234,
    onExit(listener) {
      const finished = done
      if (finished === null) exitListeners.push(listener)
      else queueMicrotask(() => listener(finished.code, finished.signal))
    },
    onStderr(listener) {
      stderrListeners.push(listener)
    },
    kill(signal) {
      child.signals.push(signal)
      child.exit(null, signal)
    },
    exited: () => done !== null,
    emit(chunk) {
      for (const listener of stderrListeners) listener(chunk)
    },
    exit(code, signal = null) {
      if (done !== null) return
      done = { code, signal }
      if (existsSync(controlPath)) rmSync(controlPath)
      for (const listener of exitListeners) listener(code, signal)
    },
  }
  return child
}

// failMaster makes the master print stderr and exit before its ControlPath appears.
function failMaster(stderr: string, code: number | null = 255): (child: FakeChild) => void {
  return (child) => queueMicrotask(() => {
    child.emit(stderr)
    child.exit(code)
  })
}

async function settle(session: SshSession): Promise<SshReady> {
  const result = session.ready()
  await flush()
  return result
}

// advance moves fake time in steps so async continuations schedule their next timers.
async function advance(fake: FakeTimers, ms: number, step = 100): Promise<void> {
  for (let elapsed = 0; elapsed < ms; elapsed += step) {
    fake.advance(Math.min(step, ms - elapsed))
    await flush(2)
  }
  await flush()
}

function failureOf(result: SshReady): NonNullable<Extract<SshReady, { ok: false }>["failure"]> {
  assert.ok(!result.ok, "expected a failure")
  return result.failure
}

test("connect runs preflight, master, check, ensure, forward, probe in order with exact argv", async () => {
  const h = harness()
  writeFileSync(h.controlPath, "stale")
  writeFileSync(h.forwardSocket, "stale")
  const staleSeen: boolean[] = []
  h.onSpawn = () => {
    staleSeen.push(existsSync(h.controlPath), existsSync(h.forwardSocket))
    writeFileSync(h.controlPath, "")
  }
  const states: SshState[] = []
  const session = createSshConnector(h.deps).connect(target)
  session.onState((state) => states.push(state))
  assert.deepEqual(session.state(), { kind: "connecting" })
  const result = await settle(session)
  assert.deepEqual(result, { ok: true, socket: h.forwardSocket, status: status() })
  assert.deepEqual(session.state(), { kind: "ready", socket: h.forwardSocket, status: status() })
  assert.deepEqual(states, [{ kind: "ready", socket: h.forwardSocket, status: status() }])
  assert.deepEqual(staleSeen, [false, false])
  assert.deepEqual(h.events, ["version", "spawn", "check", "ensure", "forward", `probe ${h.forwardSocket}`])
  assert.deepEqual(h.spawned, [{ file: "/usr/bin/ssh", args: masterArgs(target, h.controlPath, "/cfg/ssh_config"), env: h.deps.env }])
  assert.deepEqual(h.calls.map(({ file, args, timeoutMs, env }) => ({ file, args, timeoutMs, env })), [
    { file: "/usr/bin/ssh", args: ["-V"], timeoutMs: MUX_TIMEOUT_MS, env: h.deps.env },
    { file: "/usr/bin/ssh", args: muxControlArgs(target, h.controlPath, { op: "check" }), timeoutMs: MUX_TIMEOUT_MS, env: h.deps.env },
    { file: "/usr/bin/ssh", args: muxCommandArgs(target, h.controlPath, REMOTE_ENSURE), timeoutMs: ENSURE_SSH_TIMEOUT_MS, env: h.deps.env },
    { file: "/usr/bin/ssh", args: muxControlArgs(target, h.controlPath, { op: "forward", local: h.forwardSocket, remote: SOCKET_A }), timeoutMs: MUX_TIMEOUT_MS, env: h.deps.env },
  ])
  assert.equal(h.fake.pending(), 0)
  session.closeSync()
})

test("ControlPath wait timeout is unreachable, kills the master, and retries", async () => {
  const h = harness()
  h.onSpawn = () => {}
  const session = createSshConnector(h.deps).connect(target)
  await flush()
  await advance(h.fake, CONTROL_WAIT_MS - 100)
  assert.deepEqual(session.state(), { kind: "connecting" })
  await advance(h.fake, 100)
  const failure = failureOf(await session.ready())
  assert.equal(failure.kind, "unreachable")
  assert.equal(failure.destination, "devbox")
  assert.match(failure.detail, /Timed out after 25 s waiting for the SSH connection to devbox/)
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(session.state().kind, "reconnecting")
  assert.deepEqual(h.events, ["version", "spawn"])
  session.closeSync()
})

for (const sample of [
  { name: "unknown host key", stderr: HOST_KEY_UNKNOWN, kind: "hostKey", next: "failed" },
  { name: "no usable key", stderr: NO_KEY, kind: "auth", next: "failed" },
  { name: "connection refused", stderr: REFUSED, kind: "unreachable", next: "reconnecting" },
] as const) {
  test(`master exit before the ControlPath: ${sample.name}`, async () => {
    const h = harness()
    h.onSpawn = failMaster(sample.stderr)
    const session = createSshConnector(h.deps).connect(target)
    const failure = failureOf(await settle(session))
    assert.equal(failure.kind, sample.kind)
    assert.equal(failure.detail, sample.stderr.trim())
    assert.equal(session.state().kind, sample.next)
    assert.deepEqual(h.events, ["version", "spawn"])
    // Permanent kinds schedule nothing.
    assert.equal(h.fake.pending(), sample.next === "failed" ? 0 : 1)
    if (sample.next === "failed") {
      await advance(h.fake, 60_000, 10_000)
      assert.equal(h.children.length, 1)
    }
    session.closeSync()
  })
}

test("servef missing on the remote carries os and arch and does not retry", async () => {
  const h = harness()
  h.handlers.ensure = () => ({ code: 127, stdout: "", stderr: "servef-missing Linux x86_64\n" })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.deepEqual(failure, { kind: "servefMissing", destination: "devbox", detail: "servef-missing Linux x86_64", os: "Linux", arch: "x86_64" })
  assert.equal(session.state().kind, "failed")
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(h.events.includes("forward"), false)
  assert.equal(h.fake.pending(), 0)
})

test("servef too old on the remote", async () => {
  const h = harness()
  h.handlers.ensure = () => ({ code: 1, stdout: "", stderr: "Error: unknown command \"daemon\" for \"servef\"\n" })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "servefTooOld")
  assert.equal(failure.detail, "Error: unknown command \"daemon\" for \"servef\"")
  assert.equal(session.state().kind, "failed")
})

test("mismatch keeps the master so the remote daemon can be stopped, then Retry reconnects", async () => {
  const h = harness()
  const newer = status(SOCKET_A, EXPECTED_PROTOCOL + 1)
  h.handlers.ensure = () => ({ code: 1, stdout: JSON.stringify(newer), stderr: "protocol mismatch" })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "mismatch")
  assert.deepEqual(failure.status, newer)
  assert.equal(session.state().kind, "failed")
  assert.deepEqual(h.children[0]?.signals, [])

  assert.equal(await session.stopRemoteDaemon(), true)
  const stop = h.calls.at(-1)
  assert.deepEqual(stop?.args, muxCommandArgs(target, h.controlPath, REMOTE_STOP))
  assert.equal(stop?.timeoutMs, STOP_SSH_TIMEOUT_MS)

  delete h.handlers.ensure
  const retried = session.reensure()
  assert.deepEqual(session.state(), { kind: "connecting" })
  await flush()
  assert.equal(await retried, true)
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(h.children.length, 2)
  assert.equal(session.state().kind, "ready")
  session.closeSync()
})

test("stopRemoteDaemon without a live master is false", async () => {
  const h = harness()
  h.onSpawn = failMaster(NO_KEY)
  const session = createSshConnector(h.deps).connect(target)
  await settle(session)
  assert.equal(await session.stopRemoteDaemon(), false)
  assert.equal(h.events.includes("stop"), false)
})

test("an unsupported remote socket path fails with the status and retries", async () => {
  const h = harness()
  h.handlers.ensure = () => ensured("/tmp/odd:path.sock")
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "failed")
  assert.match(failure.detail, /Unsupported remote socket path/)
  assert.equal(failure.status?.socket, "/tmp/odd:path.sock")
  assert.equal(session.state().kind, "reconnecting")
  assert.equal(h.events.includes("forward"), false)
  session.closeSync()
})

test("forwardDenied: probe resets, re-ensure returns the same socket, probe resets again", async () => {
  const h = harness()
  h.probes = ["gone", "gone"]
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "forwardDenied")
  assert.equal(failure.status?.socket, SOCKET_A)
  assert.deepEqual(h.events, ["version", "spawn", "check", "ensure", "forward", `probe ${h.forwardSocket}`, "ensure", `probe ${h.forwardSocket}`])
  assert.equal(session.state().kind, "failed")
  assert.equal(h.fake.pending(), 0)
})

test("a changed remote socket after a probe reset cancels the old forward and forwards the new one", async () => {
  const h = harness()
  h.probes = ["gone", "ok"]
  const sockets = [SOCKET_A, SOCKET_B]
  h.handlers.ensure = () => ensured(sockets.shift() ?? SOCKET_B)
  const session = createSshConnector(h.deps).connect(target)
  const result = await settle(session)
  assert.deepEqual(result, { ok: true, socket: h.forwardSocket, status: status(SOCKET_B) })
  assert.deepEqual(h.events.slice(2), ["check", "ensure", "forward", `probe ${h.forwardSocket}`, "ensure", "cancel", "forward", `probe ${h.forwardSocket}`])
  const muxTail = h.calls.slice(-2).map((call) => call.args)
  assert.deepEqual(muxTail, [
    muxControlArgs(target, h.controlPath, { op: "cancel", local: h.forwardSocket, remote: SOCKET_A }),
    muxControlArgs(target, h.controlPath, { op: "forward", local: h.forwardSocket, remote: SOCKET_B }),
  ])
  session.closeSync()
})

test("a probe that fails (not reset) is a transient failure with the status", async () => {
  const h = harness()
  h.probes = ["failed"]
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "failed")
  assert.equal(failure.status?.socket, SOCKET_A)
  assert.equal(session.state().kind, "reconnecting")
  session.closeSync()
})

test("a master that dies during the probe is not ready", async () => {
  const h = harness()
  h.deps.probe = async () => {
    h.children[0]?.emit("Connection to devbox closed by remote host.\r\n")
    h.children[0]?.exit(255)
    return "ok"
  }
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "failed")
  assert.equal(failure.detail, "Connection to devbox closed by remote host.")
  assert.equal(session.state().kind, "reconnecting")
  session.closeSync()
})

test("an ensure timeout is a transient failure", async () => {
  const h = harness()
  h.handlers.ensure = () => ({ code: null, stdout: "", stderr: "", error: { message: "killed", timedOut: true } })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "failed")
  assert.match(failure.detail, /did not finish within 20 s/)
  assert.equal(session.state().kind, "reconnecting")
  session.closeSync()
})

test("a forward failure is classified with the forward stderr", async () => {
  const h = harness()
  h.handlers.forward = () => ({ code: 255, stdout: "", stderr: "mux_client_forward: forwarding request failed: Port forwarding failed\r\n" })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "failed")
  assert.equal(failure.detail, "mux_client_forward: forwarding request failed: Port forwarding failed")
  assert.equal(failure.status?.socket, SOCKET_A)
  session.closeSync()
})

test("master exit after ready reconnects with backoff 1 s doubling to 30 s, reset after ready", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  const states: SshState[] = []
  session.onState((state) => states.push(state))

  h.onSpawn = failMaster(REFUSED)
  h.children[0]?.emit("Connection to devbox closed by remote host.\r\n")
  h.children[0]?.exit(255)
  const lost = session.state()
  assert.ok(lost.kind === "reconnecting")
  assert.equal(lost.delayMs, 1_000)
  assert.equal(lost.failure.kind, "unreachable")
  assert.equal(lost.failure.detail, "The SSH connection to devbox closed.\nConnection to devbox closed by remote host.")
  assert.equal(existsSync(h.forwardSocket), false)
  assert.equal(failureOf(await session.ready()).kind, "unreachable")

  const delays: number[] = [lost.delayMs]
  for (let attempt = 0; attempt < 6; attempt++) {
    const current = session.state()
    assert.ok(current.kind === "reconnecting")
    await advance(h.fake, current.delayMs, 500)
    const next = session.state()
    assert.ok(next.kind === "reconnecting")
    assert.equal(next.failure.kind, "unreachable")
    assert.equal(next.failure.detail, REFUSED.trim())
    delays.push(next.delayMs)
  }
  assert.deepEqual(delays, [1_000, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000])
  assert.equal(h.events.filter((event) => event === "version").length, 1)

  h.onSpawn = () => writeFileSync(h.controlPath, "")
  await advance(h.fake, 30_000, 1_000)
  assert.equal(session.state().kind, "ready")
  assert.equal(states.filter((state) => state.kind === "connecting").length, 0)

  liveChild(h)?.exit(255)
  const again = session.state()
  assert.ok(again.kind === "reconnecting")
  assert.equal(again.delayMs, 1_000)
  session.closeSync()
})

test("reensure from ready is single flight and re-forwards a changed socket", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  let release: (outcome: ExecOutcome) => void = () => {}
  h.handlers.ensure = () => new Promise<ExecOutcome>((resolve) => {
    release = resolve
  })
  const first = session.reensure()
  const second = session.reensure()
  assert.equal(first, second)
  await flush()
  assert.equal(h.events.filter((event) => event === "ensure").length, 2)
  release(ensured(SOCKET_B))
  assert.equal(await first, true)
  assert.deepEqual(session.state(), { kind: "ready", socket: h.forwardSocket, status: status(SOCKET_B) })
  assert.deepEqual(h.events.slice(-4), ["ensure", "cancel", "forward", `probe ${h.forwardSocket}`])
  assert.equal(h.children.length, 1)
  session.closeSync()
})

test("reensure from ready with the same socket only probes", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  const before = h.events.length
  assert.equal(await session.reensure(), true)
  assert.deepEqual(h.events.slice(before), ["ensure", `probe ${h.forwardSocket}`])
  session.closeSync()
})

test("reensure from ready that cannot reach the daemon reconnects instead of reporting forwardDenied", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  h.probes = ["gone", "gone"]
  assert.equal(await session.reensure(), false)
  const state = session.state()
  assert.ok(state.kind === "reconnecting")
  assert.equal(state.failure.kind, "failed")
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  session.closeSync()
})

test("reensure while reconnecting starts an attempt now", async () => {
  const h = harness()
  h.onSpawn = failMaster(REFUSED)
  const session = createSshConnector(h.deps).connect(target)
  await settle(session)
  assert.equal(session.state().kind, "reconnecting")
  h.onSpawn = () => writeFileSync(h.controlPath, "")
  const retried = session.reensure()
  const joined = session.reensure()
  await flush()
  assert.deepEqual([await retried, await joined], [true, true])
  assert.equal(h.children.length, 2)
  assert.equal(h.fake.pending(), 0)
  session.closeSync()
})

test("close runs -O exit, unlinks the forward socket and ControlPath, and stops everything", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  assert.equal(existsSync(h.forwardSocket), true)
  await session.close()
  const exit = h.calls.at(-1)
  assert.deepEqual(exit?.args, muxControlArgs(target, h.controlPath, { op: "exit" }))
  assert.equal(exit?.timeoutMs, EXIT_TIMEOUT_MS)
  assert.equal(h.children[0]?.exited(), true)
  assert.deepEqual(h.children[0]?.signals, [])
  assert.equal(existsSync(h.forwardSocket), false)
  assert.equal(existsSync(h.controlPath), false)
  // The master exit after close does not reconnect.
  assert.equal(session.state().kind, "ready")
  assert.equal(h.fake.pending(), 0)
  assert.equal(await session.reensure(), false)
  assert.equal(h.children.length, 1)
})

test("close SIGTERMs a master that -O exit did not stop", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  h.handlers.exit = () => ({ code: 255, stdout: "", stderr: "mux_client_request_session: read from master failed\r\n" })
  await session.close()
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(existsSync(h.forwardSocket), false)
})

test("close while reconnecting cancels the retry timer", async () => {
  const h = harness()
  h.onSpawn = failMaster(REFUSED)
  const session = createSshConnector(h.deps).connect(target)
  await settle(session)
  assert.equal(h.fake.pending(), 1)
  await session.close()
  assert.equal(h.fake.pending(), 0)
  await advance(h.fake, 60_000, 10_000)
  assert.equal(h.children.length, 1)
})

test("close during the ControlPath wait ends the attempt and spawns nothing more", async () => {
  const h = harness()
  h.onSpawn = () => {}
  const session = createSshConnector(h.deps).connect(target)
  await advance(h.fake, 300)
  await session.close()
  const failure = failureOf(await session.ready())
  assert.equal(failure.kind, "failed")
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(h.fake.pending(), 0)
  await advance(h.fake, 60_000, 10_000)
  assert.equal(h.children.length, 1)
})

test("closeSync SIGTERMs the master and unlinks synchronously", async () => {
  const h = harness()
  const session = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(session)).ok)
  const runs = h.calls.length
  session.closeSync()
  assert.deepEqual(h.children[0]?.signals, ["SIGTERM"])
  assert.equal(existsSync(h.forwardSocket), false)
  assert.equal(existsSync(h.controlPath), false)
  assert.equal(h.calls.length, runs)
  assert.equal(h.fake.pending(), 0)
})

test("preflight runs once per connector and is shared by sessions", async () => {
  const h = harness()
  const connector = createSshConnector(h.deps)
  const first = connector.connect(target)
  const second = connector.connect({ id: "ssh-other", destination: "other", port: 2222 })
  await flush()
  assert.ok((await first.ready()).ok)
  await second.ready()
  const third = connector.connect({ id: "ssh-third", destination: "third", port: null })
  await settle(third)
  assert.equal(h.events.filter((event) => event === "version").length, 1)
  for (const session of [first, second, third]) session.closeSync()
})

test("preflight: ssh not found is sshMissing and spawns nothing; a later attempt checks again", async () => {
  const h = harness()
  h.handlers.version = () => ({ code: null, stdout: "", stderr: "", error: { code: "ENOENT", message: "spawn /usr/bin/ssh ENOENT", timedOut: false } })
  const connector = createSshConnector(h.deps)
  const session = connector.connect(target)
  const failure = failureOf(await settle(session))
  assert.deepEqual(failure, { kind: "sshMissing", destination: "devbox", detail: "Cannot run /usr/bin/ssh: spawn /usr/bin/ssh ENOENT" })
  assert.equal(session.state().kind, "failed")
  assert.equal(h.children.length, 0)
  delete h.handlers.version
  assert.equal(await session.reensure(), true)
  assert.equal(h.events.filter((event) => event === "version").length, 2)
  session.closeSync()
})

test("preflight: OpenSSH older than 6.7 is sshTooOld; other clients proceed", async () => {
  const h = harness()
  h.handlers.version = () => ({ code: 0, stdout: "", stderr: "OpenSSH_6.6.1p1 Ubuntu-2ubuntu2, OpenSSL 1.0.1f 6 Jan 2014\n" })
  const old = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(old))
  assert.equal(failure.kind, "sshTooOld")
  assert.match(failure.detail, /^OpenSSH_6\.6\.1p1 .* is too old/)
  assert.equal(h.children.length, 0)

  h.handlers.version = () => ({ code: 0, stdout: "", stderr: "Dropbear SSH client v2022.83\n" })
  const other = createSshConnector(h.deps).connect(target)
  assert.ok((await settle(other)).ok)
  other.closeSync()
})

test("a refused runtime directory is a permanent runtimeDir failure", async () => {
  const h = harness({ ok: false, dir: "/run/user/1000/servef-desktop-0123abcd", detail: "it is a symbolic link" })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.deepEqual(failure, {
    kind: "runtimeDir",
    destination: "devbox",
    detail: "Cannot use the private runtime directory /run/user/1000/servef-desktop-0123abcd: it is a symbolic link.",
  })
  assert.equal(session.state().kind, "failed")
  assert.equal(h.children.length, 0)
})

test("a failed -O check is classified and retried", async () => {
  const h = harness()
  h.handlers.check = () => ({ code: 255, stdout: "", stderr: `Control socket connect(${h.controlPath}): Connection refused\r\n` })
  const session = createSshConnector(h.deps).connect(target)
  const failure = failureOf(await settle(session))
  assert.equal(failure.kind, "unreachable")
  assert.equal(session.state().kind, "reconnecting")
  session.closeSync()
})

test("nodeSpawn reports a missing binary through stderr and onExit without throwing", async () => {
  const child = nodeSpawn("/nonexistent/servef-test-ssh", ["-V"], {})
  const stderr: string[] = []
  child.onStderr((chunk) => stderr.push(chunk))
  const exit = await new Promise<[number | null, string | null]>((resolve) => child.onExit((code, signal) => resolve([code, signal])))
  assert.deepEqual(exit, [null, null])
  assert.equal(child.exited(), true)
  assert.match(stderr.join(""), /ENOENT/)
  child.kill("SIGTERM")
})

test("nodeSpawn pipes stderr and reports the exit code", async () => {
  const child = nodeSpawn(process.execPath, ["-e", "process.stderr.write('boom'); process.exit(3)"], process.env)
  const stderr: string[] = []
  child.onStderr((chunk) => stderr.push(chunk))
  const exit = await new Promise<[number | null, string | null]>((resolve) => child.onExit((code, signal) => resolve([code, signal])))
  assert.deepEqual(exit, [3, null])
  assert.equal(stderr.join(""), "boom")
})
