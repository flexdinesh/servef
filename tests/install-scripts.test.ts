import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { access, chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import test from 'node:test'

// The install scripts (decisions 0073, 0079) run under /bin/sh with a fake
// HOME and stub uname, curl, wget, go, ditto, xdg-mime,
// update-desktop-database, and pgrep first on PATH. Stubs log their arguments
// to STUB_LOG.

const repositoryRoot = process.cwd()
const scriptsDir = join(repositoryRoot, 'scripts')
const SYSTEM_PATH = '/usr/bin:/bin'

interface Sandbox {
  root: string
  home: string
  stubs: string
  log: string
  env: Record<string, string>
}

interface Result {
  code: number
  stdout: string
  stderr: string
}

const STUBS: Record<string, string[]> = {
  uname: [
    'case "$1" in',
    '  -s) echo "$STUB_UNAME_S" ;;',
    '  -m) echo "$STUB_UNAME_M" ;;',
    'esac',
  ],
  go: [
    'echo "go $*" >> "$STUB_LOG"',
    'if [ "$1" = env ]; then',
    '  case "$2" in',
    '    GOBIN) echo "" ;;',
    '    GOPATH) echo "$HOME/go" ;;',
    '  esac',
    'fi',
  ],
  // macOS only: extracts a release zip as servef.app (servef-native.app for
  // native zips), or copies a directory.
  ditto: [
    'echo "ditto $*" >> "$STUB_LOG"',
    'if [ "$1" = -x ]; then',
    '  case "$3" in',
    '    *servef-native*) app=servef-native ;;',
    '    *) app=servef ;;',
    '  esac',
    '  mkdir -p "$4/$app.app/Contents/MacOS"',
    "  printf 'app' > \"$4/$app.app/Contents/MacOS/$app\"",
    'else',
    '  cp -R "$1" "$2"',
    'fi',
  ],
  'xdg-mime': ['echo "xdg-mime $*" >> "$STUB_LOG"'],
  'update-desktop-database': ['echo "update-desktop-database $*" >> "$STUB_LOG"'],
  pgrep: ['exit "${STUB_PGREP:-1}"'],
  // Serves STUB_RELEASE_DIR/<last URL segment>; -w answers the latest redirect.
  curl: [
    'echo "curl $*" >> "$STUB_LOG"',
    'out=""; write=""; url=""',
    'while [ $# -gt 0 ]; do',
    '  case "$1" in',
    '    -o) out="$2"; shift ;;',
    '    -w) write="$2"; shift ;;',
    '    --retry | --connect-timeout) shift ;;',
    '    -*) ;;',
    '    *) url="$1" ;;',
    '  esac',
    '  shift',
    'done',
    'if [ -n "$write" ]; then printf "%s" "$STUB_LATEST"; exit 0; fi',
    'case " ${STUB_CURL_MISSING:-} " in *" $url "*) exit 22 ;; esac',
    'file="$STUB_RELEASE_DIR/${url##*/}"',
    '[ -f "$file" ] || exit 22',
    'if [ -z "$out" ]; then cat "$file"; else cp "$file" "$out"; fi',
  ],
  wget: [
    'echo "wget $*" >> "$STUB_LOG"',
    'out=""; url=""; headers=""',
    'while [ $# -gt 0 ]; do',
    '  case "$1" in',
    '    -O) out="$2"; shift ;;',
    '    -S) headers=1 ;;',
    '    -*) ;;',
    '    *) url="$1" ;;',
    '  esac',
    '  shift',
    'done',
    'if [ -n "$headers" ]; then',
    '  echo "  HTTP/1.1 302 Found" >&2',
    '  echo "  Location: $STUB_LATEST" >&2',
    '  exit 8',
    'fi',
    'file="$STUB_RELEASE_DIR/${url##*/}"',
    '[ -f "$file" ] || exit 8',
    'if [ "$out" = - ]; then cat "$file"; else cp "$file" "$out"; fi',
  ],
}

function fakeServef(version: string): string {
  return [
    '#!/bin/sh',
    'case "$1" in',
    `  --version) echo "servef ${version}" ;;`,
    '  daemon) exit "${STUB_DAEMON_STATUS:-1}" ;;',
    'esac',
    '',
  ].join('\n')
}

async function writeExecutable(path: string, content: string): Promise<void> {
  await mkdir(dirname(path), { recursive: true })
  await writeFile(path, content)
  await chmod(path, 0o755)
}

async function createSandbox(
  registerCleanup: (cleanup: () => Promise<void>) => void,
  options: { homeName?: string; without?: string[] } = {},
): Promise<Sandbox> {
  const testRoot = join(repositoryRoot, '.scratch', 'test-tmp')
  await mkdir(testRoot, { recursive: true })
  const root = await mkdtemp(join(testRoot, 'servef-install-'))
  registerCleanup(async () => {
    await rm(root, { recursive: true, force: true })
  })
  const home = join(root, options.homeName ?? 'home')
  const stubs = join(root, 'stubs')
  await mkdir(home, { recursive: true })
  for (const [name, lines] of Object.entries(STUBS)) {
    if (options.without?.includes(name) === true) continue
    await writeExecutable(join(stubs, name), ['#!/bin/sh', ...lines, ''].join('\n'))
  }
  const log = join(root, 'stub.log')
  await writeFile(log, '')
  return {
    root,
    home,
    stubs,
    log,
    env: {
      HOME: home,
      PATH: `${stubs}:${join(home, '.local', 'bin')}:${SYSTEM_PATH}`,
      SHELL: '/bin/bash',
      STUB_LOG: log,
      STUB_UNAME_S: 'Linux',
      STUB_UNAME_M: 'x86_64',
      STUB_RELEASE_DIR: join(root, 'release'),
      STUB_LATEST: 'https://github.com/flexdinesh/servef/releases/tag/v0.1.7',
    },
  }
}

// createRelease writes a release's assets for linux/<arch>, named for version
// (0.1.7 by default; the edge prerelease uses 0.0.0-edge): the CLI archive,
// with desktop the AppImage and the darwin/arm64 zip, and the icon.
async function createRelease(
  sandbox: Sandbox,
  options: { arch?: string; desktop?: boolean; version?: string } = {},
): Promise<void> {
  const arch = options.arch ?? 'amd64'
  const version = options.version ?? '0.1.7'
  const release = sandbox.env.STUB_RELEASE_DIR ?? ''
  const name = `servef_${version}_linux_${arch}`
  await writeExecutable(join(release, name, 'servef'), fakeServef(`${version} (abc123)`))
  await new Promise<void>((resolve, reject) => {
    execFile('tar', ['-czf', join(release, `${name}.tar.gz`), '-C', release, name], (error) => {
      if (error === null) resolve()
      else reject(error)
    })
  })
  await rm(join(release, name), { recursive: true })
  await writeFile(join(release, 'icon.png'), 'png')
  if (options.desktop === true) {
    await writeFile(join(release, `servef-electron_${version}_linux_${arch}.AppImage`), 'appimage')
    await writeFile(join(release, `servef-electron_${version}_darwin_arm64.zip`), 'zip')
  }
}

// createDarwinArchive writes the darwin/arm64 CLI archive for version.
async function createDarwinArchive(sandbox: Sandbox, version: string): Promise<void> {
  const release = sandbox.env.STUB_RELEASE_DIR ?? ''
  const name = `servef_${version}_darwin_arm64`
  await writeExecutable(join(release, name, 'servef'), fakeServef(`${version} (abc123)`))
  await new Promise<void>((resolve, reject) => {
    execFile('tar', ['-czf', join(release, `${name}.tar.gz`), '-C', release, name], (error) => {
      if (error === null) resolve()
      else reject(error)
    })
  })
  await rm(join(release, name), { recursive: true })
}

// createNativeRelease writes the native app's assets for version: the Linux
// archive for arch (the app, its bundled servef, and its icon) and the
// darwin/arm64 zip.
async function createNativeRelease(sandbox: Sandbox, options: { arch?: string; version?: string } = {}): Promise<void> {
  const arch = options.arch ?? 'amd64'
  const version = options.version ?? '0.1.7'
  const release = sandbox.env.STUB_RELEASE_DIR ?? ''
  const staging = join(release, 'native-staging')
  await writeExecutable(join(staging, 'servef-native'), `#!/bin/sh\necho "servef-native ${version} abc123"\n`)
  await writeExecutable(join(staging, 'bin', 'servef'), fakeServef(`${version} (abc123)`))
  await writeFile(join(staging, 'servef-native.png'), 'native png')
  await new Promise<void>((resolve, reject) => {
    execFile('tar', ['-czf', join(release, `servef-native_${version}_linux_${arch}.tar.gz`), '-C', staging, '.'], (error) => {
      if (error === null) resolve()
      else reject(error)
    })
  })
  await rm(staging, { recursive: true })
  await writeFile(join(release, `servef-native_${version}_darwin_arm64.zip`), 'zip')
}

function run(script: string, sandbox: Sandbox, env: Record<string, string> = {}): Promise<Result> {
  return new Promise((resolve) => {
    execFile('/bin/sh', [join(scriptsDir, script)], { env: { ...sandbox.env, ...env } }, (error, stdout, stderr) => {
      const code = error === null ? 0 : typeof error.code === 'number' ? error.code : 1
      resolve({ code, stdout, stderr })
    })
  })
}

async function exists(path: string): Promise<boolean> {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

async function isExecutable(path: string): Promise<boolean> {
  return ((await stat(path)).mode & 0o111) !== 0
}

function assertOk(result: Result): void {
  assert.equal(result.code, 0, `exit ${result.code}\nstdout:\n${result.stdout}\nstderr:\n${result.stderr}`)
}

// --- shared blocks ---

// HANDOVERS only fetch and run the script that replaced them (decision 0092).
const HANDOVERS = new Map([['install-desktop.sh', 'install-electron.sh'], ['install-desktop-edge.sh', 'install-electron-edge.sh']])

void test('shared blocks are identical in every script that carries them', async () => {
  const scripts = (await readdir(scriptsDir)).filter((name) => name.endsWith('.sh') && !HANDOVERS.has(name))
  assert.ok(scripts.length >= 3)
  const blocks = new Map<string, Map<string, string>>()
  for (const script of scripts) {
    const text = await readFile(join(scriptsDir, script), 'utf8')
    const begins = [...text.matchAll(/^# shared:([a-z-]+):begin$/gm)].map((match) => match[1])
    const ends = [...text.matchAll(/^# shared:([a-z-]+):end$/gm)].map((match) => match[1])
    assert.deepEqual(ends, begins, `${script}: unbalanced shared markers`)
    assert.ok(begins.includes('base'), `${script} lacks the base block`)
    for (const match of text.matchAll(/^# shared:([a-z-]+):begin\n([\s\S]*?)^# shared:\1:end$/gm)) {
      const name = match[1] ?? ''
      const copies = blocks.get(name) ?? new Map<string, string>()
      copies.set(script, match[2] ?? '')
      blocks.set(name, copies)
    }
  }
  for (const [name, copies] of blocks) {
    const [first, ...rest] = [...copies]
    if (first === undefined) continue
    for (const [script, body] of rest) {
      assert.equal(body, first[1], `shared:${name} in ${script} differs from ${first[0]}`)
    }
  }
})

// --- install-edge.sh ---

const EDGE_DOWNLOAD = 'https://github.com/flexdinesh/servef/releases/download/edge/'

void test('install-edge.sh installs the CLI from the edge prerelease on PATH', async (t) => {
  for (const [machine, arch] of [['x86_64', 'amd64'], ['aarch64', 'arm64']] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox, { arch, version: '0.0.0-edge' })

    const result = await run('install-edge.sh', sandbox, { STUB_UNAME_M: machine })

    assertOk(result)
    const bin = join(sandbox.home, '.local', 'bin', 'servef')
    assert.ok(await isExecutable(bin))
    assert.match(result.stdout, /downloading servef edge \(linux\/(amd64|arm64)\)/)
    assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.0\.0-edge \(abc123\)\)/)
    assert.doesNotMatch(result.stdout, /not on PATH/)
    const log = await readFile(sandbox.log, 'utf8')
    assert.ok(log.includes(`${EDGE_DOWNLOAD}servef_0.0.0-edge_linux_${arch}.tar.gz`), log)
    assert.doesNotMatch(log, /releases\/latest/)
    assert.doesNotMatch(log, /api\.github\.com/)
  }
})

void test('install-edge.sh prefers ~/bin when only it is on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { version: '0.0.0-edge' })

  const result = await run('install-edge.sh', sandbox, { PATH: `${sandbox.stubs}:${join(sandbox.home, 'bin')}:${SYSTEM_PATH}` })

  assertOk(result)
  assert.ok(await exists(join(sandbox.home, 'bin', 'servef')))
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'servef')), false)
})

