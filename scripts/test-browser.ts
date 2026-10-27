import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'

// Chromium aborts in-flight requests with ERR_NETWORK_CHANGED whenever the
// host's interfaces change, e.g. when Docker creates a veth link for a
// container. Vite serves each module separately, so a page loading at that
// moment stays blank and its test times out. On Linux, run Playwright, its
// servers, and Chromium in a private network namespace with only loopback,
// which host interface changes cannot reach. Elsewhere, or where unprivileged
// namespaces are unavailable, run Playwright directly.
const playwright = ['playwright', 'test', ...process.argv.slice(2)]
const isolate = ['--user', '--map-root-user', '--net', 'sh', '-c', 'ip link set lo up && exec "$@"', 'sh']

function isolated(): boolean {
  if (process.platform !== 'linux') return false
  const probe = spawnSync('unshare', [...isolate, 'true'], { stdio: 'ignore' })
  return probe.status === 0
}

// One git fixture folder per run; see tests/browser/git-fixture.ts.
const gitFixtureRoot = mkdtempSync(path.join(tmpdir(), 'servef-browser-git-'))
// And one live-update fixture; see tests/browser/live-fixture.ts.
const liveFixtureRoot = mkdtempSync(path.join(tmpdir(), 'servef-browser-live-'))
const [command, args] = isolated() ? ['unshare', [...isolate, ...playwright]] : [playwright[0], playwright.slice(1)]
const result = spawnSync(command, args, {
  stdio: 'inherit',
  env: { ...process.env, SERVEF_BROWSER_GIT_ROOT: gitFixtureRoot, SERVEF_BROWSER_LIVE_ROOT: liveFixtureRoot },
})
rmSync(gitFixtureRoot, { recursive: true, force: true })
rmSync(liveFixtureRoot, { recursive: true, force: true })
// The fixture's base-branch worktree sits beside it.
rmSync(`${gitFixtureRoot}-main`, { recursive: true, force: true })
// So does the cache holding walkthroughs the specs publish.
rmSync(`${gitFixtureRoot}-cache`, { recursive: true, force: true })
if (result.error) throw result.error
process.exitCode = result.status ?? 1
