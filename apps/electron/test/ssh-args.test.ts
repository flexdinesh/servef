import assert from "node:assert/strict"
import { test } from "node:test"
import {
  masterArgs, muxCommandArgs, muxControlArgs, parseSshG, parseVersion, portForwardSpec, REMOTE_ENSURE, REMOTE_LIST, REMOTE_STOP, sshGArgs, validDestination, validRemoteSocket,
} from "../src/main/ssh-args.ts"

const target = { id: "ssh-devbox", destination: "devbox", port: null }
const cp = "/run/user/1000/servef-electron-0123abcd/c-0123456789ab"

test("REMOTE_ENSURE and REMOTE_STOP are the exact remote command lines", () => {
  const prefix = "sh -c 'PATH=\"$PATH:$HOME/.local/bin:${GOPATH:-$HOME/go}/bin:$HOME/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:/usr/local/bin\"; export PATH; command -v servef >/dev/null 2>&1 || { echo \"servef-missing $(uname -s) $(uname -m)\" >&2; exit 127; }; "
  assert.equal(REMOTE_ENSURE, `${prefix}exec servef daemon ensure --json'`)
  assert.equal(REMOTE_STOP, `${prefix}exec servef daemon stop'`)
  assert.equal(REMOTE_LIST, `${prefix}exec servef control list --json'`)
})

test("remote commands pass through every login shell: no quote or ! inside the sh -c string", () => {
  for (const command of [REMOTE_ENSURE, REMOTE_STOP, REMOTE_LIST]) {
    assert.ok(command.startsWith("sh -c '") && command.endsWith("'"))
    const inner = command.slice("sh -c '".length, -1)
    assert.equal(inner.includes("'"), false)
    assert.equal(inner.includes("!"), false)
  }
})

test("masterArgs is the app-owned ControlMaster argv with the destination after --", () => {
  assert.deepEqual(masterArgs(target, cp, null), [
    "-M", "-N",
    "-o", "ControlMaster=yes", "-o", `ControlPath=${cp}`, "-o", "ControlPersist=no",
    "-o", "BatchMode=yes", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3",
    "-o", "ClearAllForwardings=yes", "-o", "StreamLocalBindUnlink=yes", "-o", "StreamLocalBindMask=0177",
    "-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none",
    "-o", "RequestTTY=no", "-o", "LogLevel=ERROR",
    "--", "devbox",
  ])
})

test("masterArgs adds -F and -p before --", () => {
  const args = masterArgs({ id: "ssh-dee-10-0-0-5", destination: "dee@10.0.0.5", port: 2222 }, cp, "/tmp/ssh_config")
  assert.deepEqual(args.slice(-7), ["LogLevel=ERROR", "-F", "/tmp/ssh_config", "-p", "2222", "--", "dee@10.0.0.5"])
  assert.equal(args.filter((arg) => arg === "--").length, 1)
})

const muxOptions = ["-o", "ControlMaster=no", "-o", `ControlPath=${cp}`, "-o", "BatchMode=yes", "-o", "ProxyCommand=false", "-F", "none"]

test("muxControlArgs: check and exit never use the user's config", () => {
  assert.deepEqual(muxControlArgs(target, cp, { op: "check" }), ["-O", "check", ...muxOptions, "--", "devbox"])
  assert.deepEqual(muxControlArgs(target, cp, { op: "exit" }), ["-O", "exit", ...muxOptions, "--", "devbox"])
})

test("muxControlArgs: forward and cancel carry -L local:remote", () => {
  const local = "/run/user/1000/servef-electron-0123abcd/f-0123456789ab.sock"
  const remote = "/home/dee/.cache/servef/daemon.sock"
  assert.deepEqual(muxControlArgs(target, cp, { op: "forward", local, remote }), ["-O", "forward", "-L", `${local}:${remote}`, ...muxOptions, "--", "devbox"])
  assert.deepEqual(muxControlArgs({ ...target, port: 2222 }, cp, { op: "cancel", local, remote }), ["-O", "cancel", "-L", `${local}:${remote}`, ...muxOptions, "-p", "2222", "--", "devbox"])
})

test("muxCommandArgs: -n -T, mux options, --, destination, then the command as one element", () => {
  assert.deepEqual(muxCommandArgs(target, cp, REMOTE_ENSURE), ["-n", "-T", ...muxOptions, "--", "devbox", REMOTE_ENSURE])
  const withPort = muxCommandArgs({ ...target, port: 2200 }, cp, REMOTE_STOP)
  assert.deepEqual(withPort.slice(-5), ["-p", "2200", "--", "devbox", REMOTE_STOP])
})

test("sshGArgs", () => {
  assert.deepEqual(sshGArgs("devbox", null), ["-G", "--", "devbox"])
  assert.deepEqual(sshGArgs("devbox", "/cfg"), ["-G", "-F", "/cfg", "--", "devbox"])
})

test("validDestination accepts aliases, user@host, and IPv6 literals", () => {
  for (const ok of ["devbox", "Devbox_2", "work-box", "dee@devbox", "dee@10.0.0.5", "dee@2001:db8::5", "2001:db8::5", "::1", "host.example.com", "a.b-c_d", "_svc@x", "h".repeat(253), `${"u".repeat(64)}@h`]) {
    assert.equal(validDestination(ok), true, ok)
  }
})