void test('install-edge.sh prints the PATH line for the shell and never edits rc files', async (t) => {
  for (const [shell, line] of [
    ['/usr/bin/fish', /fish_add_path ".*\/\.local\/bin"/],
    ['/bin/zsh', /export PATH=".*\/\.local\/bin:\$PATH" {3}# in ~\/\.zshrc/],
    ['/bin/bash', /export PATH=".*\/\.local\/bin:\$PATH" {3}# in ~\/\.bashrc/],
    ['/bin/dash', /export PATH=".*\/\.local\/bin:\$PATH" {3}# in ~\/\.profile/],
  ] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox, { version: '0.0.0-edge' })

    const result = await run('install-edge.sh', sandbox, { PATH: `${sandbox.stubs}:${SYSTEM_PATH}`, SHELL: shell })

    assertOk(result)
    assert.ok(await exists(join(sandbox.home, '.local', 'bin', 'servef')))
    assert.match(result.stdout, /\.local\/bin is not on PATH; add it:/)
    assert.match(result.stdout, line)
    assert.deepEqual(await readdir(sandbox.home), ['.local'], `${shell}: only ~/.local is written`)
  }
})

void test('install-edge.sh reports an update, a shadowing servef, and a running daemon', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { version: '0.0.0-edge' })
  await writeExecutable(join(sandbox.home, '.local', 'bin', 'servef'), fakeServef('0.1.6'))
  const earlier = join(sandbox.root, 'brew-bin')
  await writeExecutable(join(earlier, 'servef'), fakeServef('0.1.5'))

  const result = await run('install-edge.sh', sandbox, {
    PATH: `${sandbox.stubs}:${earlier}:${join(sandbox.home, '.local', 'bin')}:${SYSTEM_PATH}`,
    STUB_DAEMON_STATUS: '0',
  })

  assertOk(result)
  assert.match(result.stdout, /updated .*\/\.local\/bin\/servef \(servef 0\.1\.6 -> servef 0\.0\.0-edge/)
  assert.match(result.stdout, /brew-bin\/servef comes first on PATH and is not the servef just installed/)
  assert.match(result.stdout, /servef daemon stop/)
})

void test('install-edge.sh stops without its archive or on an unsupported platform', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const missing = await run('install-edge.sh', sandbox)
  assert.notEqual(missing.code, 0)
  assert.match(missing.stderr, /could not download servef_0\.0\.0-edge_linux_amd64\.tar\.gz from servef edge/)

  const windows = await run('install-edge.sh', sandbox, { STUB_UNAME_S: 'MINGW64_NT-10.0' })
  assert.notEqual(windows.code, 0)
  assert.match(windows.stderr, /unsupported OS MINGW64_NT-10\.0/)
  const i686 = await run('install-edge.sh', sandbox, { STUB_UNAME_M: 'i686' })
  assert.notEqual(i686.code, 0)
  assert.match(i686.stderr, /unsupported architecture i686/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- install-electron-edge.sh ---

void test('install-electron-edge.sh installs the Linux AppImage, registers servef://, and adds folder actions', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t), { homeName: 'home 50%' })
  await createRelease(sandbox, { desktop: true, version: '0.0.0-edge' })
  const share = join(sandbox.home, '.local', 'share')
  await writeExecutable(join(share, 'servef-electron', 'app', 'servef-electron'), 'unpacked build')

  const result = await run('install-electron-edge.sh', sandbox)

  assertOk(result)
  const appImage = join(share, 'servef-electron', 'servef-electron.AppImage')
  assert.equal(await readFile(appImage, 'utf8'), 'appimage')
  assert.ok(await isExecutable(appImage))
  assert.equal(await exists(join(share, 'servef-electron', 'app')), false, 'the AppImage replaces an unpacked build from an earlier edge script')
  assert.ok(await isExecutable(join(sandbox.home, '.local', 'bin', 'servef')))
  assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.0\.0-edge/)

  const exec = `"${appImage.replaceAll('%', '%%')}"`
  const entry = await readFile(join(share, 'applications', 'servef-electron.desktop'), 'utf8')
  assert.match(entry, /^MimeType=x-scheme-handler\/servef;$/m)
  assert.doesNotMatch(entry, /inode\/directory/)
  assert.ok(entry.includes(`\nExec=${exec} %U\n`), entry)
  const dolphin = join(share, 'kio', 'servicemenus', 'servef-electron.desktop')
  assert.ok((await readFile(dolphin, 'utf8')).includes(`\nExec=${exec} --open-path %F\n`))
  assert.ok(await isExecutable(dolphin), 'KDE runs only executable user service menus')
  const nemo = await readFile(join(share, 'nemo', 'actions', 'servef-electron.nemo_action'), 'utf8')
  assert.ok(nemo.includes(`\nExec=${exec} --open-path %F\n`))
  assert.match(nemo, /^Extensions=dir;$/m)
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-electron.png'), 'utf8'), 'png')

  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes(`${EDGE_DOWNLOAD}servef-electron_0.0.0-edge_linux_amd64.AppImage`), log)
  assert.ok(log.includes(`${EDGE_DOWNLOAD}servef_0.0.0-edge_linux_amd64.tar.gz`), log)
  assert.ok(log.includes('https://raw.githubusercontent.com/flexdinesh/servef/edge/apps/electron/resources/icon.png'), log)
  assert.doesNotMatch(log, /releases\/latest/)
  assert.match(log, /^xdg-mime default servef-electron\.desktop x-scheme-handler\/servef$/m)
  assert.match(log, /^update-desktop-database .*\/applications$/m)
  assert.match(result.stdout, /servef \./)
})

