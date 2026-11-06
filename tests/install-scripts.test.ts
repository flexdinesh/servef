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
  // macOS only: extracts a release zip as srcz-desktop.app, or copies a
  // directory.
  ditto: [
    'echo "ditto $*" >> "$STUB_LOG"',
    'if [ "$1" = -x ]; then',
    '  mkdir -p "$4/srcz-desktop.app/Contents/MacOS"',
    "  printf 'app' > \"$4/srcz-desktop.app/Contents/MacOS/srcz-desktop\"",
    'else',
    '  cp -R "$1" "$2"',
    'fi',
  ],
  'xdg-mime': ['echo "xdg-mime $*" >> "$STUB_LOG"'],
  'update-desktop-database': ['echo "update-desktop-database $*" >> "$STUB_LOG"'],
  pgrep: ['exit "${STUB_PGREP:-1}"'],
  ldconfig: ['echo "${STUB_LDCONFIG:-}"'],
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

function fakeSrcz(version: string): string {
  return [
    '#!/bin/sh',
    'case "$1" in',
    `  --version) echo "srcz ${version}" ;;`,
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
  const root = await mkdtemp(join(testRoot, 'srcz-install-'))
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
      STUB_LATEST: 'https://github.com/flexdinesh/srcz/releases/tag/v0.1.7',
    },
  }
}

// createRelease writes a release's assets for linux/<arch>, named for version
// (0.1.7 by default; the edge prerelease uses 0.0.0-edge): the CLI archive.
async function createRelease(
  sandbox: Sandbox,
  options: { arch?: string; version?: string } = {},
): Promise<void> {
  const arch = options.arch ?? 'amd64'
  const version = options.version ?? '0.1.7'
  const release = sandbox.env.STUB_RELEASE_DIR ?? ''
  const name = `srcz_${version}_linux_${arch}`
  await writeExecutable(join(release, name, 'srcz'), fakeSrcz(`${version} (abc123)`))
  await new Promise<void>((resolve, reject) => {
    execFile('tar', ['-czf', join(release, `${name}.tar.gz`), '-C', release, name], (error) => {
      if (error === null) resolve()
      else reject(error)
    })
  })
  await rm(join(release, name), { recursive: true })
}

