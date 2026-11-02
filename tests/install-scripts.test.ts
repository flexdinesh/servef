import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { access, chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import test from 'node:test'

// The install scripts (decision 0073) run under /bin/sh with a fake HOME and
// stub git, go, mise, uname, xdg-mime, update-desktop-database, and pgrep
// first on PATH. Stubs log their arguments to STUB_LOG.

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
  git: [
    'echo "git $*" >> "$STUB_LOG"',
    'if [ "$1" = clone ]; then',
    '  for last; do :; done',
    '  mkdir -p "$last/desktop/resources"',
    "  printf 'png' > \"$last/desktop/resources/icon.png\"",
    '  echo "$last" > "$STUB_LOG.clone"',
    '  exit 0',
    'fi',
    'case "$*" in',
    '  *rev-parse*) echo 0123456789ab ;;',
    '  *log*) echo "edge: abcdef012345 with built frontend" ;;',
    '  *show*) echo 2026-11-02T00:00:00Z ;;',
    'esac',
  ],
  go: [
    'echo "go $*" >> "$STUB_LOG"',
    'case "$1" in',
    '  build)',
    '    out=""',
    '    while [ $# -gt 0 ]; do',
    '      if [ "$1" = -o ]; then out="$2"; fi',
    '      shift',
    '    done',
    '    cp "$STUB_FAKE_SERVEF" "$out"',
    '    ;;',
    '  env)',
    '    case "$2" in',
    '      GOBIN) echo "" ;;',
    '      GOPATH) echo "$HOME/go" ;;',
    '    esac',
    '    ;;',
    'esac',
  ],
  mise: [
    'echo "mise $*" >> "$STUB_LOG"',
    'if [ "$1 $2" = "run package:desktop:dir" ]; then',
    '  mkdir -p desktop/.build desktop/dist/linux-unpacked desktop/dist/mac-arm64/servef.app/Contents/MacOS',
    '  cp "$STUB_FAKE_SERVEF" desktop/.build/servef-host',
    "  printf 'app' > desktop/dist/linux-unpacked/servef-desktop",
    "  printf 'app' > desktop/dist/mac-arm64/servef.app/Contents/MacOS/servef",
    'fi',
  ],
  'xdg-mime': ['echo "xdg-mime $*" >> "$STUB_LOG"'],
  'update-desktop-database': ['echo "update-desktop-database $*" >> "$STUB_LOG"'],
  pgrep: ['exit "${STUB_PGREP:-1}"'],
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
  const fake = join(root, 'fake-servef')
  await writeExecutable(fake, fakeServef('0.0.0-dev (0123456789ab)'))
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
      STUB_FAKE_SERVEF: fake,
      STUB_UNAME_S: 'Linux',
      STUB_UNAME_M: 'x86_64',
    },
  }
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

void test('install-edge.sh builds the edge branch and installs the CLI on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

  const result = await run('install-edge.sh', sandbox)

  assertOk(result)
  const bin = join(sandbox.home, '.local', 'bin', 'servef')
  assert.ok(await isExecutable(bin))
  assert.match(result.stdout, /installed .*\/\.local\/bin\/servef \(servef 0\.0\.0-dev/)
  assert.doesNotMatch(result.stdout, /not on PATH/)
  const log = await readFile(sandbox.log, 'utf8')
  assert.match(log, /git clone --quiet --depth 1 --branch edge https:\/\/github\.com\/flexdinesh\/servef\.git /)
  assert.match(log, /go build -trimpath -ldflags -s -w -X github\.com\/flexdinesh\/servef\/internal\/version\.Version=0\.0\.0-dev -X github\.com\/flexdinesh\/servef\/internal\/version\.Commit=0123456789ab -X github\.com\/flexdinesh\/servef\/internal\/version\.Date=2026-11-02T00:00:00Z/)
  const clone = (await readFile(`${sandbox.log}.clone`, 'utf8')).trim()
  assert.equal(await exists(clone), false, 'the temporary clone is removed')
})

