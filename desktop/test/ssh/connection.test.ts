import assert from "node:assert/strict"
import { existsSync, mkdirSync, readdirSync, readFileSync, symlinkSync, writeFileSync } from "node:fs"
import { request } from "node:http"
import path from "node:path"
import { after, before, describe, test, type TestContext } from "node:test"

import { listDirectories, nodeExecFile, openPath, probeDaemon } from "../../src/main/daemon.ts"
import { createForwarder, type Forwarder } from "../../src/main/forward.ts"
import { prepareRuntimeDir, type RuntimeDirResult } from "../../src/main/runtime-dir.ts"
import { muxControlArgs } from "../../src/main/ssh-args.ts"
import { createSshConnector, nodeSpawn, type SshReady, type SshSession, type SshTarget } from "../../src/main/ssh-connection.ts"
import type { SshFailure, SshFailureKind } from "../../src/main/ssh-errors.ts"
import { systemTimers } from "../../src/main/timers.ts"
import { accepts, REQUIRED, rootSkip, sshBinary, sshSkip, startSshd, type Sshd } from "./sshd.ts"

// The real connector (spawn, execFile, probe, timers, runtime dir) against a
// private sshd (decision 0070): connect, re-ensure, reconnect, every failure
// kind within 30 s, and teardown that leaves no files or ssh processes.

const LIMIT_MS = 30_000

interface Opened {
  target: SshTarget
  session: SshSession
  runtime: RuntimeDirResult
  forwarder: Forwarder
}

