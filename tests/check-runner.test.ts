import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import test from "node:test"
import { pathToFileURL } from "node:url"

import { jobCount, parseGroups, runChecks, type Phase } from "../scripts/check.ts"

function phase(name: string, source: string): Phase {
  return { name, command: [process.execPath, "-e", source], rerun: `mise run ${name}` }
}

function temporary(t: test.TestContext): string {
  const root = mkdtempSync(path.join(tmpdir(), "servef-check-"))
  t.after(() => { rmSync(root, { recursive: true, force: true }) })
  return path.join(root, "logs")
}

async function run(groups: Phase[][], logDir: string, signal?: AbortSignal) {
  let output = ""
  const code = await runChecks({ groups, logDir, jobs: 2, verbose: false, write: (text) => { output += text }, signal })
  return { code, output }
}

const noisy = "for (let i = 1; i <= 50; i++) console.log('noise ' + i); console.error('warning')"

test("success prints one line per phase and keeps full output in logs", async (t) => {
  const logDir = temporary(t)
  const { code, output } = await run([[phase("build:web", noisy)], [phase("lint", noisy), phase("test:go", noisy)]], logDir)
  assert.equal(code, 0)
  const lines = output.trimEnd().split("\n")
  assert.equal(lines.length, 3)
  for (const line of lines) assert.match(line, /^ok (build:web|lint|test:go) \d+\.\ds$/)
  const log = readFileSync(path.join(logDir, "01-build-web.log"), "utf8")
  assert.match(log, /noise 1\n/)
  assert.match(log, /noise 50\n/)
  assert.match(log, /warning/)
  assert.ok(existsSync(path.join(logDir, "03-test-go.log")))
})

test("failure prints its tail, log path, and rerun command, finishes its group, and skips later groups", async (t) => {
  const logDir = temporary(t)
  const failing = phase("lint", "for (let i = 1; i <= 150; i++) console.log('line ' + i); process.exit(3)")
  const sibling = phase("test:go", "setTimeout(() => console.log('done'), 300)")
  const { code, output } = await run([[failing, sibling], [phase("test:browser", "")]], logDir)
  assert.equal(code, 3)
  assert.match(output, /^FAIL lint \d+\.\ds$/m)
  assert.match(output, /^line 51$/m)
  assert.match(output, /^line 150$/m)
  assert.doesNotMatch(output, /^line 50$/m)
  const log = path.join(logDir, "01-lint.log")
  assert.match(output, new RegExp(`^log: ${log.replaceAll(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`, "m"))
  assert.match(output, /^rerun: mise run lint$/m)
  assert.match(output, /^ok test:go /m)
  assert.match(output, /^skip test:browser$/m)
  assert.match(readFileSync(log, "utf8"), /^line 1\n/)
})

test("verbose mode streams output and writes no logs", (t) => {
  const logDir = temporary(t)
  const script = `
    import { runChecks } from ${JSON.stringify(pathToFileURL(path.resolve("scripts/check.ts")).href)}
    process.exitCode = await runChecks({
      groups: [[{ name: "lint", command: [process.execPath, "-e", "console.log('streamed'); console.error('to stderr')"], rerun: "mise run lint" }]],
      logDir: ${JSON.stringify(logDir)}, jobs: 2, verbose: true, write: (text) => { process.stdout.write(text) },
    })
  `
  const result = spawnSync(process.execPath, ["--input-type=module", "-e", script], { encoding: "utf8" })
  assert.equal(result.status, 0, result.stderr)
  assert.match(result.stdout, /^streamed$/m)
  assert.match(result.stdout, /^ok lint \d+\.\ds$/m)
  assert.match(result.stderr, /to stderr/)
  assert.equal(existsSync(logDir), false)
})

test("cancellation stops running phases, skips the rest, and exits 130", { skip: process.platform === "win32" }, async (t) => {
  const logDir = temporary(t)
  const controller = new AbortController()
  const started = performance.now()
  setTimeout(() => { controller.abort() }, 300)
  const { code, output } = await run([[phase("test:go", "setInterval(() => {}, 1000)")], [phase("test:browser", "")]], logDir, controller.signal)
  assert.equal(code, 130)
  assert.ok(performance.now() - started < 5_000)
  assert.match(output, /^stop test:go /m)
  assert.match(output, /^skip test:browser$/m)
})

test("arguments split into groups at ::: and job counts follow mise", () => {
  assert.deepEqual(parseGroups(["build:web", ":::", "fmt:check lint", "test:go", ":::", "test:browser"]),
    [["build:web"], ["fmt:check", "lint", "test:go"], ["test:browser"]])
  assert.deepEqual(parseGroups([":::", ""]), [])
  assert.equal(jobCount(undefined), 8)
  assert.equal(jobCount("2"), 2)
  assert.equal(jobCount("0"), 1)
  assert.equal(jobCount("x"), 8)
})