void test('install-edge.sh prefers ~/bin when only it is on PATH', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

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
  await writeExecutable(join(sandbox.home, '.local', 'bin', 'servef'), fakeServef('0.1.6'))
  const earlier = join(sandbox.root, 'brew-bin')
  await writeExecutable(join(earlier, 'servef'), fakeServef('0.1.5'))

  const result = await run('install-edge.sh', sandbox, {
    PATH: `${sandbox.stubs}:${earlier}:${join(sandbox.home, '.local', 'bin')}:${SYSTEM_PATH}`,
    STUB_DAEMON_STATUS: '0',
  })

  assertOk(result)
  assert.match(result.stdout, /updated .*\/\.local\/bin\/servef \(servef 0\.1\.6 -> servef 0\.0\.0-dev/)
  assert.match(result.stdout, /brew-bin\/servef comes first on PATH and is not the servef just installed/)
  assert.match(result.stdout, /servef daemon stop/)
})

void test('install-edge.sh stops without Go or on an unsupported platform', async (t) => {
  const noGo = await createSandbox(t.after.bind(t), { without: ['go'] })
  const missing = await run('install-edge.sh', noGo)
  assert.notEqual(missing.code, 0)
  assert.match(missing.stderr, /Go is required to build servef: install it from https:\/\/go\.dev\/dl\//)

  const sandbox = await createSandbox(t.after.bind(t))
  const windows = await run('install-edge.sh', sandbox, { STUB_UNAME_S: 'MINGW64_NT-10.0' })
  assert.notEqual(windows.code, 0)
  assert.match(windows.stderr, /unsupported OS MINGW64_NT-10\.0/)
  const i686 = await run('install-edge.sh', sandbox, { STUB_UNAME_M: 'i686' })
  assert.notEqual(i686.code, 0)
  assert.match(i686.stderr, /unsupported architecture i686/)
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- install-desktop-edge.sh ---

void test('install-desktop-edge.sh installs the Linux app, registers servef://, and adds folder actions', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t), { homeName: 'home 50%' })
  const share = join(sandbox.home, '.local', 'share')
  await writeExecutable(join(share, 'servef-desktop', 'servef-desktop.AppImage'), 'old')

  const result = await run('install-desktop-edge.sh', sandbox)

  assertOk(result)
  const app = join(share, 'servef-desktop', 'app', 'servef-desktop')
  assert.equal(await readFile(app, 'utf8'), 'app')
  assert.equal(await exists(join(share, 'servef-desktop', 'servef-desktop.AppImage')), false, 'the unpacked app replaces an AppImage')
  assert.ok(await isExecutable(join(sandbox.home, '.local', 'bin', 'servef')))

  const exec = `"${app.replaceAll('%', '%%')}"`
  const entry = await readFile(join(share, 'applications', 'servef-desktop.desktop'), 'utf8')
  assert.match(entry, /^MimeType=x-scheme-handler\/servef;$/m)
  assert.doesNotMatch(entry, /inode\/directory/)
  assert.ok(entry.includes(`\nExec=${exec} %U\n`), entry)
  const dolphin = join(share, 'kio', 'servicemenus', 'servef-desktop.desktop')
  assert.ok((await readFile(dolphin, 'utf8')).includes(`\nExec=${exec} --open-path %F\n`))
  assert.ok(await isExecutable(dolphin), 'KDE runs only executable user service menus')
  const nemo = await readFile(join(share, 'nemo', 'actions', 'servef-desktop.nemo_action'), 'utf8')
  assert.ok(nemo.includes(`\nExec=${exec} --open-path %F\n`))
  assert.match(nemo, /^Extensions=dir;$/m)
  assert.equal(await readFile(join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-desktop.png'), 'utf8'), 'png')

  const log = await readFile(sandbox.log, 'utf8')
  assert.match(log, /^mise trust mise\.toml$/m)
  assert.match(log, /^mise install$/m)
  assert.match(log, /^mise exec -- pnpm install --frozen-lockfile$/m)
  assert.match(log, /^mise run package:desktop:dir$/m)
  assert.match(log, /^xdg-mime default servef-desktop\.desktop x-scheme-handler\/servef$/m)
  assert.match(log, /^update-desktop-database .*\/applications$/m)
  assert.match(result.stdout, /servef \./)
})

void test('install-desktop-edge.sh installs servef.app on macOS without Linux registration', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))

  const result = await run('install-desktop-edge.sh', sandbox, { STUB_UNAME_S: 'Darwin', STUB_UNAME_M: 'arm64' })

  assertOk(result)
  const apps = (await exists('/Applications')) ? '/Applications' : join(sandbox.home, 'Applications')
  t.after(async () => {
    if (apps === '/Applications') await rm('/Applications/servef.app', { recursive: true, force: true })
  })
  assert.equal(await readFile(join(apps, 'servef.app', 'Contents', 'MacOS', 'servef'), 'utf8'), 'app')
  assert.ok(await exists(join(sandbox.home, '.local', 'bin', 'servef')))
  assert.equal(await exists(join(sandbox.home, '.local', 'share')), false)
  assert.doesNotMatch(await readFile(sandbox.log, 'utf8'), /xdg-mime/)
})