// createDesktopRelease writes the desktop app's assets for version: the Linux
// archive for arch (the app, its bundled srcz, and its icon) and the
// darwin/arm64 zip.
async function createDesktopRelease(sandbox: Sandbox, options: { arch?: string; version?: string } = {}): Promise<void> {
  const arch = options.arch ?? 'amd64'
  const version = options.version ?? '0.1.7'
  const release = sandbox.env.STUB_RELEASE_DIR ?? ''
  const staging = join(release, 'desktop-staging')
  await writeExecutable(join(staging, 'srcz-desktop'), `#!/bin/sh\necho "srcz-desktop ${version} abc123"\n`)
  await writeExecutable(join(staging, 'bin', 'srcz'), fakeSrcz(`${version} (abc123)`))
  await writeFile(join(staging, 'srcz-desktop.png'), 'desktop png')
  await new Promise<void>((resolve, reject) => {
    execFile('tar', ['-czf', join(release, `srcz-desktop_${version}_linux_${arch}.tar.gz`), '-C', staging, '.'], (error) => {
      if (error === null) resolve()
      else reject(error)
    })
  })
  await rm(staging, { recursive: true })
  await writeFile(join(release, `srcz-desktop_${version}_darwin_arm64.zip`), 'zip')
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

void test('shared blocks are identical in every script that carries them', async () => {
  const scripts = (await readdir(scriptsDir)).filter((name) => name.endsWith('.sh'))
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

const EDGE_DOWNLOAD = 'https://github.com/flexdinesh/srcz/releases/download/edge/'

void test('edge install scripts stop on macOS Intel before downloading', async (t) => {
  for (const script of ['install-edge.sh', 'install-desktop-edge.sh']) {
    const sandbox = await createSandbox(t.after.bind(t))

    const result = await run(script, sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'x86_64' })

    assert.notEqual(result.code, 0, script)
    assert.match(result.stderr, /edge has no macOS Intel builds; use the stable install script/, script)
    assert.doesNotMatch(await readFile(sandbox.log, 'utf8'), /releases\/download/, script)
    assert.equal(await exists(join(sandbox.home, '.local')), false, script)
  }
})

void test('stable install.sh still targets macOS Intel', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

  await run('install.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'x86_64' })

  assert.ok((await readFile(sandbox.log, 'utf8')).includes('/releases/download/v0.1.7/srcz_0.1.7_darwin_amd64.tar.gz'))
})

void test('install-edge.sh installs the CLI from the edge prerelease on PATH', async (t) => {
  for (const [machine, arch] of [['x86_64', 'amd64'], ['aarch64', 'arm64']] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox, { arch, version: '0.0.0-edge' })

    const result = await run('install-edge.sh', sandbox, { STUB_UNAME_M: machine })

    assertOk(result)
    const bin = join(sandbox.home, '.local', 'bin', 'srcz')
    assert.ok(await isExecutable(bin))
    assert.match(result.stdout, /downloading srcz edge \(linux\/(amd64|arm64)\)/)
    assert.match(result.stdout, /installed .*\/\.local\/bin\/srcz \(srcz 0\.0\.0-edge \(abc123\)\)/)
    assert.doesNotMatch(result.stdout, /not on PATH/)
    const log = await readFile(sandbox.log, 'utf8')
    assert.ok(log.includes(`${EDGE_DOWNLOAD}srcz_0.0.0-edge_linux_${arch}.tar.gz`), log)
    assert.doesNotMatch(log, /releases\/latest/)
    assert.doesNotMatch(log, /api\.github\.com/)
  }
})

void test('install-edge.sh prefers ~/bin when only it is on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { version: '0.0.0-edge' })

  const result = await run('install-edge.sh', sandbox, { PATH: `${sandbox.stubs}:${join(sandbox.home, 'bin')}:${SYSTEM_PATH}` })

  assertOk(result)
  assert.ok(await exists(join(sandbox.home, 'bin', 'srcz')))
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'srcz')), false)
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
    assert.ok(await exists(join(sandbox.home, '.local', 'bin', 'srcz')))
    assert.match(result.stdout, /\.local\/bin is not on PATH; add it:/)
    assert.match(result.stdout, line)
    assert.deepEqual(await readdir(sandbox.home), ['.local'], `${shell}: only ~/.local is written`)
  }
})

void test('install-edge.sh reports an update, a shadowing srcz, and a running daemon', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createRelease(sandbox, { version: '0.0.0-edge' })
  await writeExecutable(join(sandbox.home, '.local', 'bin', 'srcz'), fakeSrcz('0.1.6'))
  const earlier = join(sandbox.root, 'brew-bin')
  await writeExecutable(join(earlier, 'srcz'), fakeSrcz('0.1.5'))

  const result = await run('install-edge.sh', sandbox, {
    PATH: `${sandbox.stubs}:${earlier}:${join(sandbox.home, '.local', 'bin')}:${SYSTEM_PATH}`,
    STUB_DAEMON_STATUS: '0',
  })

  assertOk(result)
  assert.match(result.stdout, /updated .*\/\.local\/bin\/srcz \(srcz 0\.1\.6 -> srcz 0\.0\.0-edge/)
  assert.match(result.stdout, /brew-bin\/srcz comes first on PATH and is not the srcz just installed/)
  assert.match(result.stdout, /srcz daemon stop/)
})