void test('install-electron-edge.sh installs servef.app on macOS without Linux registration', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { desktop: true, version: '0.0.0-edge' })
  await createDarwinArchive(sandbox, '0.0.0-edge')

  const result = await run('install-electron-edge.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'arm64' })

  assertOk(result)
  const apps = (await exists('/Applications')) ? '/Applications' : join(sandbox.home, 'Applications')
  t.after(async () => {
    if (apps === '/Applications') await rm('/Applications/servef.app', { recursive: true, force: true })
  })
  assert.equal(await readFile(join(apps, 'servef.app', 'Contents', 'MacOS', 'servef'), 'utf8'), 'app')
  assert.ok(await exists(join(sandbox.home, '.local', 'bin', 'servef')))
  assert.equal(await exists(join(sandbox.home, '.local', 'share')), false)
  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes(`${EDGE_DOWNLOAD}servef-electron_0.0.0-edge_darwin_arm64.zip`), log)
  assert.ok(log.includes(`${EDGE_DOWNLOAD}servef_0.0.0-edge_darwin_arm64.tar.gz`), log)
  assert.doesNotMatch(log, /xdg-mime/)
})

void test('install-electron-edge.sh stops before installing when it cannot', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { version: '0.0.0-edge' })

  const noPackage = await run('install-electron-edge.sh', sandbox)
  assert.notEqual(noPackage.code, 0)
  assert.match(noPackage.stderr, /servef edge has no servef-electron_0\.0\.0-edge_linux_amd64\.AppImage/)

  const running = await run('install-electron-edge.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(running.code, 0)
  assert.match(running.stderr, /the servef desktop app is running; quit it, then run this again/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- install-native.sh, install-native-edge.sh ---

void test('install-native-edge.sh installs the Linux app, registers servef-native://, and links it on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t), { homeName: 'home 50%' })
  await createNativeRelease(sandbox, { version: '0.0.0-edge' })
  const share = join(sandbox.home, '.local', 'share')
  await writeExecutable(join(share, 'servef-native', 'stale'), 'earlier build')

  const result = await run('install-native-edge.sh', sandbox)

  assertOk(result)
  const app = join(share, 'servef-native')
  assert.ok(await isExecutable(join(app, 'servef-native')))
  assert.ok(await isExecutable(join(app, 'bin', 'servef')), 'the app bundles the servef it runs the daemon with')
  assert.equal(await exists(join(app, 'stale')), false, 'a new install replaces the earlier one')
  assert.match(result.stdout, /installed .*servef-native \(servef-native 0\.0\.0-edge abc123\)/)
  const link = join(sandbox.home, '.local', 'bin', 'servef-native')
  assert.equal((await readFile(link, 'utf8')).includes('servef-native 0.0.0-edge'), true)
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'servef')), false, 'the CLI is not installed')

  const exec = `"${join(app, 'servef-native').replaceAll('%', '%%')}"`
  const entry = await readFile(join(share, 'applications', 'servef-native.desktop'), 'utf8')
  assert.match(entry, /^MimeType=x-scheme-handler\/servef-native;$/m)
  assert.ok(entry.includes(`\nExec=${exec} %U\n`), entry)
  assert.match(entry, /^Icon=servef-native$/m)
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-native.png'), 'utf8'), 'native png')

  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes(`${EDGE_DOWNLOAD}servef-native_0.0.0-edge_linux_amd64.tar.gz`), log)
  assert.doesNotMatch(log, /servef_0\.0\.0-edge_linux_amd64\.tar\.gz/)
  assert.doesNotMatch(log, /releases\/latest/)
  assert.match(log, /^xdg-mime default servef-native\.desktop x-scheme-handler\/servef-native$/m)
  assert.doesNotMatch(log, /x-scheme-handler\/servef$/m)
  assert.match(result.stdout, /servef-native \./)
})

