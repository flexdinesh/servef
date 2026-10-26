import { spawnSync } from "node:child_process"
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

// gitFixtureRoot is a throwaway repository with one change of every kind,
// rebuilt before each run. Without git, it is a plain folder and the Source
// Control specs skip. Each run gets its own folder, through the environment so
// the runner, its workers, and the server command agree, so concurrent runs from
// other worktrees cannot rebuild it mid-run. mise run dev rebuilds it in .dev.
function fixtureRoot(): string {
  const existing = process.env.SERVEF_BROWSER_GIT_ROOT
  if (existing) return existing
  const created = mkdtempSync(path.join(tmpdir(), "servef-browser-git-"))
  process.env.SERVEF_BROWSER_GIT_ROOT = created
  return created
}

export const gitFixtureRoot = fixtureRoot()

// gitFixtureBaseWorktree is a linked worktree of the fixture with main, the
// base branch, checked out. It lives beside the served root; servef opens a
// linked worktree whose repository lies in a root.
export const gitFixtureBaseWorktree = `${gitFixtureRoot}-main`

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

// The feature branch is the middle of a stack on main: lowerBranch below it,
// HEAD on feature, and upperBranch on top. Each layer adds one commit.
export const lowerBranch = "docs-base"
export const lowerSubject = "docs: add the guide index"
const lowerCommit: Record<string, string> = {
  "docs/index.md": "# Index\n\nWritten on the lower layer.\n",
}

export const upperBranch = "docs-followup"
export const upperSubject = "docs: add a changelog"
const upperCommit: Record<string, string> = {
  "docs/changelog.md": "# Changelog\n\nWritten on the upper layer.\n",
}

// featureCommit is the feature branch's one commit; the working tree keeps it.
const featureCommit: Record<string, string> = {
  "docs/history.md": "# History\n\nWritten on the feature branch.\n",
  "clean.md": "# Clean\n\nEdited on the feature branch.\n",
}

export const featureSubject = "docs: add history notes"
const featureMessage = `${featureSubject}\n\nExplain how the fixture's branch history is built.\n\nThe feature branch adds one commit on top of ${lowerBranch}.`

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

// Git hooks and `git bisect run` export GIT_DIR and friends, which would aim
// these commands at servef's own repository instead of the fixture.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("GIT_")))

function git(...args: string[]): boolean {
  const result = spawnSync("git", ["-c", "user.name=servef", "-c", "user.email=servef@example.invalid", "-c", "commit.gpgsign=false", ...args], {
    cwd: gitFixtureRoot,
    env: { ...env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    stdio: "ignore",
  })
  return result.status === 0
}

// commitOn creates branch at HEAD, switches to it, and commits files.
function commitOn(branch: string, files: Record<string, string>, message: string) {
  git("switch", "--quiet", "--create", branch)
  write(files)
  git("add", "--all")
  git("commit", "--quiet", "--message", message)
}

export function createGitFixture() {
  rmSync(gitFixtureRoot, { recursive: true, force: true })
  rmSync(gitFixtureBaseWorktree, { recursive: true, force: true })
  mkdirSync(gitFixtureRoot, { recursive: true })
  write(committed)
  if (!git("init", "--quiet", "--initial-branch=main")) {
    rmSync(path.join(gitFixtureRoot, ".git"), { recursive: true, force: true })
    return
  }
  git("add", "--all")
  git("commit", "--quiet", "--message", "Initial")
  // A second base for the branch compare's base picker.
  git("branch", "release")
  // A branch stack, so Commits has a branch range against main and Branches
  // has layers below and above HEAD.
  commitOn(lowerBranch, lowerCommit, lowerSubject)
  commitOn("feature", featureCommit, featureMessage)
  commitOn(upperBranch, upperCommit, upperSubject)
  git("switch", "--quiet", "feature")
  // HEAD is the base branch in this worktree.
  git("worktree", "add", "--quiet", gitFixtureBaseWorktree, "main")
  rmSync(path.join(gitFixtureRoot, "notes/removed.md"))
  rmSync(path.join(gitFixtureRoot, "guides/intro.md"))
  write(working)
  git("add", "added.md")
}

if (process.argv[1] && path.resolve(process.argv[1]) === new URL(import.meta.url).pathname) createGitFixture()