void test('install-edge.sh stops without its archive or on an unsupported platform', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const missing = await run('install-edge.sh', sandbox)
  assert.notEqual(missing.code, 0)
  assert.match(missing.stderr, /could not download srcz_0\.0\.0-edge_linux_amd64\.tar\.gz from srcz edge/)

  const windows = await run('install-edge.sh', sandbox, { STUB_UNAME_S: 'MINGW64_NT-10.0' })
  assert.notEqual(windows.code, 0)
  assert.match(windows.stderr, /unsupported OS MINGW64_NT-10\.0/)
  const i686 = await run('install-edge.sh', sandbox, { STUB_UNAME_M: 'i686' })
  assert.notEqual(i686.code, 0)
  assert.match(i686.stderr, /unsupported architecture i686/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- install-desktop.sh, install-desktop-edge.sh ---

void test('install-desktop-edge.sh installs the Linux app, registers srcz://, and links it on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t), { homeName: 'home 50%' })
  await createDesktopRelease(sandbox, { version: '0.0.0-edge' })
  const share = join(sandbox.home, '.local', 'share')
  await writeExecutable(join(share, 'srcz-desktop', 'app', 'stale'), 'earlier build')
  const storage = join(share, 'srcz-desktop', 'localstorage', 'page.localstorage')
  await mkdir(dirname(storage), { recursive: true })
  await writeFile(storage, 'tabs')

  const result = await run('install-desktop-edge.sh', sandbox)

  assertOk(result)
  const app = join(share, 'srcz-desktop', 'app')
  assert.ok(await isExecutable(join(app, 'srcz-desktop')))
  assert.ok(await isExecutable(join(app, 'bin', 'srcz')), 'the app bundles the srcz it runs the daemon with')
  assert.equal(await exists(join(app, 'stale')), false, 'a new install replaces the earlier one')
  assert.equal(await readFile(storage, 'utf8'), 'tabs', 'an install keeps the page state WebKit stores')
  assert.match(result.stdout, /installed .*srcz-desktop\/app \(srcz-desktop 0\.0\.0-edge abc123\)/)
  const link = join(sandbox.home, '.local', 'bin', 'srcz-desktop')
  assert.equal((await readFile(link, 'utf8')).includes('srcz-desktop 0.0.0-edge'), true)
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'srcz')), false, 'the CLI is not installed')

  const exec = `"${join(app, 'srcz-desktop').replaceAll('%', '%%')}"`
  const entry = await readFile(join(share, 'applications', 'srcz-desktop.desktop'), 'utf8')
  assert.match(entry, /^MimeType=x-scheme-handler\/srcz;$/m)
  assert.doesNotMatch(entry, /inode\/directory/)
  assert.ok(entry.includes(`\nExec=${exec} %U\n`), entry)
  assert.match(entry, /^Icon=srcz-desktop$/m)
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'srcz-desktop.png'), 'utf8'), 'desktop png')

  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes(`${EDGE_DOWNLOAD}srcz-desktop_0.0.0-edge_linux_amd64.tar.gz`), log)
  assert.doesNotMatch(log, /srcz_0\.0\.0-edge_linux_amd64\.tar\.gz/)
  assert.doesNotMatch(log, /releases\/latest/)
  assert.match(log, /^xdg-mime default srcz-desktop\.desktop x-scheme-handler\/srcz$/m)
  assert.ok(await exists(join(sandbox.home, '.config')), 'xdg-mime writes mimeapps.list there and does not create it')
  assert.match(log, /^update-desktop-database .*\/applications$/m)
  assert.match(result.stdout, /srcz-desktop \./)
  assert.match(result.stdout, /scripts\/install\.sh/)
})

void test('install-desktop.sh warns without WebKitGTK and installs anyway', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox)

  const missing = await run('install-desktop.sh', sandbox)
  assertOk(missing)
  assert.match(missing.stdout, /WebKitGTK 4\.1 not found/)
  assert.match(missing.stdout, /apt install libwebkit2gtk-4\.1-0/)
  assert.ok(await isExecutable(join(sandbox.home, '.local', 'share', 'srcz-desktop', 'app', 'srcz-desktop')))

  const present = await run('install-desktop.sh', sandbox, { STUB_LDCONFIG: 'libwebkit2gtk-4.1.so.0 (libc6,x86-64) => /usr/lib/libwebkit2gtk-4.1.so.0' })
  assertOk(present)
  assert.doesNotMatch(present.stdout, /WebKitGTK/)
})