void test('install-native.sh installs the latest release for the machine', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createNativeRelease(sandbox, { arch: 'arm64' })

  const result = await run('install-native.sh', sandbox, { STUB_UNAME_M: 'aarch64' })

  assertOk(result)
  assert.ok(await isExecutable(join(sandbox.home, '.local', 'share', 'servef-native', 'servef-native')))
  const log = await readFile(sandbox.log, 'utf8')
  assert.match(log, /-w %\{redirect_url\} https:\/\/github\.com\/flexdinesh\/servef\/releases\/latest/)
  assert.ok(log.includes('https://github.com/flexdinesh/servef/releases/download/v0.1.7/servef-native_0.1.7_linux_arm64.tar.gz'), log)
})

void test('install-native.sh installs servef-native.app on macOS and links its executable', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createNativeRelease(sandbox)

  const result = await run('install-native.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'arm64' })

  assertOk(result)
  const apps = (await exists('/Applications')) ? '/Applications' : join(sandbox.home, 'Applications')
  t.after(async () => {
    if (apps === '/Applications') await rm('/Applications/servef-native.app', { recursive: true, force: true })
  })
  const executable = join(apps, 'servef-native.app', 'Contents', 'MacOS', 'servef-native')
  assert.equal(await readFile(executable, 'utf8'), 'app')
  assert.equal(await readFile(join(sandbox.home, '.local', 'bin', 'servef-native'), 'utf8'), 'app')
  assert.equal(await exists(join(sandbox.home, '.local', 'share')), false)
  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes('https://github.com/flexdinesh/servef/releases/download/v0.1.7/servef-native_0.1.7_darwin_arm64.zip'), log)
  assert.doesNotMatch(log, /xdg-mime/)
})

