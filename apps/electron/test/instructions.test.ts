import assert from "node:assert/strict"
import { test } from "node:test"
import { failureInstructions, releaseOf, sshFailureInstructions } from "../src/main/instructions.ts"
import type { Instructions } from "../src/main/instructions.ts"
import { REMOTE_PATH_DIRS } from "../src/main/ssh-args.ts"
import type { SshFailure, SshFailureKind } from "../src/main/ssh-errors.ts"

const status = { protocol: 2, version: "2.0.0", pid: 1, socket: "/run/s.sock", roots: [] }

test("missing instructions differ for packaged and dev builds", () => {
  const packaged = failureInstructions({ ok: false, kind: "missing", detail: "no binary" }, true)
  assert.match(packaged.paragraphs.join(" "), /Reinstall the servef app/)
  assert.equal(packaged.stopAndRetry, false)
  const dev = failureInstructions({ ok: false, kind: "missing", detail: "no binary" }, false)
  assert.match(dev.paragraphs.join(" "), /SERVEF_DESKTOP_BINARY/)
  assert.deepEqual(dev.commands, ["mise run dev:electron"])
})

test("mismatch instructions name both protocols and offer stop and retry", () => {
  const instructions = failureInstructions({ ok: false, kind: "mismatch", detail: "x", status }, true)
  const text = instructions.paragraphs.join(" ")
  assert.match(text, /protocol 2/)
  assert.match(text, /protocol 1/)
  assert.match(text, /2\.0\.0/)
  assert.deepEqual(instructions.commands, ["servef daemon stop"])
  assert.equal(instructions.stopAndRetry, true)
})

test("failed instructions point at the daemon log when the socket is known", () => {
  assert.match(failureInstructions({ ok: false, kind: "failed", detail: "boom", status }, true).paragraphs.join(" "), /\/run\/s\.sock\.log/)
  const generic = failureInstructions({ ok: false, kind: "failed", detail: "timed out" }, true)
  assert.equal(generic.paragraphs[0], "timed out")
  assert.doesNotMatch(generic.paragraphs.join(" "), /\.log/)
})

const ssh = (kind: SshFailureKind, extra: Partial<SshFailure> = {}): SshFailure => ({ kind, destination: "devbox", detail: "stderr tail", ...extra })
const text = (instructions: Instructions): string => [instructions.title, ...instructions.paragraphs].join("\n")

test("every ssh failure kind has a title, the detail, and no stop and retry except mismatch", () => {
  const kinds: SshFailureKind[] = ["sshMissing", "sshTooOld", "hostKey", "auth", "unreachable", "servefMissing", "servefTooOld", "mismatch", "forwardDenied", "runtimeDir", "failed"]
  for (const kind of kinds) {
    const instructions = sshFailureInstructions(ssh(kind), "1.2.3")
    assert.notEqual(instructions.title, "", kind)
    if (kind !== "mismatch") {
      assert.ok(instructions.paragraphs.includes("stderr tail"), kind)
      assert.equal(instructions.stopAndRetry, false, kind)
    }
  }
})

test("sshMissing and sshTooOld ask for the OpenSSH client", () => {
  assert.match(text(sshFailureInstructions(ssh("sshMissing"), null)), /Install the OpenSSH client/)
  assert.match(text(sshFailureInstructions(ssh("sshTooOld"), null)), /OpenSSH 6\.7 or newer/)
})

test("hostKey points at ssh in a terminal and never offers to accept the key", () => {
  const instructions = sshFailureInstructions(ssh("hostKey"), null)
  assert.match(text(instructions), /Run ssh devbox in a terminal once to verify the host key, then Retry\./)
  assert.deepEqual(instructions.commands, ["ssh devbox"])
  assert.equal(instructions.stopAndRetry, false)
  assert.doesNotMatch(text(instructions), /accept (the|this) (host )?key/i)
})

test("auth asks for an agent key and a batch-mode ssh that succeeds", () => {
  const instructions = sshFailureInstructions(ssh("auth"), null)
  assert.deepEqual(instructions.commands, ["ssh-add", "ssh -o BatchMode=yes devbox true"])
  assert.match(text(instructions), /never asks for passwords/)
})

test("unreachable shows the detail and says servef retries", () => {
  const instructions = sshFailureInstructions(ssh("unreachable", { detail: "ssh: connect to host devbox port 22: Connection timed out" }), null)
  assert.deepEqual(instructions.paragraphs, ["ssh: connect to host devbox port 22: Connection timed out", "servef retries while a window for this host is open."])
})

const INSTALL_SCRIPT = "# on devbox\ncurl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh"

test("releaseOf links only packaged stable releases", () => {
  assert.equal(releaseOf("1.2.3", true), "1.2.3")
  assert.equal(releaseOf("1.2.3", false), null)
  assert.equal(releaseOf("0.0.0-dev", true), null, "local package builds have no release page")
  assert.equal(releaseOf("0.0.0-edge", true), null, "edge prerelease builds have no versioned release page")
})

