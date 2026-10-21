import { spawnSync } from "node:child_process"
import { mkdirSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

import { gitApiPort } from "./ports.ts"

// gitFixtureRoot is a throwaway repository with one change of every kind,
// rebuilt before each run. Without git, it is a plain folder and the Source
// Control specs skip.
export const gitFixtureRoot = path.join(tmpdir(), `servef-browser-git-${gitApiPort}`)

export const gitFixtureChanges = 8

// largeText is a ~1.6 MB file; the working copy changes every twentieth line.
function largeText(changed: boolean): string {
  const lines = Array.from({ length: 24_000 }, (_, index) => {
    const fox = changed && index % 20 === 0 ? "slow" : "quick"
    return `Line ${index}: the ${fox} brown fox jumps over the lazy dog, step ${index}.`
  })
  return `${lines.join("\n")}\n`
}

const committed: Record<string, string> = {
  "README.md": "# Fixture\n\nOne\nTwo\nThree\n",
  "src/app.ts": "export const answer = 1\n\nexport function run() {\n  return answer\n}\n",
  "notes/removed.md": "# Removed\n\nThis note goes away.\n",
  "guides/intro.md": "# Intro\n\nRenamed without edits so git pairs it by content.\n",
  "assets/logo.bin": "\u0000\u0001\u0002 logo v1",
  "clean.md": "# Clean\n",
  "large.txt": largeText(false),
  ".gitignore": "*.log\n",
}

const working: Record<string, string> = {
  "README.md": "# Fixture\n\nOne\nTwo, changed\nThree\nFour\n",
  "src/app.ts": "export const answer = 2\n\nexport function run() {\n    return answer\n}\n",
  "guides/start.md": committed["guides/intro.md"] ?? "",
  "assets/logo.bin": "\u0000\u0001\u0002 logo v2, larger",
  "added.md": "# Added\n\nStaged, never committed.\n",
  "untracked.txt": "Not yet added.\n",
  "large.txt": largeText(true),
  "debug.log": "ignored\n",
}

function write(files: Record<string, string>) {
  for (const [name, content] of Object.entries(files)) {
    const file = path.join(gitFixtureRoot, name)
    mkdirSync(path.dirname(file), { recursive: true })
    writeFileSync(file, content)
  }
}

function git(...args: string[]): boolean {
  const result = spawnSync("git", ["-c", "user.name=servef", "-c", "user.email=servef@example.invalid", "-c", "commit.gpgsign=false", ...args], {
    cwd: gitFixtureRoot,
    env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    stdio: "ignore",
  })
  return result.status === 0
}

export function createGitFixture() {
  rmSync(gitFixtureRoot, { recursive: true, force: true })
  mkdirSync(gitFixtureRoot, { recursive: true })
  write(committed)
  if (!git("init", "--quiet", "--initial-branch=main")) {
    rmSync(path.join(gitFixtureRoot, ".git"), { recursive: true, force: true })
    return
  }
  git("add", "--all")
  git("commit", "--quiet", "--message", "Initial")
  rmSync(path.join(gitFixtureRoot, "notes/removed.md"))
  rmSync(path.join(gitFixtureRoot, "guides/intro.md"))
  write(working)
  git("add", "added.md")
}

if (process.argv[1] && path.resolve(process.argv[1]) === new URL(import.meta.url).pathname) createGitFixture()