void test('install-native.sh stops on a release without the native app or while it runs', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox)

  const noPackage = await run('install-native.sh', sandbox)
  assert.notEqual(noPackage.code, 0)
  assert.match(noPackage.stderr, /servef v0\.1\.7 has no servef-native_0\.1\.7_linux_amd64\.tar\.gz; releases before the native app have none/)

  const running = await run('install-native-edge.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(running.code, 0)
  assert.match(running.stderr, /the servef native app is running; quit it, then run this again/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- install.sh ---

void test('install.sh installs the CLI from the release the latest page redirects to', async (t) => {
  for (const [machine, arch] of [['x86_64', 'amd64'], ['aarch64', 'arm64']] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox, { arch })

    const result = await run('install.sh', sandbox, { STUB_UNAME_M: machine })

    assertOk(result)
    assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.1\.7 \(abc123\)\)/)
    const log = await readFile(sandbox.log, 'utf8')
    assert.match(log, /-w %\{redirect_url\} https:\/\/github\.com\/flexdinesh\/servef\/releases\/latest/)
    assert.ok(log.includes(`https://github.com/flexdinesh/servef/releases/download/v0.1.7/servef_0.1.7_linux_${arch}.tar.gz`), log)
    assert.doesNotMatch(log, /api\.github\.com/)
  }
})