function within<T>(promise: Promise<T>, ms: number, what: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_resolve, reject) => {
    timer = setTimeout(() => reject(new Error(`${what} did not settle within ${ms} ms`)), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

async function waitFor(check: () => boolean | Promise<boolean>, ms: number, what: string): Promise<void> {
  const deadline = Date.now() + ms
  while (!(await check())) {
    if (Date.now() > deadline) throw new Error(`timed out after ${ms} ms waiting for ${what}`)
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
}

function pidAlive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    return !(error instanceof Error && "code" in error && error.code === "ESRCH")
  }
}

// processesMentioning lists live processes whose argv mentions text (Linux).
function processesMentioning(text: string): number[] {
  if (process.platform !== "linux") return []
  const pids: number[] = []
  for (const entry of readdirSync("/proc")) {
    if (!/^\d+$/.test(entry) || Number(entry) === process.pid) continue
    try {
      if (readFileSync(`/proc/${entry}/cmdline`, "utf8").includes(text)) pids.push(Number(entry))
    } catch {
      // Exited while listing.
    }
  }
  return pids
}

function ready(result: SshReady): { socket: string; pid: number; remote: string } {
  if (!result.ok) assert.fail(`not ready: ${result.failure.kind}: ${result.failure.detail}`)
  return { socket: result.socket, pid: result.status.pid, remote: result.status.socket }
}

function failed(result: SshReady): SshFailure {
  if (result.ok) assert.fail(`connected to ${result.status.socket}; expected a failure`)
  return result.failure
}

interface Reply {
  status: number
  text: string
}

// get sends GET path over the forward socket as the daemon expects (Host
// localhost); a forwarded CLI server takes host localhost:<port>.
function get(socket: string, route: string, host = "localhost"): Promise<Reply> {
  return new Promise((resolve, reject) => {
    const req = request({ socketPath: socket, method: "GET", path: route, headers: { host }, agent: false }, (response) => {
      const chunks: Buffer[] = []
      response.on("data", (chunk: Buffer) => chunks.push(chunk))
      response.on("end", () => resolve({ status: response.statusCode ?? 0, text: Buffer.concat(chunks).toString("utf8") }))
      response.on("error", reject)
    })
    req.on("error", reject)
    req.setTimeout(10_000, () => req.destroy(new Error(`GET ${route} timed out`)))
    req.end()
  })
}

function statusPid(reply: Reply): number {
  const value: unknown = JSON.parse(reply.text)
  if (typeof value !== "object" || value === null || !("pid" in value) || typeof value.pid !== "number") assert.fail(`no pid in ${reply.text}`)
  return value.pid
}

const skip = sshSkip()

describe("ssh connections against a real sshd", { skip }, () => {
  let fixture: Sshd | undefined
  let runs = 0
  let project = ""

  const sshd = (): Sshd => {
    if (fixture === undefined) throw new Error("sshd did not start")
    return fixture
  }

  before(async () => {
    fixture = await startSshd()
    project = path.join(fixture.dir, "projects", "alpha")
    mkdirSync(project, { recursive: true })
    writeFileSync(path.join(project, "README.md"), "# Alpha\n")
    fixture.writeRoots([project])
  })

  after(async () => {
    await fixture?.close()
  })

  const freshRuntime = (): RuntimeDirResult => {
    runs += 1
    return prepareRuntimeDir(path.join(sshd().dir, `r${runs}`))
  }

  const open = (destination: string, runtime: RuntimeDirResult = freshRuntime()): Opened => {
    const forwarder = createForwarder()
    const connector = createSshConnector({
      ssh: sshBinary(),
      config: sshd().config,
      runtime,
      env: { ...process.env, SSH_ASKPASS_REQUIRE: "never" },
      spawn: nodeSpawn,
      run: nodeExecFile,
      probe: (socket) => probeDaemon(forwarder.forward, socket),
      timers: systemTimers,
      exists: existsSync,
    })
    const target: SshTarget = { id: `ssh-${destination}`, destination, port: null }
    return { target, session: connector.connect(target), runtime, forwarder }
  }

  const controlPath = (opened: Opened): string => {
    if (!opened.runtime.ok) throw new Error("no runtime dir")
    return opened.runtime.runtime.controlPath(opened.target.id)
  }

  // masterPid asks the master itself (`ssh -O check`: "Master running (pid=N)").
  const masterPid = async (opened: Opened): Promise<number | null> => {
    if (!opened.runtime.ok) return null
    const outcome = await nodeExecFile(sshBinary(), muxControlArgs(opened.target, controlPath(opened), { op: "check" }), { timeoutMs: 10_000 })
    const match = /Master running \(pid=(\d+)\)/.exec(outcome.stderr)
    return match === null ? null : Number(match[1])
  }

  // closeClean closes the session and checks teardown: the master is gone, no
  // ssh process names the runtime dir, and no c-*, f-*.sock, or p-*.sock file remains.
  const closeClean = async (opened: Opened): Promise<void> => {
    const pid = await masterPid(opened)
    await opened.session.close()
    opened.forwarder.close()
    if (pid !== null) await waitFor(() => !pidAlive(pid), 5_000, `ssh master ${pid} to exit`)
    if (!opened.runtime.ok) return
    const dir = opened.runtime.runtime.dir
    await waitFor(() => processesMentioning(dir).length === 0, 5_000, `ssh processes using ${dir} to exit`)
    assert.deepEqual(readdirSync(dir).filter((name) => name.startsWith("c-") || ((name.startsWith("f-") || name.startsWith("p-")) && name.endsWith(".sock"))), [])
  }

  // scenario closes and checks teardown after body, and adds the sshd log to failures.
  const scenario = async <T>(opened: Opened, body: () => Promise<T>): Promise<T> => {
    try {
      const result = await body()
      await closeClean(opened)
      return result
    } catch (error) {
      await opened.session.close().catch(() => undefined)
      opened.forwarder.close()
      const message = error instanceof Error ? error.message : String(error)
      throw new Error(`${message}\n--- sshd log (tail) ---\n${sshd().log().slice(-4_000)}`, { cause: error })
    }
  }

  const expectFailure = async (destination: string, kind: SshFailureKind, runtime?: RuntimeDirResult): Promise<SshFailure> => {
    const opened = open(destination, runtime)
    return scenario(opened, async () => {
      const failure = failed(await within(opened.session.ready(), LIMIT_MS, `${destination} connect`))
      assert.equal(failure.kind, kind, failure.detail)
      return failure
    })
  }

  test("connects, forwards the remote daemon, and keeps user forwards out of the master", async () => {
    const opened = open("servef-test")
    await scenario(opened, async () => {
      const connected = ready(await within(opened.session.ready(), LIMIT_MS, "servef-test connect"))
      // The daemon runs with the master's SetEnv (its XDG_RUNTIME_DIR).
      assert.equal(connected.remote, sshd().remoteSocket)
      assert.equal(opened.session.state().kind, "ready")

      const status = await get(connected.socket, "/_daemon/status")
      assert.equal(status.status, 200, status.text)
      assert.equal(statusPid(status), connected.pid)
      const projects = await get(connected.socket, "/api/projects")
      assert.equal(projects.status, 200, projects.text)
      assert.ok(projects.text.includes(JSON.stringify(project)), projects.text)

      // ClearAllForwardings: servef-test's configured LocalForward stays unbound.
      assert.equal(await accepts(sshd().localForwardPort), false)
    })
  })

  test("browses remote folders and opens one through the forwarded daemon (decision 0076)", async () => {
    const opened = open("servef-test")
    await scenario(opened, async () => {
      const connected = ready(await within(opened.session.ready(), LIMIT_MS, "servef-test connect"))
      const projects = path.dirname(project)
      const beta = path.join(projects, "beta")
      mkdirSync(beta, { recursive: true })
      const listed = await listDirectories(opened.forwarder.forward, connected.socket, projects)
      if (!listed.ok) assert.fail(`${listed.code}: ${listed.error}`)
      assert.equal(listed.listing.path, projects)
      assert.deepEqual(listed.listing.entries.map((entry) => entry.name), ["alpha", "beta"])
      const missing = await listDirectories(opened.forwarder.forward, connected.socket, path.join(projects, "nope"))
      assert.ok(!missing.ok && missing.code === "pathUnavailable" && !missing.upgrade)

      const added = await openPath(opened.forwarder.forward, connected.socket, beta)
      if (!added.ok) assert.fail(`${added.code}: ${added.error}`)
      assert.equal(added.result.project, beta)
      const roots = await get(connected.socket, "/_daemon/status")
      assert.ok(roots.text.includes(JSON.stringify(beta)), roots.text)
    })
  })

  test("lists remote servers and forwards a port socket to one (decision 0076)", async () => {
    const opened = open("servef-test")
    const server = await sshd().startServer(project)
    try {
      await scenario(opened, async () => {
        ready(await within(opened.session.ready(), LIMIT_MS, "servef-test connect"))
        const listed = await within(opened.session.listServers(), LIMIT_MS, "list servers")
        if (!listed.ok) assert.fail(listed.detail)
        const entry = listed.servers.find((candidate) => candidate.port === server.port)
        assert.deepEqual(entry, { host: "127.0.0.1", port: server.port, origin: `http://127.0.0.1:${server.port}` })

        const id = `${opened.target.id}-${server.port}`
        const forwarded = await within(opened.session.forwardPort(id, "127.0.0.1", server.port), LIMIT_MS, "forward port")
        if (!forwarded.ok) assert.fail(forwarded.detail)
        const reply = await get(forwarded.socket, "/api/projects", `localhost:${server.port}`)
        assert.equal(reply.status, 200, reply.text)
        assert.ok(reply.text.includes(JSON.stringify(project)), reply.text)

        await opened.session.cancelPort(id)
        assert.equal(existsSync(forwarded.socket), false)
        // Forwarded again, then left for close to clean up.
        const again = await opened.session.forwardPort(id, "127.0.0.1", server.port)
        assert.ok(again.ok)
      })
    } finally {
      await server.close()
    }
  })

  test("a stopped remote daemon is ensured again", async () => {
    const opened = open("servef-test")
    await scenario(opened, async () => {
      const first = ready(await within(opened.session.ready(), LIMIT_MS, "servef-test connect"))
      assert.equal(await opened.session.stopRemoteDaemon(), true)
      await waitFor(async () => (await probeDaemon(opened.forwarder.forward, first.socket)) === "gone", 10_000, "the stopped daemon to stop answering")

      assert.equal(await within(opened.session.reensure(), LIMIT_MS, "reensure"), true)
      const state = opened.session.state()
      assert.equal(state.kind, "ready")
      const status = await get(first.socket, "/_daemon/status")
      assert.equal(status.status, 200, status.text)
      assert.notEqual(statusPid(status), first.pid)
    })
  })

  test("a killed master reconnects", async () => {
    const opened = open("servef-test")
    await scenario(opened, async () => {
      const first = ready(await within(opened.session.ready(), LIMIT_MS, "servef-test connect"))
      const pid = await masterPid(opened)
      if (pid === null) assert.fail("ssh -O check found no master")
      const kinds: string[] = []
      const back = new Promise<void>((resolve) => {
        opened.session.onState((state) => {
          kinds.push(state.kind)
          if (state.kind === "ready") resolve()
        })
      })
      process.kill(pid, "SIGTERM")
      await within(back, LIMIT_MS, "reconnect")
      assert.equal(kinds[0], "reconnecting")
      const next = await masterPid(opened)
      assert.ok(next !== null && next !== pid, `master ${next} after ${pid}`)
      assert.equal(await probeDaemon(opened.forwarder.forward, first.socket), "ok")
    })
  })

  test("an unknown host key fails as hostKey", async () => {
    await expectFailure("servef-hostkey", "hostKey")
  })

  test("a rejected key fails as auth", async () => {
    await expectFailure("servef-nokey", "auth")
  })

  test("a refused connection fails as unreachable", async () => {
    await expectFailure("servef-refused", "unreachable")
  })

  test("a host without servef fails as servefMissing with its os and arch", async (t: TestContext) => {
    if (await sshd().remoteHasServef("servef-nosrv")) {
      if (REQUIRED) assert.fail("servef-nosrv finds servef on its PATH")
      t.skip("the current user's remote PATH has servef")
      return
    }
    const failure = await expectFailure("servef-nosrv", "servefMissing")
    if (process.platform === "linux") assert.equal(failure.os, "Linux")
    assert.ok(failure.arch !== undefined && failure.arch !== "", "arch")
  })

  test("a server that refuses socket forwarding fails as forwardDenied", { skip: rootSkip() }, async () => {
    await expectFailure("servef-nofwd", "forwardDenied")
  })

  test("a symlinked runtime dir fails as runtimeDir", async () => {
    const real = path.join(sshd().dir, "rtarget")
    const link = path.join(sshd().dir, "rlink")
    mkdirSync(real, { mode: 0o700 })
    symlinkSync(real, link)
    const runtime = prepareRuntimeDir(link)
    assert.equal(runtime.ok, false)
    await expectFailure("servef-test", "runtimeDir", runtime)
    assert.deepEqual(readdirSync(real), [])
  })
})
