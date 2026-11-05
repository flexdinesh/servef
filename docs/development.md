# Development

## Setup

```sh
mise trust && mise install && mise run setup   # once per clone; mise 2026.10.18+
mise tasks                                     # list every task
```

## Install from main

CI publishes the `edge` prerelease after every green push to `main`; `servef --version` prints `0.0.0-edge` and the commit. Edge has no macOS Intel builds.

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-edge.sh | sh           # CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-electron-edge.sh | sh   # desktop app and CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-native-edge.sh | sh    # native app
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh                # back to stable
curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/uninstall.sh | sh              # remove all
```

Run it:

```sh
servef --version              # servef 0.0.0-edge <commit> <date>
servef daemon stop            # restart the daemon on the new build (next servef starts it)
servef .                      # desktop app, else browser
servef browser .              # browser
servef-native .               # native app
servef-native --version
which -a servef               # which install wins on PATH
```

From a checkout:

```sh
mise run install                                                  # go install to $(go env GOBIN) (mise's Go bin; on PATH in mise shells)
mise run build:web && go build -o ./bin/servef ./apps/servef      # local binary in ./bin
./bin/servef .                                                    # run it (--version prints servef dev)
```

## Browser

```sh
mise run dev                  # Go :8080 + Vite :5173; serves testdata/markdown and .dev/git-fixture
mise run dev:server           # Go only
mise run dev:web              # Vite only, against the Go server
mise run dev:mock             # Vite with testdata/api fixtures, no Go; /__ui is the primitive gallery
mise run dev:walkthroughs     # republish sample walkthroughs
./bin/servef browser ~/code/foo   # local binary against any folder
```

Ports: copy `.env.example` to `.env`, set `SERVEF_DEV_SERVER_PORT` and `SERVEF_DEV_WEB_PORT`.

## Desktop (Electron)

```sh
mise run dev:electron          # isolated dev daemon, roots, and app data under .dev/electron
mise run build:electron        # dev binary and bundle only
apps/electron/node_modules/.bin/electron --no-sandbox apps/electron --open-path <dir>   # OS folder entry (dev:electron env)
SERVEF_DESKTOP_AUTOSTART_DIR=<abs dir> mise run dev:electron   # show Open at Login (Linux)
mise run measure:electron      # memory scenario (needs a display)
```

## Native (mygo, experimental)

User docs: [native app](../apps/site/src/pages/docs/native.mdx); decision 0082.

```sh
go build -o .dev/native/servef ./apps/servef
GOWORK=off SERVEF_NATIVE_BINARY=$PWD/.dev/native/servef SERVEF_NATIVE_SOCKET=$PWD/.dev/native/daemon.sock SERVEF_NATIVE_ROOTS_FILE=$PWD/.dev/native/roots.json go -C apps/native run . $PWD/testdata/markdown
.dev/native/servef daemon stop --socket $PWD/.dev/native/daemon.sock   # stop its daemon
mise run test:native          # opt-in tests (not in check:pr)
```

Headless agent tooling (decision 0086; `packages/mygo`, loop in `skills/mygo-parity/SKILL.md`, queue in [native-parity.md](native-parity.md)). Outputs go to `.dev/native-shots/<scenario>/`. Headless needs no display but does need the system's Pango, cairo, and fontconfig with a font (Debian: `libpango-1.0-0 libpangocairo-1.0-0 fontconfig fonts-dejavu-core`): mygo draws text through them and bundles no fonts (decision 0088).

```sh
mise run native:shots -- document-preview     # scenario shots (apps/native/scenarios), no display
mise run native:compare -- document-preview   # plus web shots from pnpm dev:mock and web|native composites
mise run native:agent                         # serve the app headless on the API fixtures, then:
.dev/native-agent/bin/mygo-agent --socket .dev/native-agent/agent.sock snapshot -i
.dev/native-agent/bin/mygo-agent --socket .dev/native-agent/agent.sock click "markdown"
.dev/native-agent/bin/mygo-agent --socket .dev/native-agent/agent.sock screenshot .dev/native-shots/now.png
mise run test:native:e2e                      # built app (-tags e2e) under Xvfb in Dagger
```

Needs Go 1.27 (`GOTOOLCHAIN=auto` downloads it). Omit the `SERVEF_NATIVE_*` socket and roots vars to use your real daemon.

## Remote and ssh

```sh
ssh host 'servef ~/code/foo'                    # no-UI host: prints web: and desktop: links
servef browser --host 0.0.0.0 --no-open .       # simulate a remote server locally
ssh -o BatchMode=yes host true                  # ssh host must pass this for the desktop app
mise run dev:electron                            # then File > Connect... and pick the host
```

Dev seams: `SERVEF_DESKTOP_SSH`, `SERVEF_DESKTOP_SSH_CONFIG`, `SERVEF_DESKTOP_RUNTIME_DIR`, `SERVEF_DESKTOP_AUTO_CONFIRM=1` with `SERVEF_DESKTOP_DIALOG_LOG`.

## Docs site

```sh
mise run dev:site                               # Vocs dev server
pnpm build:site                                 # static build to apps/site/dist/public
pnpm --filter @servef/site preview              # preview the build
```

## Test

```sh
mise run test:go              # Go
mise run test:race            # Go with race detector
mise run test:web             # frontend
mise run test:browser -- tests/browser/<spec>.ts   # browser (omit spec for all)
mise run test:browser:webkit -- tests/browser/<spec>.ts   # WebKit (opt-in)
mise run test:electron         # desktop unit, real sshd, Electron smoke
mise run test:release         # install scripts and release tooling
mise run typecheck            # TypeScript
mise run lint                 # oxlint and stylelint
mise run fmt:check && mise run tidy:check
mise run contracts:generate && mise run contracts:check   # after pkg/contract edits
```

Desktop smoke on a host display:

```sh
mise run build:electron && SERVEF_DESKTOP_BINARY=$PWD/.dev/electron/servef pnpm --filter @servef/electron test:smoke
```

Add `SERVEF_DESKTOP_ELECTRON_ARGS=--ozone-platform=wayland` under Wayland.

## Before a PR

```sh
mise run check:pr             # full validation of committed HEAD (--force reruns)
mise run check:status         # right before push; HEAD must match
mise run check:prune          # prune old artifacts
```

Details: [validation](validation.md). Adding file types: `AGENTS.md`, "Adding Languages and File Types".

## Package

```sh
mise run package:electron:dir  # unpacked desktop app, host OS and arch
mise run package:electron      # installers in apps/electron/dist
```

Releases and docs deploy: [release](release.md).
