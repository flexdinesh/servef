import { execFile, execFileSync, spawn, type ChildProcess } from "node:child_process"
import { chmodSync, chownSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs"
import { connect, createServer, type Server } from "node:net"
import { tmpdir, userInfo } from "node:os"
import path from "node:path"

import { findOnPath } from "../../src/main/binary.ts"
import { REMOTE_PATH_DIRS } from "../../src/main/ssh-args.ts"

// A private sshd on 127.0.0.1 for the ssh integration tests and the remote
// Electron smoke (decisions 0070, 0076). Each run owns one temp dir: host and client
// keys, known hosts, sshd and client configs, the remote servef binary, and the
// remote XDG directories. Remote env comes only from the master's SetEnv, so
// every env variant is its own alias. Nothing reads the user's ssh config,
// agent, keys, or daemon.

// SERVEF_DESKTOP_SSH_TESTS=require turns every skip into a failure (Dagger).
export const REQUIRED = process.env.SERVEF_DESKTOP_SSH_TESTS === "require"

const SSHD = "/usr/sbin/sshd"
// SECOND_USER has servef only through its alias's GOPATH and may not forward
// Unix sockets (root runs only).
const SECOND_USER = "sfnofwd"

export const isRoot = process.getuid?.() === 0

// sshBinary is the client the app would pick: ssh on PATH, else /usr/bin/ssh.
export function sshBinary(): string {
  return findOnPath("ssh", process.env) ?? "/usr/bin/ssh"
}

// sshSkip is the reason to skip real-sshd tests, or false to run them. When
// required, missing tools fail in startSshd instead.
export function sshSkip(): string | false {
  if (REQUIRED) return false
  if (process.platform === "win32") return "no OpenSSH server on Windows"
  if (!existsSync(SSHD)) return `${SSHD} is missing`
  if (!existsSync(sshBinary())) return "ssh is missing"
  if (keygenBinary() === null) return "ssh-keygen is missing"
  return false
}

// rootSkip is the reason to skip cases that need a second user, or false.
export function rootSkip(): string | false {
  if (isRoot || REQUIRED) return false
  return "needs root to create a second sshd user"
}

function keygenBinary(): string | null {
  return findOnPath("ssh-keygen", process.env) ?? (existsSync("/usr/bin/ssh-keygen") ? "/usr/bin/ssh-keygen" : null)
}

export function servefBinary(): string {
  const binary = process.env.SERVEF_DESKTOP_BINARY
  if (binary === undefined || binary === "") throw new Error("SERVEF_DESKTOP_BINARY must name a servef binary built from this checkout")
  const resolved = path.resolve(binary)
  if (!existsSync(resolved)) throw new Error(`SERVEF_DESKTOP_BINARY does not exist: ${resolved}`)
  return resolved
}

// RemoteEnv is the SetEnv one alias's master sends.
export interface RemoteEnv {
  gopath: string
  runtime: string
  config: string
  cache: string
}

export interface RemoteServer {
  port: number
  close(): Promise<void>
}

export interface SshResult {
  code: number | null
  stdout: string
  stderr: string
}

export interface Sshd {
  dir: string
  port: number
  // config is the client ssh_config (SERVEF_DESKTOP_SSH_CONFIG).
  config: string
  user: string
  // remote is servef-test's environment; its daemon socket is remoteSocket.
  remote: RemoteEnv
  remoteSocket: string
  // remoteServef is the binary servef-test reaches through GOPATH.
  remoteServef: string
  // localForwardPort is servef-test's configured LocalForward; the app's
  // master must never bind it.
  localForwardPort: number
  // nofwd is the forward-denied alias's daemon socket (root runs), else null.
  nofwdSocket: string | null
  // writeRoots sets servef-test's remote roots before its daemon starts.
  writeRoots(roots: readonly string[]): void
  // ssh runs a command on alias with the test config, no mux, no forwards.
  ssh(alias: string, command: string, timeoutMs?: number): Promise<SshResult>
  // remoteHasServef reports whether alias finds servef on the remote PATH
  // with the app's fallback directories.
  remoteHasServef(alias: string): Promise<boolean>
  // startServer runs `servef browser` for root on 127.0.0.1 in servef-test's
  // remote environment, so REMOTE_LIST over servef-test lists it (decision
  // 0076); close stops it.
  startServer(root: string): Promise<RemoteServer>
  log(): string
  close(): Promise<void>
}

// <base>/sfs-XXXXXX/<...>/daemon.sock and ControlPaths must fit sun_path.
function shortTempBase(): string {
  const base = realpathSync(tmpdir())
  return base.length <= 40 ? base : "/tmp"
}

function freePorts(count: number): Promise<number[]> {
  const servers: Server[] = []
  const listen = (): Promise<number> => new Promise((resolve, reject) => {
    const server = createServer()
    servers.push(server)
    server.once("error", reject)
    server.listen(0, "127.0.0.1", () => {
      const address = server.address()
      if (address === null || typeof address === "string") reject(new Error("no TCP address"))
      else resolve(address.port)
    })
  })
  return Promise.all(Array.from({ length: count }, listen)).finally(() => {
    for (const server of servers) server.close()
  })
}

// accepts resolves true when something accepts TCP connections on port.
export function accepts(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = connect({ host: "127.0.0.1", port })
    socket.once("connect", () => {
      socket.destroy()
      resolve(true)
    })
    socket.once("error", () => resolve(false))
  })
}

