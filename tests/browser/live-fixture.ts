import { spawnSync } from "node:child_process"
import { mkdirSync, rmSync, writeFileSync } from "node:fs"
import path from "node:path"

// longLines is long.md's line count: enough to scroll.
export const longLines = 400

export function longDocument(marker: string): string {
  const lines = Array.from({ length: longLines }, (_, index) => `Line ${index + 1} of the long document.`)
  return `# Long ${marker}\n\n${lines.join("\n")}\n`
}

// Git hooks export GIT_DIR and friends, which would aim these commands at
// srcz's own repository instead of the fixture.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("GIT_")))

function git(liveFixtureRoot: string, ...args: string[]): boolean {
  const result = spawnSync("git", ["-c", "user.name=srcz", "-c", "user.email=srcz@example.invalid", "-c", "commit.gpgsign=false", ...args], {
    cwd: liveFixtureRoot,
    env: { ...env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    stdio: "ignore",
  })
  return result.status === 0
}

export function createLiveFixture(liveFixtureRoot: string) {
  rmSync(liveFixtureRoot, { recursive: true, force: true })
  mkdirSync(path.join(liveFixtureRoot, "docs"), { recursive: true })
  writeFileSync(path.join(liveFixtureRoot, "long.md"), longDocument("original"))
  writeFileSync(path.join(liveFixtureRoot, "docs", "guide.md"), "# Guide\n")
  if (!git(liveFixtureRoot, "init", "--quiet", "--initial-branch=main")) {
    throw new Error("Could not initialize live fixture repository")
  }
  if (!git(liveFixtureRoot, "add", "--all") || !git(liveFixtureRoot, "commit", "--quiet", "--message", "Initial")) {
    throw new Error("Could not commit live fixture repository")
  }
}
