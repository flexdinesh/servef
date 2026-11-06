import { spawn, type ChildProcess } from "node:child_process"

export interface ManagedProcess {
  child: ChildProcess
  exited: Promise<number>
  ownsProcessGroup: boolean
}

// `output`, a file descriptor, receives both stdout and stderr instead.
export function startProcess(
  command: string, args: string[], env: NodeJS.ProcessEnv, capture = false, output?: number,
  ownership: "group" | "child" = "group",
): ManagedProcess {
  const ownsProcessGroup = ownership === "group" && process.platform !== "win32"
  const child = spawn(command, args, {
    env,
    detached: ownsProcessGroup,
    stdio: output === undefined ? ["ignore", capture ? "pipe" : "inherit", "inherit"] : ["ignore", output, output],
  })
  if (capture) child.stdout?.pipe(process.stdout, { end: false })
  const exited = new Promise<number>((resolve, reject) => {
    child.once("error", reject)
    child.once("close", (code) => resolve(code ?? 1))
  })
  // A server can fail before its caller awaits readiness or cleanup.
  void exited.catch(() => {})
  return { child, exited, ownsProcessGroup }
}

function terminate(proc: ManagedProcess, signal: NodeJS.Signals) {
  const { child } = proc
  if (child.pid === undefined) return
  try {
    if (!proc.ownsProcessGroup) child.kill(signal)
    else process.kill(-child.pid, signal)
  } catch (error) {
    if (!(error instanceof Error && "code" in error && error.code === "ESRCH")) throw error
  }
}

export async function stopProcess(proc: ManagedProcess): Promise<void> {
  terminate(proc, "SIGTERM")
  const force = setTimeout(() => terminate(proc, "SIGKILL"), 5_000)
  try {
    await proc.exited.catch(() => {})
    // A launcher may exit before its descendants. Once it has finished,
    // terminate anything still in its process group before removing fixtures.
    if (proc.ownsProcessGroup) terminate(proc, "SIGKILL")
  } finally {
    clearTimeout(force)
  }
}

// Both srcz and Vite print their bound loopback URL after listening. Keeping
// that listener open avoids the race in probing and then releasing a free port.
export async function serverPort(proc: ManagedProcess): Promise<number> {
  return new Promise<number>((resolve, reject) => {
    let output = ""
    const timeout = setTimeout(() => finish(new Error("Browser server did not report its URL within 120 seconds")), 120_000)
    function finish(result: number | Error) {
      clearTimeout(timeout)
      proc.child.stdout?.off("data", read)
      if (result instanceof Error) reject(result)
      else resolve(result)
    }
    function read(chunk: Buffer) {
      output = (output + chunk.toString()).slice(-8_192)
      const match = /http:\/\/(?:127\.0\.0\.1|localhost):(\d+)\//.exec(output)
      if (match) finish(Number(match[1]))
    }
    proc.child.stdout?.on("data", read)
    void proc.exited.then(
      (code) => finish(new Error(`Browser server exited before readiness (status ${code})`)),
      (error: unknown) => finish(error instanceof Error ? error : new Error(String(error))),
    )
  })
}

export function requestedPort(value: string | undefined, name: string, max = 65535): number {
  if (value === undefined || value === "") return 0
  const port = Number(value)
  if (!Number.isInteger(port) || port < 0 || port > max) throw new Error(`${name} must be a port number from 0 to ${max}`)
  return port
}