void test('install-desktop.sh installs the latest release for the machine', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox, { arch: 'arm64' })

  const result = await run('install-desktop.sh', sandbox, { STUB_UNAME_M: 'aarch64' })

  assertOk(result)
  assert.ok(await isExecutable(join(sandbox.home, '.local', 'share', 'srcz-desktop', 'app', 'srcz-desktop')))
  const log = await readFile(sandbox.log, 'utf8')
  assert.match(log, /-w %\{redirect_url\} https:\/\/github\.com\/flexdinesh\/srcz\/releases\/latest/)
  assert.ok(log.includes('https://github.com/flexdinesh/srcz/releases/download/v0.1.7/srcz-desktop_0.1.7_linux_arm64.tar.gz'), log)
})

void test('install-desktop.sh installs srcz-desktop.app on macOS and links it', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox)
  const apps = (await exists('/Applications')) ? '/Applications' : join(sandbox.home, 'Applications')
  t.after(async () => {
    if (apps === '/Applications') await rm('/Applications/srcz-desktop.app', { recursive: true, force: true })
  })
  const result = await run('install-desktop.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'arm64' })

  assertOk(result)
  const executable = join(apps, 'srcz-desktop.app', 'Contents', 'MacOS', 'srcz-desktop')
  assert.equal(await readFile(executable, 'utf8'), 'app')
  assert.equal(await readFile(join(sandbox.home, '.local', 'bin', 'srcz-desktop'), 'utf8'), 'app')
  assert.equal(await exists(join(sandbox.home, '.local', 'share')), false)
  const log = await readFile(sandbox.log, 'utf8')
  assert.ok(log.includes('https://github.com/flexdinesh/srcz/releases/download/v0.1.7/srcz-desktop_0.1.7_darwin_arm64.zip'), log)
  assert.doesNotMatch(log, /xdg-mime/)
})

void test('install-desktop.sh explains a release without the desktop app and changes nothing', async (t) => {
  for (const [os, machine, asset] of [['Linux', 'x86_64', 'srcz-desktop_0\\.1\\.7_linux_amd64\\.tar\\.gz'], ['Darwin', 'arm64', 'srcz-desktop_0\\.1\\.7_darwin_arm64\\.zip']] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox)
    const old = join(sandbox.home, '.local', 'share', 'srcz-desktop', 'app', 'srcz-desktop')
    await writeExecutable(old, 'old')

    const result = await run('install-desktop.sh', sandbox, { STUB_UNAME_S: os, STUB_UNAME_M: machine })

    assert.notEqual(result.code, 0)
    assert.match(result.stderr, new RegExp(`srcz v0\\.1\\.7 has no ${asset}; earlier releases have no desktop app; it ships in the next stable release and on edge, except macOS Intel`))
    assert.ok(await exists(old), 'a failed install leaves the installed app alone')
  }
})

void test('install-desktop scripts stop while the app runs', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox, { version: '0.0.0-edge' })

  const running = await run('install-desktop-edge.sh', sandbox, { STUB_PGREP: '0' })

  assert.notEqual(running.code, 0)
  assert.match(running.stderr, /the srcz desktop app is running; quit it, then run this again/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

void test('install-desktop.sh overwrites an earlier desktop entry', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox)
  const share = join(sandbox.home, '.local', 'share')
  const oldEntry = join(share, 'applications', 'srcz-desktop.desktop')
  await writeExecutable(oldEntry, 'old')

  const result = await run('install-desktop.sh', sandbox)

  assertOk(result)
  assert.match(await readFile(oldEntry, 'utf8'), /srcz-desktop\/app\/srcz-desktop %U/, 'the desktop entry is overwritten')
  assert.ok(await isExecutable(join(share, 'srcz-desktop', 'app', 'srcz-desktop')))
})