test("servefMissing on Darwin offers the install script", () => {
  const instructions = sshFailureInstructions(ssh("servefMissing", { os: "Darwin", arch: "arm64" }), "1.2.3")
  assert.deepEqual(instructions.commands, [INSTALL_SCRIPT])
  assert.doesNotMatch(text(instructions), /Homebrew|go install/)
  assert.match(text(instructions), new RegExp(`in one of: ${escape(REMOTE_PATH_DIRS.join(", "))}\\.`))
  assert.match(text(instructions), /The app never installs anything on devbox\./)
})

test("servefMissing on Linux maps uname -m to the release archive", () => {
  for (const [machine, arch] of [["x86_64", "amd64"], ["aarch64", "arm64"], ["arm64", "arm64"]]) {
    const instructions = sshFailureInstructions(ssh("servefMissing", { os: "Linux", arch: machine }), "1.2.3")
    const archive = `servef_1.2.3_linux_${arch}`
    assert.deepEqual(instructions.commands, [
      INSTALL_SCRIPT,
      `# on devbox\nmkdir -p ~/.local/bin\ncurl -fsSL https://github.com/flexdinesh/servef/releases/download/v1.2.3/${archive}.tar.gz | tar -xz -C ~/.local/bin --strip-components=1 ${archive}/servef`,
    ], machine)
    assert.match(text(instructions), new RegExp(`${archive}\\.tar\\.gz from https://github\\.com/flexdinesh/servef/releases/tag/v1\\.2\\.3`))
    assert.match(text(instructions), /must be on PATH for non-interactive SSH commands/)
    assert.match(text(instructions), /never installs anything on devbox/)
  }
})

test("servefMissing in a dev build links the latest release without a versioned download", () => {
  const instructions = sshFailureInstructions(ssh("servefMissing", { os: "Linux", arch: "x86_64" }), null)
  assert.deepEqual(instructions.commands, [INSTALL_SCRIPT])
  assert.match(text(instructions), /releases\/latest/)
  assert.doesNotMatch(text(instructions), /releases\/tag/)
})

test("servefMissing with an unknown arch or os names the supported platforms and the releases page", () => {
  for (const extra of [{ os: "Linux", arch: "riscv64" }, { os: "FreeBSD", arch: "amd64" }, {}]) {
    const instructions = sshFailureInstructions(ssh("servefMissing", extra), "1.2.3")
    assert.deepEqual(instructions.commands, [INSTALL_SCRIPT])
    assert.match(text(instructions), /macOS and Linux on amd64 and arm64/)
    assert.match(text(instructions), /releases\/tag\/v1\.2\.3/)
    assert.doesNotMatch(text(instructions), /linux_/)
    assert.match(text(instructions), /must be on PATH/)
  }
})

test("servefTooOld offers the install script on the host", () => {
  const instructions = sshFailureInstructions(ssh("servefTooOld"), "1.2.3")
  assert.deepEqual(instructions.commands, [INSTALL_SCRIPT])
})

test("mismatch tells which side to update and offers stop and retry on the host", () => {
  const older = sshFailureInstructions(ssh("mismatch", { status: { ...status, protocol: 0, version: "0.9.0" } }), "1.2.3")
  assert.equal(older.stopAndRetry, true)
  assert.match(text(older), /Upgrade servef on devbox/)
  assert.match(text(older), /version 0\.9\.0\) speaks protocol 0; this app needs protocol 1/)
  assert.equal(older.commands.at(-1), "# on devbox\nservef daemon stop")
  assert.deepEqual(older.commands, [INSTALL_SCRIPT, "# on devbox\nservef daemon stop"])

  const newer = sshFailureInstructions(ssh("mismatch", { status }), "1.2.3")
  assert.equal(newer.stopAndRetry, true)
  assert.match(text(newer), /Update the servef app/)
  assert.deepEqual(newer.commands, ["# on devbox\nservef daemon stop"])
})

test("forwardDenied names the sshd settings", () => {
  const value = text(sshFailureInstructions(ssh("forwardDenied"), null))
  assert.match(value, /AllowStreamLocalForwarding yes/)
  assert.match(value, /DisableForwarding/)
  assert.match(value, /OpenSSH 6\.7/)
})

test("runtimeDir shows the path and reason from the detail", () => {
  const instructions = sshFailureInstructions(ssh("runtimeDir", { detail: "/run/user/1000/servef-electron-ab is a symlink" }), null)
  assert.equal(instructions.paragraphs[0], "/run/user/1000/servef-electron-ab is a symlink")
})

test("failed shows the detail tail and the remote log when the socket is known", () => {
  const long = `${"x".repeat(5000)}END`
  const instructions = sshFailureInstructions(ssh("failed", { detail: long, status: { ...status, protocol: 1 } }), null)
  assert.ok((instructions.paragraphs[0] ?? "").endsWith("END"))
  assert.ok((instructions.paragraphs[0] ?? "").length <= 2048)
  assert.match(text(instructions), /See \/run\/s\.sock\.log on devbox/)
  const bare = sshFailureInstructions(ssh("failed", { detail: "" }), null)
  assert.deepEqual(bare.paragraphs, ["The SSH connection failed."])
})

function escape(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}