// systemPathWithout links every /usr/bin and /bin program except names into
// one directory, so a real system copy cannot stand in for a removed stub.
async function systemPathWithout(sandbox: Sandbox, names: string[]): Promise<string> {
  const dir = join(sandbox.root, 'system')
  await mkdir(dir, { recursive: true })
  for (const source of SYSTEM_PATH.split(':')) {
    for (const name of await readdir(source)) {
      if (names.includes(name) || (await exists(join(dir, name)))) continue
      await symlink(join(source, name), join(dir, name))
    }
  }
  return dir
}

void test('install.sh resolves the latest release with wget when curl is missing', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t), { without: ['curl'] })
  await createRelease(sandbox)
  const system = await systemPathWithout(sandbox, ['curl'])

  const result = await run('install.sh', sandbox, { PATH: `${sandbox.stubs}:${join(sandbox.home, '.local', 'bin')}:${system}` })

  assertOk(result)
  assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.1\.7 \(abc123\)\)/)
  assert.doesNotMatch(await readFile(sandbox.log, 'utf8'), /^curl /m)
  assert.match(await readFile(sandbox.log, 'utf8'), /wget -S --max-redirect=0 -O \/dev\/null https:\/\/github\.com\/flexdinesh\/servef\/releases\/latest/)
})

void test('install.sh stops when the latest release or its archive is missing', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

  const noLatest = await run('install.sh', sandbox, { STUB_LATEST: '' })
  assert.notEqual(noLatest.code, 0)
  assert.match(noLatest.stderr, /could not find the latest servef release at https:\/\/github\.com\/flexdinesh\/servef\/releases\/latest/)

  const noArchive = await run('install.sh', sandbox)
  assert.notEqual(noArchive.code, 0)
  assert.match(noArchive.stderr, /could not download servef_0\.1\.7_linux_amd64\.tar\.gz from servef v0\.1\.7/)
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'servef')), false)
})

// --- install-electron.sh ---

void test('install-electron.sh installs the Linux AppImage, registers it, and installs the CLI', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { desktop: true })
  const share = join(sandbox.home, '.local', 'share')
  await writeExecutable(join(share, 'servef-electron', 'app', 'servef-electron'), 'edge build')

  const result = await run('install-electron.sh', sandbox)

  assertOk(result)
  const appImage = join(share, 'servef-electron', 'servef-electron.AppImage')
  assert.equal(await readFile(appImage, 'utf8'), 'appimage')
  assert.ok(await isExecutable(appImage))
  assert.equal(await exists(join(share, 'servef-electron', 'app')), false, 'the AppImage replaces an unpacked build from an earlier edge script')
  const entry = await readFile(join(share, 'applications', 'servef-electron.desktop'), 'utf8')
  assert.ok(entry.includes(`\nExec=${appImage} %U\n`), entry)
  assert.ok((await readFile(join(share, 'kio', 'servicemenus', 'servef-electron.desktop'), 'utf8')).includes(`\nExec=${appImage} --open-path %F\n`))
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-electron.png'), 'utf8'), 'png')
  assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.1\.7/)
  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes('https://github.com/flexdinesh/servef/releases/download/v0.1.7/servef-electron_0.1.7_linux_amd64.AppImage'))
  assert.ok(log.includes('https://raw.githubusercontent.com/flexdinesh/servef/v0.1.7/apps/electron/resources/icon.png'))
  assert.match(log, /^xdg-mime default servef-electron\.desktop x-scheme-handler\/servef$/m)
})