// --- install.sh ---

void test('install.sh installs the CLI from the release the latest page redirects to', async (t) => {
  for (const [machine, arch] of [['x86_64', 'amd64'], ['aarch64', 'arm64']] as const) {
    const sandbox = await createSandbox(t.after.bind(t))
    await createRelease(sandbox, { arch })

    const result = await run('install.sh', sandbox, { STUB_UNAME_M: machine })

    assertOk(result)
    assert.match(result.stdout, /installed .*\/\.local\/bin\/srcz \(srcz 0\.1\.7 \(abc123\)\)/)
    const log = await readFile(sandbox.log, 'utf8')
    assert.match(log, /-w %\{redirect_url\} https:\/\/github\.com\/flexdinesh\/srcz\/releases\/latest/)
    assert.ok(log.includes(`https://github.com/flexdinesh/srcz/releases/download/v0.1.7/srcz_0.1.7_linux_${arch}.tar.gz`), log)
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
  assert.match(result.stdout, /installed .*\/\.local\/bin\/srcz \(srcz 0\.1\.7 \(abc123\)\)/)
  assert.doesNotMatch(await readFile(sandbox.log, 'utf8'), /^curl /m)
  assert.match(await readFile(sandbox.log, 'utf8'), /wget -S --max-redirect=0 -O \/dev\/null https:\/\/github\.com\/flexdinesh\/srcz\/releases\/latest/)
})

void test('install.sh stops when the latest release or its archive is missing', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

  const noLatest = await run('install.sh', sandbox, { STUB_LATEST: '' })
  assert.notEqual(noLatest.code, 0)
  assert.match(noLatest.stderr, /could not find the latest srcz release at https:\/\/github\.com\/flexdinesh\/srcz\/releases\/latest/)

  const noArchive = await run('install.sh', sandbox)
  assert.notEqual(noArchive.code, 0)
  assert.match(noArchive.stderr, /could not download srcz_0\.1\.7_linux_amd64\.tar\.gz from srcz v0\.1\.7/)
  assert.equal(await exists(join(sandbox.home, '.local', 'bin', 'srcz')), false)
})

for (const script of ['install-desktop.sh', 'uninstall.sh']) {
  void test(`${script} keeps page state when XDG_DATA_HOME equals XDG_CONFIG_HOME`, async (t) => {
    const sandbox = await createSandbox(t.after.bind(t))
    await createDesktopRelease(sandbox)
    const same = join(sandbox.root, 'xdg')
    const pageState = join(same, 'srcz-desktop', 'localstorage', 'page.localstorage')
    await writeExecutable(pageState, 'tabs')

    const result = await run(script, sandbox, { XDG_DATA_HOME: same, XDG_CONFIG_HOME: same })

    assertOk(result)
    assert.ok(await exists(pageState), 'page state shares the directory and stays')
    assert.equal(await exists(join(same, 'srcz-desktop', 'app', 'srcz-desktop')), script === 'install-desktop.sh')
  })
}

// --- uninstall.sh ---

