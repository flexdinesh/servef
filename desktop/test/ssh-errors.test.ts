import assert from "node:assert/strict"
import { test } from "node:test"
import { classifySsh, parseServefMissing, permanent } from "../src/main/ssh-errors.ts"
import type { SshFailureKind, SshPhase } from "../src/main/ssh-errors.ts"

// Real OpenSSH stderr captured by the B0 spike (OpenSSH 10.6p1 client, 9.2p1 and 10.6p1 servers).
const HOST_KEY_CHANGED = [
  "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\r",
  "@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\r",
  "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\r",
  "IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!\r",
  "Someone could be eavesdropping on you right now (man-in-the-middle attack)!\r",
  "It is also possible that a host key has just been changed.\r",
  "The fingerprint for the ED25519 key sent by the remote host is",
  "SHA256:7DqBFh1yc/khEk5jqlFw7deuDhnzjz6dQXl0PObfBTA.\r",
  "Please contact your system administrator.\r",
  "Add correct host key in /tmp/m4b.DRVd0r/known_hosts_changed to get rid of this message.\r",
  "Offending ED25519 key in /tmp/m4b.DRVd0r/known_hosts_changed:1\r",
  "Host key for [127.0.0.1]:47343 has changed and you have requested strict checking.\r",
  "Host key verification failed.\r",
  "",
].join("\n")
const HOST_KEY_UNKNOWN = "No ED25519 host key is known for [127.0.0.1]:47343 and you have requested strict checking.\r\nHost key verification failed.\r\n"
const NO_KEY = "dee@127.0.0.1: Permission denied (publickey).\r\n"
const REFUSED = "ssh: connect to host 127.0.0.1 port 35983: Connection refused\r\n"
const RESOLVE = "ssh: Could not resolve hostname no-such-host.invalid: Name or service not known\r\n"
const NO_ROUTE = "ssh: connect to host 10.255.255.1 port 22: No route to host\r\n"
const DEAD_MASTER_SESSION = "Connection closed by UNKNOWN port 65535\r\n"

function classify(phase: SshPhase, code: number | null, stderr: string, masterStderr = ""): SshFailureKind {
  return classifySsh({ phase, code, stderr, masterStderr })
}

test("host key failures", () => {
  assert.equal(classify("master", 255, HOST_KEY_CHANGED), "hostKey")
  assert.equal(classify("master", 255, HOST_KEY_UNKNOWN), "hostKey")
  assert.equal(classify("master", 255, "No ECDSA host key is known for devbox and you have requested strict checking.\n"), "hostKey")
})

test("authentication failures", () => {
  assert.equal(classify("master", 255, NO_KEY), "auth")
  assert.equal(classify("master", 255, "Received disconnect from 10.0.0.5 port 22:2: Too many authentication failures\n"), "auth")
})

test("unreachable hosts", () => {
  assert.equal(classify("master", 255, REFUSED), "unreachable")
  assert.equal(classify("master", 255, RESOLVE), "unreachable")
  assert.equal(classify("master", 255, NO_ROUTE), "unreachable")
  assert.equal(classify("master", 255, "ssh: connect to host 10.0.0.5 port 22: Connection timed out\n"), "unreachable")
  assert.equal(classify("master", 255, "ssh: connect to host 10.0.0.5 port 22: Network is unreachable\n"), "unreachable")
})

test("a dropped connection is unreachable only while the master connects", () => {
  assert.equal(classify("master", 255, "Connection closed by 10.0.0.5 port 22\n"), "unreachable")
  assert.equal(classify("master", 255, "kex_exchange_identification: read: Connection reset by peer\n"), "unreachable")
  assert.equal(classify("forward", 255, DEAD_MASTER_SESSION), "failed")
})

test("master phase with no exit code and no stderr is the ControlPath wait timeout", () => {
  assert.equal(classify("master", null, ""), "unreachable")
  assert.equal(classify("master", null, "  \n"), "unreachable")
  assert.equal(classify("master", null, "something odd\n"), "failed")
  assert.equal(classify("master", 255, ""), "failed")
})

test("servef missing on the remote", () => {
  assert.equal(classify("ensure", 127, "servef-missing Linux x86_64\n"), "servefMissing")
  assert.equal(classify("ensure", 127, "bash: line 1: servef: command not found\n"), "failed")
  assert.equal(classify("ensure", 1, "servef-missing Linux x86_64\n"), "failed")
})

test("servef too old on the remote", () => {
  assert.equal(classify("ensure", 1, "Error: unknown command \"daemon\" for \"servef\"\nRun 'servef --help' for usage.\n"), "servefTooOld")
  assert.equal(classify("ensure", 1, "Error: unknown flag: --json\n"), "servefTooOld")
  assert.equal(classify("ensure", 1, "Error: unknown shorthand flag: 'j' in -j\n"), "servefTooOld")
  assert.equal(classify("ensure", 0, "unknown flag: --json"), "failed")
})

test("mux failures fall back to the master's stderr for connection kinds", () => {
  assert.equal(classify("ensure", 255, DEAD_MASTER_SESSION, "Connection closed by 10.0.0.5 port 22\n"), "unreachable")
  assert.equal(classify("forward", 255, "mux_client_forward: forwarding request failed: Port forwarding failed\n", ""), "failed")
})

test("forwardDenied is never decided from stderr", () => {
  const denied = "channel 1: open failed: connect failed: open failed\n"
  for (const phase of ["master", "ensure", "forward", "probe"] as const) {
    assert.notEqual(classify(phase, 255, denied, denied), "forwardDenied")
    assert.notEqual(classify(phase, 255, "administratively prohibited", "administratively prohibited"), "forwardDenied")
  }
})

test("preflight: no exit code means ssh could not run; old OpenSSH is too old", () => {
  assert.equal(classify("preflight", null, ""), "sshMissing")
  assert.equal(classify("preflight", 0, "OpenSSH_6.6, OpenSSL 1.0.1f 6 Jan 2014\n"), "sshTooOld")
  assert.equal(classify("preflight", 0, "OpenSSH_5.9p1\n"), "sshTooOld")
  assert.equal(classify("preflight", 0, "OpenSSH_6.7p1\n"), "failed")
  assert.equal(classify("preflight", 0, "Dropbear\n"), "failed")
})

test("anything else is failed", () => {
  assert.equal(classify("ensure", 1, "daemon did not start; see /home/dee/.cache/servef/daemon.sock.log\n"), "failed")
  assert.equal(classify("probe", null, ""), "failed")
})

test("permanent and transient kinds", () => {
  for (const kind of ["sshMissing", "sshTooOld", "hostKey", "auth", "servefMissing", "servefTooOld", "mismatch", "forwardDenied", "runtimeDir"] as const) {
    assert.equal(permanent(kind), true, kind)
  }
  assert.equal(permanent("unreachable"), false)
  assert.equal(permanent("failed"), false)
})

test("parseServefMissing", () => {
  assert.deepEqual(parseServefMissing("servef-missing Linux x86_64\n"), { os: "Linux", arch: "x86_64" })
  assert.deepEqual(parseServefMissing("motd line\nservef-missing Darwin arm64\r\n"), { os: "Darwin", arch: "arm64" })
  assert.equal(parseServefMissing("servef: command not found\n"), null)
  assert.equal(parseServefMissing("servef-missing Linux\n"), null)
  assert.equal(parseServefMissing("servef-missing <script> x\n"), null)
})
