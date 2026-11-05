import assert from "node:assert/strict"
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { test } from "node:test"
import type { ExecFile, ExecOutcome } from "../src/main/daemon.ts"
import { listHostAliases, MAX_ALIASES, nodeConfigReader, resolveAliases, SSH_G_CONCURRENCY, SSH_G_TIMEOUT_MS } from "../src/main/ssh-config.ts"
import type { ConfigReader } from "../src/main/ssh-config.ts"

function memoryReader(files: Record<string, string>, home = "/home/dee"): ConfigReader & { reads: string[] } {
  const reads: string[] = []
  return {
    home,
    reads,
    readFile(path) {
      reads.push(path)
      return files[path] ?? null
    },
    listDir(path) {
      const prefix = `${path}/`
      const names = Object.keys(files).filter((file) => file.startsWith(prefix) && !file.slice(prefix.length).includes("/")).map((file) => file.slice(prefix.length))
      return names.length === 0 ? null : names
    },
  }
}

const CONFIG = "/home/dee/.ssh/config"

test("lists Host tokens in config order, deduplicated", () => {
  const reader = memoryReader({
    [CONFIG]: [
      "# comment",
      "Host devbox work-box",
      "  HostName 10.0.0.5",
      "  User dee",
      "",
      "host Devbox_2",
      "HOST devbox",
      "Host=eq-one eq-two",
      "Host = spaced",
      "\tHost   tabbed   # trailing comment",
    ].join("\n"),
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["devbox", "work-box", "Devbox_2", "eq-one", "eq-two", "spaced", "tabbed"])
})

test("skips patterns, negations, and tokens that are not valid destinations", () => {
  const reader = memoryReader({
    [CONFIG]: [
      "Host * !bastion",
      "Host *.example.com web? ok-one",
      "Host \"quoted name\" \"quoted\"",
      "Host -oProxyCommand=x has%percent dee@ok-two",
    ].join("\n"),
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["ok-one", "quoted", "dee@ok-two"])
})

test("ignores Match blocks until the next Host or Match", () => {
  const reader = memoryReader({
    [CONFIG]: [
      "Host before",
      "Match host foo exec \"true\"",
      "  Include matched",
      "  HostName x",
      "Match all",
      "  User y",
      "Host after",
    ].join("\n"),
    "/home/dee/.ssh/matched": "Host from-match\n",
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["before", "after"])
})

test("follows Include: relative to ~/.ssh, ~/ expanded, absolute paths, and last-segment globs sorted", () => {
  const reader = memoryReader({
    [CONFIG]: "Include config.d/* ~/other.conf /etc/ssh/extra.conf\nHost top\n",
    "/home/dee/.ssh/config.d/20-b": "Host b-host\n",
    "/home/dee/.ssh/config.d/10-a": "Host a-host\n",
    "/home/dee/.ssh/config.d/.hidden": "Host hidden-host\n",
    "/home/dee/other.conf": "Host other\n",
    "/etc/ssh/extra.conf": "Host extra\n",
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["a-host", "b-host", "other", "extra", "top"])
})

test("? matches one character and globs outside the last segment are ignored", () => {
  const reader = memoryReader({
    [CONFIG]: "Include hosts-? */nested\n",
    "/home/dee/.ssh/hosts-1": "Host one\n",
    "/home/dee/.ssh/hosts-12": "Host twelve\n",
    "/home/dee/.ssh/sub/nested": "Host nested\n",
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["one"])
})

test("include cycles are ignored", () => {
  const reader = memoryReader({
    [CONFIG]: "Host a\nInclude loop\n",
    "/home/dee/.ssh/loop": "Host b\nInclude config\nInclude loop\n",
  })
  assert.deepEqual(listHostAliases(CONFIG, reader), ["a", "b"])
})

test("include depth is bounded at 16", () => {
  const files: Record<string, string> = { [CONFIG]: "Include level-1\n" }
  for (let level = 1; level <= 20; level++) files[`/home/dee/.ssh/level-${level}`] = `Host host-${level}\nInclude level-${level + 1}\n`
  const aliases = listHostAliases(CONFIG, memoryReader(files))
  assert.equal(aliases.length, 16)
  assert.equal(aliases.at(-1), "host-16")
})

test("a missing config lists nothing", () => {
  assert.deepEqual(listHostAliases(CONFIG, memoryReader({})), [])
})

test("nodeConfigReader reads files and directories", () => {
  const home = mkdtempSync(join(tmpdir(), "servef-ssh-config-"))
  try {
    mkdirSync(join(home, ".ssh", "config.d"), { recursive: true })
    writeFileSync(join(home, ".ssh", "config"), "Include config.d/*.conf\nHost real\n")
    writeFileSync(join(home, ".ssh", "config.d", "a.conf"), "Host from-dir\n")
    writeFileSync(join(home, ".ssh", "config.d", "a.txt"), "Host not-matched\n")
    const reader = nodeConfigReader(home)
    assert.equal(reader.readFile(join(home, "missing")), null)
    assert.equal(reader.listDir(join(home, "missing")), null)
    assert.deepEqual(listHostAliases(join(home, ".ssh", "config"), reader), ["from-dir", "real"])
  } finally {
    rmSync(home, { recursive: true, force: true })
  }
})

function sshG(hostname: string, user: string, port: number): ExecOutcome {
  return { code: 0, stdout: `host x\nhostname ${hostname}\nuser ${user}\nport ${port}\n`, stderr: "" }
}

test("resolveAliases runs ssh -G per alias with the timeout and keeps successes in order", async () => {
  const calls: { file: string; args: readonly string[]; timeoutMs: number }[] = []
  const run: ExecFile = async (file, args, options) => {
    calls.push({ file, args, timeoutMs: options.timeoutMs })
    const alias = args.at(-1)
    if (alias === "broken") return { code: 255, stdout: "", stderr: "bad config" }
    if (alias === "slow") return { code: null, stdout: "", stderr: "", error: { message: "timed out", timedOut: true } }
    if (alias === "garbage") return { code: 0, stdout: "nothing useful", stderr: "" }
    return sshG(`${alias}.example.com`, "dee", alias === "work" ? 2222 : 22)
  }
  const resolved = await resolveAliases(["devbox", "broken", "work", "slow", "garbage"], "/usr/bin/ssh", "/cfg", run)
  assert.deepEqual(resolved, [
    { alias: "devbox", hostname: "devbox.example.com", user: "dee", port: 22 },
    { alias: "work", hostname: "work.example.com", user: "dee", port: 2222 },
  ])
  assert.deepEqual(calls[0], { file: "/usr/bin/ssh", args: ["-G", "-F", "/cfg", "--", "devbox"], timeoutMs: SSH_G_TIMEOUT_MS })
  assert.equal(calls.length, 5)
})

test("resolveAliases caps aliases and concurrency", async () => {
  let running = 0
  let peak = 0
  const seen: string[] = []
  const run: ExecFile = async (_file, args) => {
    running += 1
    peak = Math.max(peak, running)
    seen.push(args.at(-1) ?? "")
    await new Promise((resolve) => setImmediate(resolve))
    running -= 1
    return sshG("h", "u", 22)
  }
  const aliases = Array.from({ length: 100 }, (_, index) => `host-${index}`)
  const resolved = await resolveAliases(aliases, "ssh", null, run)
  assert.equal(resolved.length, MAX_ALIASES)
  assert.equal(seen.length, MAX_ALIASES)
  assert.equal(peak, SSH_G_CONCURRENCY)
  assert.deepEqual(resolved.map((entry) => entry.alias), aliases.slice(0, MAX_ALIASES))
})