void test('uninstall.sh removes the desktop app, reports package-managed copies, and keeps user data', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox, { version: '0.0.0-edge' })
  assertOk(await run('install-desktop-edge.sh', sandbox))
  const share = join(sandbox.home, '.local', 'share')
  const config = join(sandbox.home, '.config')
  const pageState = join(share, 'srcz-desktop', 'localstorage')
  await mkdir(pageState, { recursive: true })
  await writeExecutable(join(sandbox.home, 'go', 'bin', 'srcz'), fakeSrcz('0.1.6'))
  await writeExecutable(join(sandbox.home, '.local', 'bin', 'srcz'), fakeSrcz('0.1.6'))
  await mkdir(join(config, 'srcz'), { recursive: true })
  await writeFile(join(config, 'srcz', 'roots.json'), '[]')
  await mkdir(join(sandbox.home, '.cache', 'srcz'), { recursive: true })
  const shims = join(sandbox.root, 'mise', 'shims')
  await writeExecutable(join(shims, 'srcz'), fakeSrcz('0.1.6'))

  const result = await run('uninstall.sh', sandbox, { PATH: `${sandbox.stubs}:${join(sandbox.home, '.local', 'bin')}:${shims}:${SYSTEM_PATH}` })

  assertOk(result)
  for (const path of [
    join(sandbox.home, '.local', 'bin', 'srcz'),
    join(sandbox.home, '.local', 'bin', 'srcz-desktop'),
    join(sandbox.home, 'go', 'bin', 'srcz'),
    join(share, 'srcz-desktop', 'app'),
    join(share, 'applications', 'srcz-desktop.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'srcz-desktop.png'),
  ]) {
    assert.equal(await exists(path), false, `${path} is removed`)
  }
  for (const path of [join(config, 'srcz', 'roots.json'), join(sandbox.home, '.cache', 'srcz'), pageState]) {
    assert.ok(await exists(path), `${path} is kept`)
  }
  assert.match(result.stdout, /mise\/shims\/srcz is installed by mise/)
  assert.match(result.stdout, /kept your data/)
  assert.ok(result.stdout.includes(join(config, 'srcz')))
  assert.ok(result.stdout.includes(join(share, 'srcz-desktop')), 'page state is reported')
})

void test('uninstall.sh removes the macOS bundle and keeps page state', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const applications = join(sandbox.home, 'Applications')
  const bundle = join(applications, 'srcz-desktop.app')
  await writeExecutable(join(bundle, 'Contents', 'MacOS', 'app'), 'app')
  const webkit = join(sandbox.home, 'Library', 'WebKit', 'io.github.flexdinesh.srcz.desktop')
  await mkdir(webkit, { recursive: true })

  const result = await run('uninstall.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'arm64' })

  assertOk(result)
  assert.equal(await exists(bundle), false, `${bundle} is removed`)
  assert.ok(await exists(webkit), 'page state is kept')
  assert.ok(result.stdout.includes(webkit))
})

void test('install-desktop-edge.sh warns about a running daemon with the bundled srcz', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  await createDesktopRelease(sandbox, { version: '0.0.0-edge' })

  const result = await run('install-desktop-edge.sh', sandbox, { STUB_DAEMON_STATUS: '0' })

  assertOk(result)
  const bundled = join(sandbox.home, '.local', 'share', 'srcz-desktop', 'app', 'bin', 'srcz')
  assert.ok(result.stdout.includes(`"${bundled}" daemon stop`), result.stdout)
})

void test('uninstall.sh stops for a running daemon when only the app is installed', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const bundled = join(sandbox.home, '.local', 'share', 'srcz-desktop', 'app', 'bin', 'srcz')
  await writeExecutable(bundled, '#!/bin/sh\nexit 0\n')

  const result = await run('uninstall.sh', sandbox)

  assert.notEqual(result.code, 0)
  assert.ok(result.stderr.includes(`${bundled} daemon stop`), result.stderr)
  assert.ok(await exists(bundled), 'nothing is removed under a live daemon')
})

void test('uninstall.sh stops while srcz runs and reports an empty home', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const bin = join(sandbox.home, '.local', 'bin', 'srcz')
  await writeExecutable(bin, fakeSrcz('0.1.6'))

  const daemon = await run('uninstall.sh', sandbox, { STUB_DAEMON_STATUS: '0' })
  assert.notEqual(daemon.code, 0)
  assert.match(daemon.stderr, /a srcz daemon is running; stop it with '.*srcz daemon stop'/)
  const app = await run('uninstall.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(app.code, 0)
  assert.match(app.stderr, /the srcz desktop app is running/)
  assert.ok(await exists(bin))

  const empty = await createSandbox(t.after.bind(t))
  const result = await run('uninstall.sh', empty)
  assertOk(result)
  assert.match(result.stdout, /nothing to remove/)
})