test("validDestination rejects option-like, quoted, spaced, and path-like strings", () => {
  for (const bad of [
    "", "-oProxyCommand=x", "dee@-oProxyCommand=x", "-dee@host", "dee@", "@host", "dev box", "dev\tbox", "dev\nbox", "host%eth0", "fe80::1%eth0",
    "\"devbox\"", "'devbox'", "dev/box", "dee@dev/box", "a@b@c", "dee@[::1]", "h".repeat(254), `${"u".repeat(65)}@h`, "dev box é", "2001:db8::zz", "dee:pw@host",
  ]) {
    assert.equal(validDestination(bad), false, JSON.stringify(bad))
  }
})

test("validRemoteSocket", () => {
  assert.equal(validRemoteSocket("/home/dee/.cache/servef/daemon.sock"), true)
  assert.equal(validRemoteSocket(`/${"a".repeat(106)}`), true)
  for (const bad of ["relative/daemon.sock", "", `/${"a".repeat(107)}`, "/tmp/a:b.sock", "/tmp/a b.sock", "/tmp/a\tb", "/tmp/a\u0001b", "/tmp/a\u007fb", "/tmp/100%.sock", "/tmp/a b"]) {
    assert.equal(validRemoteSocket(bad), false, JSON.stringify(bad))
  }
  // 107 bytes counted in UTF-8, not characters.
  assert.equal(validRemoteSocket(`/${"é".repeat(54)}`), false)
})

test("parseVersion reads OpenSSH major and minor", () => {
  assert.deepEqual(parseVersion("OpenSSH_6.6, OpenSSL 1.0.1f 6 Jan 2014\n"), { kind: "openssh", major: 6, minor: 6 })
  assert.deepEqual(parseVersion("OpenSSH_9.2p1 Debian-2+deb12u3, OpenSSL 3.0.15 3 Sep 2024\n"), { kind: "openssh", major: 9, minor: 2 })
  assert.deepEqual(parseVersion("OpenSSH_10.6p1, OpenSSL 3.5.0 8 Apr 2025\n"), { kind: "openssh", major: 10, minor: 6 })
  assert.deepEqual(parseVersion("Dropbear SSH client v2022.83\n"), { kind: "other" })
  assert.deepEqual(parseVersion(""), { kind: "other" })
})

test("parseSshG reads hostname, user, and port", () => {
  const stdout = "host devbox\nuser dee\nhostname 10.0.0.5\nport 2222\nidentityfile ~/.ssh/id_ed25519\n"
  assert.deepEqual(parseSshG(stdout), { hostname: "10.0.0.5", user: "dee", port: 2222 })
  assert.deepEqual(parseSshG("user dee\r\nhostname devbox\r\nport 22\r\n"), { hostname: "devbox", user: "dee", port: 22 })
})

test("parseSshG rejects missing or invalid fields", () => {
  assert.equal(parseSshG("user dee\nport 22\n"), null)
  assert.equal(parseSshG("hostname h\nport 22\n"), null)
  assert.equal(parseSshG("hostname h\nuser dee\n"), null)
  assert.equal(parseSshG("hostname h\nuser dee\nport 0\n"), null)
  assert.equal(parseSshG("hostname h\nuser dee\nport 70000\n"), null)
  assert.equal(parseSshG("hostname h\nuser dee\nport abc\n"), null)
})

test("portForwardSpec accepts IP literals and localhost with a valid port only", () => {
  assert.equal(portForwardSpec("127.0.0.1", 7971), "127.0.0.1:7971")
  assert.equal(portForwardSpec("localhost", 1), "localhost:1")
  assert.equal(portForwardSpec("10.0.0.5", 65_535), "10.0.0.5:65535")
  assert.equal(portForwardSpec("::1", 7971), "[::1]:7971")
  assert.equal(portForwardSpec("[2001:db8::5]", 7971), "[2001:db8::5]:7971")
  for (const [host, port] of [
    ["box.lan", 7971], ["127.0.0.1", 0], ["127.0.0.1", 65_536], ["127.0.0.1", 1.5], ["-oProxyCommand=x", 1], ["127.0.0.1:22", 1],
    ["fe80::1%eth0", 1], ["", 1], ["127.0.0.1 ", 1], ["LOCALHOST", 1],
  ] as const) assert.equal(portForwardSpec(host, port), null, `${host}:${port}`)
})

test("muxControlArgs forwards a local socket to a remote TCP server", () => {
  const spec = portForwardSpec("127.0.0.1", 7971) ?? ""
  assert.deepEqual(muxControlArgs(target, cp, { op: "forward", local: "/run/p-1.sock", remote: spec }).slice(0, 4), ["-O", "forward", "-L", "/run/p-1.sock:127.0.0.1:7971"])
  assert.deepEqual(muxCommandArgs(target, cp, REMOTE_LIST).slice(-2), ["devbox", REMOTE_LIST])
})
