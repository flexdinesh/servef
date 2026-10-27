import { spawnSync } from "node:child_process"
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

// liveFixtureRoot is a throwaway repository the live-update specs change on
// disk (decision 0050). It has its own server so other specs never see those
// changes. Each run gets its own folder, through the environment so the
// runner, its workers, and the server command agree.
function fixtureRoot(): string {
  const existing = process.env.SERVEF_BROWSER_LIVE_ROOT
  if (existing) return existing
  const created = mkdtempSync(path.join(tmpdir(), "servef-browser-live-"))
  process.env.SERVEF_BROWSER_LIVE_ROOT = created
  return created
}

export const liveFixtureRoot = fixtureRoot()

// longLines is long.md's line count: enough to scroll.
export const longLines = 400

export function longDocument(marker: string): string {
  const lines = Array.from({ length: longLines }, (_, index) => `Line ${index + 1} of the long document.`)
  return `# Long ${marker}\n\n${lines.join("\n")}\n`
}

// Git hooks export GIT_DIR and friends, which would aim these commands at
// servef's own repository instead of the fixture.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("GIT_")))

function git(...args: string[]): boolean {
  const result = spawnSync("git", ["-c", "user.name=servef", "-c", "user.email=servef@example.invalid", "-c", "commit.gpgsign=false", ...args], {
    cwd: liveFixtureRoot,
    env: { ...env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    stdio: "ignore",
  })
  return result.status === 0
}

export function createLiveFixture() {
  rmSync(liveFixtureRoot, { recursive: true, force: true })
  mkdirSync(path.join(liveFixtureRoot, "docs"), { recursive: true })
  writeFileSync(path.join(liveFixtureRoot, "long.md"), longDocument("original"))
  writeFileSync(path.join(liveFixtureRoot, "docs", "guide.md"), "# Guide\n")
  if (!git("init", "--quiet", "--initial-branch=main")) {
    rmSync(path.join(liveFixtureRoot, ".git"), { recursive: true, force: true })
    return
  }
  git("add", "--all")
  git("commit", "--quiet", "--message", "Initial")
}

if (process.argv[1] && path.resolve(process.argv[1]) === new URL(import.meta.url).pathname) createLiveFixture()
