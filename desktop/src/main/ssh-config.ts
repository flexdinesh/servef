// ssh config alias listing (pure over an injected reader) and `ssh -G`
// resolution of those aliases (decision 0070).
import { readdirSync, readFileSync } from "node:fs"
import { posix } from "node:path"
import type { ExecFile } from "./daemon.ts"
import { parseSshG, sshGArgs, validDestination } from "./ssh-args.ts"
import type { ResolvedAlias } from "./ssh-links.ts"

export interface ConfigReader {
  // home is the user's home directory; relative Include paths resolve in home/.ssh.
  home: string
  // readFile returns the file text, else null.
  readFile(path: string): string | null
  // listDir returns entry names, else null.
  listDir(path: string): string[] | null
}

const MAX_INCLUDE_DEPTH = 16

// listHostAliases returns Host tokens from file and its Includes (depth ≤ 16,
// `*`/`?` globs in the last path segment only, cycles ignored), skipping
// tokens containing `*`, `?`, or `!`; Match blocks ignored; deduplicated in
// config order. Only tokens passing validDestination are returned.
export function listHostAliases(file: string, reader: ConfigReader): string[] {
  const aliases: string[] = []
  const seen = new Set<string>()
  const active = new Set<string>()
  const visit = (path: string, depth: number): void => {
    if (depth > MAX_INCLUDE_DEPTH || active.has(path)) return
    const text = reader.readFile(path)
    if (text === null) return
    active.add(path)
    let inMatch = false
    for (const line of text.split(/\r?\n/)) {
      const parsed = parseLine(line)
      if (parsed === null) continue
      const keyword = parsed.keyword.toLowerCase()
      if (keyword === "match") {
        inMatch = true
        continue
      }
      if (keyword === "host") {
        inMatch = false
        for (const token of parsed.args) {
          if (/[*?!]/.test(token) || !validDestination(token) || seen.has(token)) continue
          seen.add(token)
          aliases.push(token)
        }
        continue
      }
      if (keyword === "include" && !inMatch) {
        for (const pattern of parsed.args) {
          for (const included of expandInclude(pattern, reader)) visit(included, depth + 1)
        }
      }
    }
    active.delete(path)
  }
  visit(file, 0)
  return aliases
}

// parseLine splits `Keyword args` or `Keyword=args` into whitespace
// separated tokens; double quotes group a token. Comments and blanks are null.
function parseLine(line: string): { keyword: string; args: string[] } | null {
  const trimmed = line.trim()
  if (trimmed === "" || trimmed.startsWith("#")) return null
  const match = /^([A-Za-z]+)(?:\s*=\s*|\s+)(.*)$/.exec(trimmed)
  if (match === null) return null
  return { keyword: match[1] ?? "", args: tokenize(match[2] ?? "") }
}

function tokenize(text: string): string[] {
  const tokens: string[] = []
  const pattern = /"([^"]*)"|(\S+)/g
  for (let match = pattern.exec(text); match !== null; match = pattern.exec(text)) {
    const token = match[1] ?? match[2] ?? ""
    if (match[2] !== undefined && token.startsWith("#")) break
    if (token !== "") tokens.push(token)
  }
  return tokens
}

// expandInclude resolves an Include argument: `~/` is the home directory,
// relative paths are in home/.ssh, and `*`/`?` match in the last segment only
// (sorted; a leading dot must match literally, as glob(3)).
function expandInclude(pattern: string, reader: ConfigReader): string[] {
  let path = pattern
  if (path.startsWith("~/")) path = posix.join(reader.home, path.slice(2))
  else if (!path.startsWith("/")) path = posix.join(reader.home, ".ssh", path)
  const dir = posix.dirname(path)
  const name = posix.basename(path)
  if (/[*?[]/.test(dir)) return []
  if (!/[*?]/.test(name)) return [path]
  const entries = reader.listDir(dir)
  if (entries === null) return []
  const matcher = globMatcher(name)
  return entries
    .filter((entry) => matcher.test(entry) && (!entry.startsWith(".") || name.startsWith(".")))
    .sort()
    .map((entry) => posix.join(dir, entry))
}

function globMatcher(glob: string): RegExp {
  const source = [...glob].map((char) => {
    if (char === "*") return ".*"
    if (char === "?") return "."
    return char.replace(/[\\^$.|+()[\]{}]/g, "\\$&")
  }).join("")
  return new RegExp(`^${source}$`, "s")
}

// nodeConfigReader reads the real filesystem.
export function nodeConfigReader(home: string): ConfigReader {
  return {
    home,
    readFile(path) {
      try {
        return readFileSync(path, "utf8")
      } catch {
        return null
      }
    },
    listDir(path) {
      try {
        return readdirSync(path)
      } catch {
        return null
      }
    },
  }
}

export const MAX_ALIASES = 64
export const SSH_G_TIMEOUT_MS = 3_000
export const SSH_G_CONCURRENCY = 4

// resolveAliases runs `ssh -G` (sshGArgs) for at most MAX_ALIASES aliases,
// SSH_G_CONCURRENCY at a time, SSH_G_TIMEOUT_MS each; failures are dropped.
export async function resolveAliases(aliases: readonly string[], ssh: string, config: string | null, run: ExecFile): Promise<ResolvedAlias[]> {
  const queue = aliases.slice(0, MAX_ALIASES)
  const results: (ResolvedAlias | null)[] = queue.map(() => null)
  let next = 0
  const worker = async (): Promise<void> => {
    while (next < queue.length) {
      const index = next
      next += 1
      const alias = queue[index] ?? ""
      try {
        const outcome = await run(ssh, sshGArgs(alias, config), { timeoutMs: SSH_G_TIMEOUT_MS })
        if (outcome.code !== 0 || outcome.error !== undefined) continue
        const values = parseSshG(outcome.stdout)
        if (values !== null) results[index] = { alias, ...values }
      } catch {
        // A runner that throws counts as a failed alias.
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(SSH_G_CONCURRENCY, queue.length) }, worker))
  return results.filter((result) => result !== null)
}
