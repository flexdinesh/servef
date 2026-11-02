import assert from "node:assert/strict"
import { test } from "node:test"
import { cliId, cliPort, dialHost, listCliServers, parseCliList } from "../src/main/cli-servers.ts"
import type { ExecFile, ExecOutcome } from "../src/main/daemon.ts"

function runner(outcome: ExecOutcome) {
  const calls: { file: string; args: readonly string[] }[] = []
  const run: ExecFile = async (file, args) => {
    calls.push({ file, args })
    return outcome
  }
  return { run, calls }
}

test("listCliServers runs servef control list --json as argv and keeps loopback servers", async () => {
  const stdout = JSON.stringify({ servers: [
    { host: "localhost", port: 7971, origin: "http://localhost:7971" },
    { host: "0.0.0.0", port: 7972, origin: "http://127.0.0.1:7972" },
    { host: "::", port: 7973, origin: "http://[::1]:7973" },
    { host: "192.168.1.5", port: 7974, origin: "http://192.168.1.5:7974" },
    { host: "::1", port: 7971, origin: "http://[::1]:7971" },
  ] })
  const { run, calls } = runner({ code: 0, stdout: `${stdout}\n`, stderr: "" })
  assert.deepEqual(await listCliServers("/opt/servef", run), { ok: true, servers: [
    { host: "localhost", port: 7971, origin: "http://localhost:7971" },
    { host: "0.0.0.0", port: 7972, origin: "http://127.0.0.1:7972" },
    { host: "::", port: 7973, origin: "http://[::1]:7973" },
  ] })
  assert.deepEqual(calls, [{ file: "/opt/servef", args: ["control", "list", "--json"] }])
})

test("listCliServers reports spawn failures, timeouts, exits, and bad output", async () => {
  const cases: [ExecOutcome, RegExp][] = [
    [{ code: null, stdout: "", stderr: "", error: { code: "ENOENT", message: "spawn servef ENOENT", timedOut: false } }, /Cannot run servef: spawn servef ENOENT/],
    [{ code: null, stdout: "", stderr: "", error: { message: "killed", timedOut: true } }, /timed out after 5 s/],
    [{ code: 1, stdout: "", stderr: "Error: unknown command \"control\" for \"servef\"\n" }, /unknown command "control"/],
    [{ code: 2, stdout: "", stderr: "" }, /exited with code 2/],
    [{ code: 0, stdout: "http://localhost:7971\n", stderr: "" }, /unexpected answer/],
  ]
  for (const [outcome, detail] of cases) {
    const result = await listCliServers("servef", runner(outcome).run)
    assert.equal(result.ok, false)
    assert.match(result.ok ? "" : result.detail, detail)
  }
})

test("parseCliList rejects malformed shapes and skips foreign origins", () => {
  for (const text of ["", "[]", "{}", "{\"servers\":{}}", "{\"servers\":[{\"host\":\"localhost\",\"port\":\"7971\",\"origin\":\"http://localhost:7971\"}]}", "{\"servers\":[{\"host\":\"localhost\",\"port\":0,\"origin\":\"http://localhost:0\"}]}", "{\"servers\":[null]}"]) {
    assert.equal(parseCliList(text), null, text)
  }
  assert.deepEqual(parseCliList("{\"servers\":[]}"), [])
  for (const origin of ["https://localhost:7971", "http://localhost:7972", "http://user@localhost:7971", "http://localhost:7971/x", "http://example.com:7971", "http://localhost.:7971", "http://[::2]:7971"]) {
    assert.deepEqual(parseCliList(JSON.stringify({ servers: [{ host: "localhost", port: 7971, origin }] })), [], origin)
  }
})

test("dialHost strips IPv6 brackets and accepts 127.0.0.0/8", () => {
  assert.equal(dialHost({ host: "", port: 80, origin: "http://[::1]" }), null, "the origin must carry the port")
  assert.equal(dialHost({ host: "", port: 8080, origin: "http://[::1]:8080" }), "::1")
  assert.equal(dialHost({ host: "", port: 8080, origin: "http://127.0.0.2:8080" }), "127.0.0.2")
  assert.equal(dialHost({ host: "", port: 8080, origin: "http://LOCALHOST:8080" }), "localhost")
})

test("cli ids are canonical cli-<port>", () => {
  assert.equal(cliId(7971), "cli-7971")
  assert.equal(cliPort("cli-7971"), 7971)
  assert.equal(cliPort("cli-65535"), 65535)
  for (const id of ["cli-0", "cli-07971", "cli-65536", "cli-123456", "cli-", "cli-1a", "ssh-7971", "local"]) assert.equal(cliPort(id), null, id)
})