function run(file: string, args: readonly string[], timeoutMs: number): Promise<SshResult> {
  return new Promise((resolve) => {
    const child = execFile(file, [...args], { timeout: timeoutMs, encoding: "utf8", shell: false }, (error, stdout, stderr) => {
      if (error === null) resolve({ code: 0, stdout, stderr })
      else resolve({ code: typeof error.code === "number" ? error.code : null, stdout, stderr: stderr || error.message })
    })
    child.stdin?.end()
  })
}

function keygen(file: string): void {
  const binary = keygenBinary()
  if (binary === null) throw new Error("ssh-keygen is missing")
  execFileSync(binary, ["-q", "-t", "ed25519", "-N", "", "-C", "servef-test", "-f", file], { stdio: "ignore" })
}

function makeDir(directory: string, mode: number, owner: { uid: number; gid: number } | null = null): string {
  mkdirSync(directory, { recursive: true, mode })
  chmodSync(directory, mode)
  if (owner !== null) chownSync(directory, owner.uid, owner.gid)
  return directory
}

function remoteEnv(base: string, gopath: string, owner: { uid: number; gid: number } | null): RemoteEnv {
  makeDir(base, 0o755, owner)
  return {
    gopath,
    runtime: makeDir(path.join(base, "rt"), 0o700, owner),
    config: makeDir(path.join(base, "config"), 0o700, owner),
    cache: makeDir(path.join(base, "cache"), 0o700, owner),
  }
}

function setEnv(env: RemoteEnv): string {
  return `  SetEnv GOPATH=${env.gopath} XDG_RUNTIME_DIR=${env.runtime} XDG_CONFIG_HOME=${env.config} XDG_CACHE_HOME=${env.cache}`
}

// ensureSecondUser creates SECOND_USER once per machine (root only). A
// useradd user without a password is locked under UsePAM no: "*" unlocks
// key logins without allowing passwords.
function ensureSecondUser(): { uid: number; gid: number } {
  const lookup = (flag: string): number => Number(execFileSync("id", [flag, SECOND_USER], { encoding: "utf8" }).trim())
  try {
    return { uid: lookup("-u"), gid: lookup("-g") }
  } catch {
    execFileSync("useradd", ["-m", "-s", "/bin/sh", SECOND_USER], { stdio: "ignore" })
  }
  execFileSync("usermod", ["-p", "*", SECOND_USER], { stdio: "ignore" })
  return { uid: lookup("-u"), gid: lookup("-g") }
}

function waitForPort(port: number, child: ChildProcess, output: () => string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs
  return new Promise((resolve, reject) => {
    const attempt = (): void => {
      if (child.exitCode !== null || child.signalCode !== null) {
        reject(new Error(`sshd exited (${child.exitCode ?? child.signalCode}):\n${output()}`))
        return
      }
      void accepts(port).then((up) => {
        if (up) resolve()
        else if (Date.now() > deadline) reject(new Error(`sshd did not listen on ${port} within ${timeoutMs} ms:\n${output()}`))
        else setTimeout(attempt, 50)
      })
    }
    attempt()
  })
}