void test('install-electron.sh fetches the icon from earlier layouts for earlier releases', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { desktop: true })
  const icon = (dir: string): string => `https://raw.githubusercontent.com/flexdinesh/servef/v0.1.7/${dir}/resources/icon.png`

  const result = await run('install-electron.sh', sandbox, { STUB_CURL_MISSING: `${icon('apps/electron')} ${icon('apps/desktop')}` })

  assertOk(result)
  const share = join(sandbox.home, '.local', 'share')
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-electron.png'), 'utf8'), 'png')
  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes(icon('apps/electron')))
  assert.ok(log.includes(icon('apps/desktop')))
  assert.ok(log.includes(icon('desktop')))
})

void test('install-electron.sh replaces a servef-desktop install and leaves its login entry and data', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { desktop: true })
  const share = join(sandbox.home, '.local', 'share')
  const old = [
    join(share, 'servef-desktop', 'servef-desktop.AppImage'),
    join(share, 'applications', 'servef-desktop.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-desktop.png'),
    join(share, 'kio', 'servicemenus', 'servef-desktop.desktop'),
    join(share, 'nemo', 'actions', 'servef-desktop.nemo_action'),
  ]
  for (const path of old) await writeExecutable(path, 'old')
  const config = join(sandbox.home, '.config')
  const autostart = join(config, 'autostart', 'servef-desktop.desktop')
  const appData = join(config, 'servef-desktop', 'recent-folders.json')
  await writeExecutable(autostart, 'old')
  await writeExecutable(appData, '[]')

  const result = await run('install-electron.sh', sandbox)

  assertOk(result)
  assert.match(result.stdout, /replaced the earlier servef-desktop install/)
  for (const path of old) assert.equal(await exists(path), false, `${path} is removed`)
  assert.ok(await exists(join(share, 'servef-electron', 'servef-electron.AppImage')))
  assert.match(await readFile(sandbox.log, 'utf8'), /^xdg-mime default servef-electron\.desktop x-scheme-handler\/servef$/m)
  assert.ok(await exists(autostart), 'the app moves the login entry when it starts')
  assert.ok(await exists(appData), 'app data keeps its name')
})

for (const [moved, target] of HANDOVERS) {
  for (const tool of ['curl', 'wget']) {
    void test(`${moved} hands over to ${target} with ${tool}, since earlier builds print its URL`, async (t) => {
      const sandbox = await createSandbox(t.after.bind(t), { without: tool === 'wget' ? ['curl'] : [] })
      await createRelease(sandbox, { desktop: true, version: target.includes('edge') ? '0.0.0-edge' : '0.1.7' })
      await writeFile(join(sandbox.env.STUB_RELEASE_DIR ?? '', target), await readFile(join(scriptsDir, target)))
      const env: Record<string, string> = tool === 'wget' ? { PATH: `${sandbox.stubs}:${join(sandbox.home, '.local', 'bin')}:${await systemPathWithout(sandbox, ['curl'])}` } : {}

      const result = await run(moved, sandbox, env)

      assertOk(result)
      assert.ok((await readFile(sandbox.log, 'utf8')).includes(`https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/${target}`))
      assert.ok(await exists(join(sandbox.home, '.local', 'share', 'servef-electron', 'servef-electron.AppImage')))
    })
  }
}