void test('install-desktop-edge.sh stops before building when it cannot install', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  const arm = await run('install-desktop-edge.sh', sandbox, { STUB_UNAME_M: 'aarch64' })
  assert.notEqual(arm.code, 0)
  assert.match(arm.stderr, /support linux\/amd64 and darwin\/arm64 .*not linux\/arm64/)

  const running = await run('install-desktop-edge.sh', sandbox, { STUB_PGREP: '0' })
  assert.notEqual(running.code, 0)
  assert.match(running.stderr, /the servef desktop app is running; quit it, then run this again/)

  const noMise = await createSandbox(t.after.bind(t), { without: ['mise'] })
  const missing = await run('install-desktop-edge.sh', noMise)
  assert.notEqual(missing.code, 0)
  assert.match(missing.stderr, /mise is required .*curl https:\/\/mise\.run \| sh/)

  assert.equal(await readFile(sandbox.log, 'utf8'), '', 'nothing cloned or built')
  assert.equal(await exists(join(sandbox.home, '.local')), false)
})

// --- uninstall.sh ---

void test('uninstall.sh removes every install, reports package-managed copies, and keeps user data', async (t) => {
  const sandbox = await createSandbox(t.after.bind(t))
  assertOk(await run('install-desktop-edge.sh', sandbox))
  const manual = join(sandbox.home, 'Applications', 'servef-desktop_0.1.7_linux_amd64.AppImage')
  await writeExecutable(manual, 'appimage')
  const unrelated = join(sandbox.home, 'Applications', 'other.AppImage')
  await writeExecutable(unrelated, 'other')
  const config = join(sandbox.home, '.config')
  await mkdir(join(config, 'autostart'), { recursive: true })
  await writeFile(join(config, 'autostart', 'servef-desktop.desktop'), `[Desktop Entry]\nExec=${manual}\n`)
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
  const share = join(sandbox.home, '.local', 'share')
  for (const path of [
    join(sandbox.home, '.local', 'bin', 'servef'),
    join(sandbox.home, 'go', 'bin', 'servef'),
    join(share, 'servef-desktop'),
    join(share, 'applications', 'servef-desktop.desktop'),
    join(share, 'icons', 'hicolor', '512x512', 'apps', 'servef-desktop.png'),
    join(share, 'kio', 'servicemenus', 'servef-desktop.desktop'),
    join(share, 'nemo', 'actions', 'servef-desktop.nemo_action'),
    join(share, 'nautilus', 'scripts', 'Open in servef'),
    join(config, 'autostart', 'servef-desktop.desktop'),
    manual,
  ]) {
    assert.equal(await exists(path), false, `${path} is removed`)
  }
  assert.ok(await exists(unrelated), 'only servef-desktop AppImages are removed')
  for (const path of [join(config, 'servef', 'roots.json'), join(config, 'servef-desktop'), join(sandbox.home, '.cache', 'servef')]) {
    assert.ok(await exists(path), `${path} is kept`)
  }
  assert.match(result.stdout, /mise\/shims\/servef is installed by mise/)
  assert.match(result.stdout, /kept your data/)
  assert.ok(result.stdout.includes(join(config, 'servef')))
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
