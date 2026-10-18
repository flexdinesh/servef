import { spawnSync } from 'node:child_process'

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

const [command, args] = isolated() ? ['unshare', [...isolate, ...playwright]] : [playwright[0], playwright.slice(1)]
const result = spawnSync(command, args, { stdio: 'inherit' })
if (result.error) throw result.error
process.exitCode = result.status ?? 1