void test('install-desktop.sh fails when the script it hands over to cannot be fetched', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const result = await run('install-desktop.sh', sandbox)
  assert.notEqual(result.code, 0)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

void test('install-electron.sh stops on a release without desktop packages or while the app runs', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox)

  const noPackage = await run('install-electron.sh', sandbox)
  assert.notEqual(noPackage.code, 0)
  assert.match(noPackage.stderr, /servef v0\.1\.7 has no servef-electron_0\.1\.7_linux_amd64\.AppImage; releases before desktop packages have none/)

  const running = await run('install-electron.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(running.code, 0)
  assert.match(running.stderr, /the servef desktop app is running/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- uninstall.sh ---

void test('uninstall.sh removes every install, reports package-managed copies, and keeps user data', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { desktop: true, version: '0.0.0-edge' })
  assertOk(await run('install-electron-edge.sh', sandbox))
  const manual = join(sandbox.home, 'Applications', 'servef-electron_0.1.7_linux_amd64.AppImage')
  await writeExecutable(manual, 'appimage')
  const oldManual = join(sandbox.home, 'Applications', 'servef-desktop_0.0.0-edge_linux_amd64.AppImage')
  await writeExecutable(oldManual, 'appimage')
  const unrelated = join(sandbox.home, 'Applications', 'other.AppImage')
  await writeExecutable(unrelated, 'other')
  const config = join(sandbox.home, '.config')
  await mkdir(join(config, 'autostart'), { recursive: true })
  await writeFile(join(config, 'autostart', 'servef-electron.desktop'), `[Desktop Entry]\nExec=${manual}\n`)
  await writeFile(join(config, 'autostart', 'servef-desktop.desktop'), `[Desktop Entry]\nExec=${oldManual}\n`)
  const share = join(sandbox.home, '.local', 'share')
  const oldInstall = [
    join(share, 'servef-desktop', 'servef-desktop.AppImage'),
    join(share, 'applications', 'servef-desktop.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-desktop.png'),
    join(share, 'kio', 'servicemenus', 'servef-desktop.desktop'),
    join(share, 'nemo', 'actions', 'servef-desktop.nemo_action'),
  ]
  for (const path of oldInstall) await writeExecutable(path, 'old')
  await writeExecutable(join(sandbox.home, '.local', 'share', 'nautilus', 'scripts', 'Open in servef'), '#!/bin/sh\n')
  await writeExecutable(join(sandbox.home, 'go', 'bin', 'servef'), fakeServef('0.1.6'))
  await mkdir(join(config, 'servef'), { recursive: true })
  await writeFile(join(config, 'servef', 'roots.json'), '[]')
  await mkdir(join(config, 'servef-desktop'), { recursive: true })
  await mkdir(join(sandbox.home, '.cache', 'servef'), { recursive: true })
  const shims = join(sandbox.root, 'mise', 'shims')
  await writeExecutable(join(shims, 'servef'), fakeServef('0.1.6'))

  const result = await run('uninstall.sh', sandbox, { PATH: `${sandbox.stubs}:${join(sandbox.home, '.local', 'bin')}:${shims}:${SYSTEM_PATH}` })

  assertOk(result)
  for (const path of [
    ...oldInstall,
    join(sandbox.home, '.local', 'bin', 'servef'),
    join(sandbox.home, 'go', 'bin', 'servef'),
    join(share, 'servef-electron'),
    join(share, 'applications', 'servef-electron.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-electron.png'),
    join(share, 'kio', 'servicemenus', 'servef-electron.desktop'),
    join(share, 'nemo', 'actions', 'servef-electron.nemo_action'),
    join(share, 'nautilus', 'scripts', 'Open in servef'),
    join(config, 'autostart', 'servef-electron.desktop'),
    join(config, 'autostart', 'servef-desktop.desktop'),
    manual,
    oldManual,
  ]) {
    assert.equal(await exists(path), false, `${path} is removed`)
  }
  assert.ok(await exists(unrelated), 'only servef-electron and servef-desktop AppImages are removed')
  for (const path of [join(config, 'servef', 'roots.json'), join(config, 'servef-desktop'), join(sandbox.home, '.cache', 'servef')]) {
    assert.ok(await exists(path), `${path} is kept`)
  }
  assert.match(result.stdout, /mise\/shims\/servef is installed by mise/)
  assert.match(result.stdout, /kept your data/)
  assert.ok(result.stdout.includes(join(config, 'servef')))
})

void test('uninstall.sh removes the native app and keeps its data', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createNativeRelease(sandbox, { version: '0.0.0-edge' })
  assertOk(await run('install-native-edge.sh', sandbox))
  const config = join(sandbox.home, '.config', 'servef-native')
  await mkdir(config, { recursive: true })

  const result = await run('uninstall.sh', sandbox)

  assertOk(result)
  const share = join(sandbox.home, '.local', 'share')
  for (const path of [
    join(sandbox.home, '.local', 'bin', 'servef-native'),
    join(share, 'servef-native'),
    join(share, 'applications', 'servef-native.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-native.png'),
  ]) {
    assert.equal(await exists(path), false, `${path} is removed`)
  }
  assert.ok(await exists(config), 'native app data is kept')
  assert.ok(result.stdout.includes(config))
})

void test('uninstall.sh stops while servef runs and reports an empty home', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const bin = join(sandbox.home, '.local', 'bin', 'servef')
  await writeExecutable(bin, fakeServef('0.1.6'))

  const daemon = await run('uninstall.sh', sandbox, { STUB_DAEMON_STATUS: '0' })
  assert.notEqual(daemon.code, 0)
  assert.match(daemon.stderr, /a servef daemon is running; stop it with 'servef daemon stop'/)
  const app = await run('uninstall.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(app.code, 0)
  assert.match(app.stderr, /the servef desktop app is running/)
  assert.ok(await exists(bin))

  const empty = await createSandbox(t.after.bind(t))
  const result = await run('uninstall.sh', empty)
  assertOk(result)
  assert.match(result.stdout, /nothing to remove/)
})