export async function startSshd(): Promise<Sshd> {
  if (!existsSync(SSHD)) throw new Error(`${SSHD} is missing (SERVEF_DESKTOP_SSH_TESTS=require)`)
  if (!existsSync(sshBinary())) throw new Error("ssh is missing (SERVEF_DESKTOP_SSH_TESTS=require)")
  const binary = servefBinary()
  const dir = mkdtempSync(path.join(shortTempBase(), "sfs-"))
  let child: ChildProcess | null = null
  try {
    const secondUser = isRoot ? ensureSecondUser() : null
    // The second user must traverse the dir and read authorized_keys and servef.
    chmodSync(dir, secondUser === null ? 0o700 : 0o755)
    if (isRoot) mkdirSync("/run/sshd", { recursive: true, mode: 0o755 })
    const user = userInfo().username
    const [port = 0, refusedPort = 0, localForwardPort = 0] = await freePorts(3)
    const file = (name: string) => path.join(dir, name)

    for (const key of ["hostkey", "client", "wrongkey"]) keygen(file(key))
    const publicKey = (name: string) => readFileSync(file(`${name}.pub`), "utf8").trim().split(/\s+/).slice(0, 2).join(" ")
    writeFileSync(file("authorized_keys"), `${publicKey("client")}\n`, { mode: 0o644 })
    writeFileSync(file("known_hosts"), `[127.0.0.1]:${port} ${publicKey("hostkey")}\n`)
    writeFileSync(file("known_hosts_empty"), "")

    const gopath = makeDir(file("go"), 0o755)
    makeDir(path.join(gopath, "bin"), 0o755)
    const remoteServef = path.join(gopath, "bin", "servef")
    copyFileSync(binary, remoteServef)
    chmodSync(remoteServef, 0o755)
    const emptyGopath = makeDir(file("empty-go"), 0o755)

    const remote = remoteEnv(file("remote"), gopath, null)
    const nosrvUser = secondUser === null ? user : SECOND_USER
    const nosrv = remoteEnv(file("nosrv"), emptyGopath, secondUser)
    const nofwd = secondUser === null ? null : remoteEnv(file("nofwd"), gopath, secondUser)

    const sshdConfig = [
      `Port ${port}`,
      "ListenAddress 127.0.0.1",
      `HostKey ${file("hostkey")}`,
      "PidFile none",
      `AuthorizedKeysFile ${file("authorized_keys")}`,
      "StrictModes no",
      "UsePAM no",
      "PasswordAuthentication no",
      "KbdInteractiveAuthentication no",
      "PubkeyAuthentication yes",
      "PermitRootLogin prohibit-password",
      "AllowStreamLocalForwarding yes",
      "AcceptEnv GOPATH XDG_RUNTIME_DIR XDG_CONFIG_HOME XDG_CACHE_HOME",
      "LogLevel VERBOSE",
      ...(secondUser === null ? [] : [`Match User ${SECOND_USER}`, "  AllowStreamLocalForwarding no"]),
    ]
    writeFileSync(file("sshd_config"), `${sshdConfig.join("\n")}\n`)

    // First obtained value wins: specific blocks precede the shared one. The
    // app lists no aliases from patterns, so servef-* stays out of prompts.
    const clientConfig = [
      "Host servef-test",
      `  IdentityFile ${file("client")}`,
      setEnv(remote),
      `  LocalForward 127.0.0.1:${localForwardPort} 127.0.0.1:1`,
      "Host servef-nokey",
      `  IdentityFile ${file("wrongkey")}`,
      "Host servef-hostkey",
      `  IdentityFile ${file("client")}`,
      `  UserKnownHostsFile ${file("known_hosts_empty")}`,
      "Host servef-nosrv",
      `  User ${nosrvUser}`,
      `  IdentityFile ${file("client")}`,
      setEnv(nosrv),
      ...(nofwd === null ? [] : ["Host servef-nofwd", `  User ${SECOND_USER}`, `  IdentityFile ${file("client")}`, setEnv(nofwd)]),
      "Host servef-refused",
      `  Port ${refusedPort}`,
      `  IdentityFile ${file("client")}`,
      "Host servef-*",
      "  HostName 127.0.0.1",
      `  Port ${port}`,
      `  User ${user}`,
      "  IdentitiesOnly yes",
      "  IdentityAgent none",
      `  UserKnownHostsFile ${file("known_hosts")}`,
      "  GlobalKnownHostsFile /dev/null",
      "  StrictHostKeyChecking yes",
    ]
    const config = file("ssh_config")
    writeFileSync(config, `${clientConfig.join("\n")}\n`)

    let output = ""
    const started = spawn(SSHD, ["-D", "-e", "-f", file("sshd_config")], { stdio: ["ignore", "pipe", "pipe"], shell: false })
    child = started
    const collect = (chunk: Buffer) => {
      output = (output + chunk.toString("utf8")).slice(-65_536)
    }
    started.stdout.on("data", collect)
    started.stderr.on("data", collect)
    const exited = new Promise<void>((resolve) => started.once("close", () => resolve()))
    await waitForPort(port, started, () => output, 10_000)

    const ssh = (alias: string, command: string, timeoutMs = 30_000): Promise<SshResult> => run(sshBinary(), [
      "-F", config, "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none",
      "-n", "-T", "--", alias, command,
    ], timeoutMs)

    let closing: Promise<void> | null = null
    return {
      dir,
      port,
      config,
      user,
      remote,
      remoteSocket: path.join(remote.runtime, "servef", "daemon.sock"),
      remoteServef,
      localForwardPort,
      nofwdSocket: nofwd === null ? null : path.join(nofwd.runtime, "servef", "daemon.sock"),
      writeRoots(roots) {
        makeDir(path.join(remote.config, "servef"), 0o700)
        writeFileSync(path.join(remote.config, "servef", "roots.json"), JSON.stringify({ version: 1, roots }))
      },
      ssh,
      async remoteHasServef(alias) {
        const probe = `sh -c 'PATH="$PATH:${REMOTE_PATH_DIRS.join(":")}"; export PATH; command -v servef'`
        return (await ssh(alias, probe)).code === 0
      },
      async startServer(root) {
        const [serverPort = 0] = await freePorts(1)
        const env: NodeJS.ProcessEnv = { ...process.env, XDG_RUNTIME_DIR: remote.runtime, XDG_CONFIG_HOME: remote.config, XDG_CACHE_HOME: remote.cache }
        delete env.SSH_CONNECTION
        delete env.SSH_CLIENT
        delete env.SSH_TTY
        let serverOutput = ""
        const server = spawn(remoteServef, ["browser", "--no-open", "--host", "127.0.0.1", "--port", String(serverPort), root], { env, stdio: ["ignore", "pipe", "pipe"], shell: false })
        const collectServer = (chunk: Buffer) => {
          serverOutput = (serverOutput + chunk.toString("utf8")).slice(-8_192)
        }
        server.stdout.on("data", collectServer)
        server.stderr.on("data", collectServer)
        const stopped = new Promise<void>((resolve) => server.once("close", () => resolve()))
        await waitForPort(serverPort, server, () => serverOutput, 15_000)
        return {
          port: serverPort,
          async close() {
            if (server.exitCode === null && server.signalCode === null) server.kill("SIGTERM")
            await Promise.race([stopped, new Promise((resolve) => setTimeout(resolve, 5_000))])
            if (server.exitCode === null && server.signalCode === null) server.kill("SIGKILL")
          },
        }
      },
      log: () => output,
      close() {
        closing ??= (async () => {
          // Remote daemons were started through ssh; stop them the same way.
          const aliases = nofwd === null ? ["servef-test"] : ["servef-test", "servef-nofwd"]
          await Promise.all(aliases.map((alias) => ssh(alias, `${remoteServef} daemon stop`, 15_000)))
          if (started.exitCode === null && started.signalCode === null) started.kill("SIGTERM")
          await Promise.race([exited, new Promise((resolve) => setTimeout(resolve, 5_000))])
          if (started.exitCode === null && started.signalCode === null) started.kill("SIGKILL")
          rmSync(dir, { recursive: true, force: true })
        })()
        return closing
      },
    }
  } catch (error) {
    child?.kill("SIGKILL")
    rmSync(dir, { recursive: true, force: true })
    throw error
  }
}
