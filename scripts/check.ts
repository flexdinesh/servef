import { appendFileSync, closeSync, mkdirSync, openSync, readFileSync, rmSync } from "node:fs"
import path from "node:path"

import { startProcess, stopProcess, type ManagedProcess } from "./browser-process.ts"

// Runs check groups in order and each group's phases in a bounded pool
// (decision 0057). Each phase writes to its own log; the terminal gets one line
// per phase, plus the log tail, path, and rerun command when one fails. A
// failure lets its group finish and skips later groups.

export interface Phase {
  name: string
  command: string[]
  rerun: string
}

export interface CheckOptions {
  groups: Phase[][]
  logDir: string
  jobs: number
  // Stream output to the terminal instead of logs.
  verbose: boolean
  write: (text: string) => void
  env?: NodeJS.ProcessEnv
  signal?: AbortSignal
}

const tailLines = 100

function seconds(started: number): string {
  return `${((performance.now() - started) / 1000).toFixed(1)}s`
}

function tail(file: string): string {
  const lines = readFileSync(file, "utf8").split("\n")
  if (lines.at(-1) === "") lines.pop()
  return lines.slice(-tailLines).join("\n")
}

export async function runChecks(options: CheckOptions): Promise<number> {
  const { groups, logDir, verbose, write, signal } = options
  const env = options.env ?? process.env
  rmSync(logDir, { recursive: true, force: true })
  if (!verbose) mkdirSync(logDir, { recursive: true })
  const running = new Set<ManagedProcess>()
  const stop = () => { for (const proc of running) void stopProcess(proc) }
  signal?.addEventListener("abort", stop, { once: true })
  const numbers = new Map(groups.flat().map((phase, index) => [phase, index + 1]))
  let exitCode = 0

  async function run(phase: Phase) {
    if (signal?.aborted) {
      write(`skip ${phase.name}\n`)
      return
    }
    const number = String(numbers.get(phase) ?? 0).padStart(2, "0")
    const log = path.join(logDir, `${number}-${phase.name.replaceAll(":", "-")}.log`)
    const output = verbose ? undefined : openSync(log, "w")
    const started = performance.now()
    let code: number
    try {
      const [command = "", ...args] = phase.command
      const proc = startProcess(command, args, env, false, output)
      running.add(proc)
      try {
        code = await proc.exited
      } catch (error) {
        code = 127
        const message = `${error instanceof Error ? error.message : String(error)}\n`
        if (verbose) process.stderr.write(message)
        else appendFileSync(log, message)
      } finally {
        running.delete(proc)
      }
    } finally {
      if (output !== undefined) closeSync(output)
    }
    if (signal?.aborted) {
      write(`stop ${phase.name} ${seconds(started)}\n`)
      return
    }
    if (code === 0) {
      write(`ok ${phase.name} ${seconds(started)}\n`)
      return
    }
    if (exitCode === 0) exitCode = code
    const detail = verbose ? "" : `${tail(log)}\nlog: ${log}\n`
    write(`FAIL ${phase.name} ${seconds(started)}\n${detail}rerun: ${phase.rerun}\n`)
  }

  try {
    for (const group of groups) {
      if (exitCode !== 0 || signal?.aborted) {
        for (const phase of group) write(`skip ${phase.name}\n`)
        continue
      }
      const queue = [...group]
      const worker = async () => {
        for (let phase = queue.shift(); phase !== undefined; phase = queue.shift()) await run(phase)
      }
      await Promise.all(Array.from({ length: Math.min(options.jobs, group.length) }, worker))
    }
  } finally {
    signal?.removeEventListener("abort", stop)
  }
  return signal?.aborted ? 130 : exitCode
}

// Groups are separated by `:::`; one argument may hold several space-separated
// task names, as mise templates expand.
export function parseGroups(args: string[]): string[][] {
  const groups: string[][] = [[]]
  for (const arg of args) {
    if (arg === ":::") groups.push([])
    else groups[groups.length - 1]?.push(...arg.split(/\s+/).filter((name) => name !== ""))
  }
  return groups.filter((group) => group.length > 0)
}

// Matches mise: values below 1 mean 1; unset or invalid means its default of 8.
export function jobCount(value: string | undefined): number {
  const jobs = Number(value)
  if (value === undefined || value === "" || !Number.isInteger(jobs)) return 8
  return Math.max(1, jobs)
}

if (import.meta.main) {
  const groups = parseGroups(process.argv.slice(2)).map((group) => group.map((task) => ({
    name: task,
    command: ["mise", "run", "-q", "--skip-deps", task],
    rerun: `mise run ${task}`,
  })))
  if (groups.length === 0) {
    console.error("usage: node scripts/check.ts task... [::: task...]")
    process.exit(2)
  }
  const controller = new AbortController()
  const abort = () => { controller.abort() }
  process.once("SIGINT", abort)
  process.once("SIGTERM", abort)
  process.exitCode = await runChecks({
    groups,
    logDir: path.join(".dev", "check-logs"),
    jobs: jobCount(process.env.MISE_JOBS),
    verbose: process.env.SERVEF_CHECK_VERBOSE === "1",
    write: (text) => { process.stdout.write(text) },
    signal: controller.signal,
  })
}
