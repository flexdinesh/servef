# Development

## Setup

```sh
mise trust && mise install && mise run setup   # once per clone; mise 2026.10.18+
mise tasks                                     # list every task
```

## Install from main

CI publishes the `edge` prerelease after every green push to `main`; `srcz --version` prints `0.0.0-edge` and the commit. Edge has no macOS Intel builds.

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install-edge.sh | sh                  # CLI and desktop app
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install-edge.sh | sh -s -- --cli-only   # CLI only
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh                       # back to stable
curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/uninstall.sh | sh                     # remove all
```

Run it:

```sh
srcz --version              # srcz 0.0.0-edge <commit> <date>
srcz daemon stop            # restart the daemon on the new build (next srcz starts it)
srcz .                      # desktop app, else browser
srcz browser .              # browser
srcz-desktop .              # desktop app, without the CLI
srcz-desktop --version
which -a srcz               # which install wins on PATH
```

From a checkout:

```sh
mise run install                                                  # go install to $(go env GOBIN) (mise's Go bin; on PATH in mise shells)
mise run build:web && go build -o ./bin/srcz ./apps/srcz      # local binary in ./bin
./bin/srcz .                                                    # run it (--version prints srcz dev)
```

## Browser

```sh
mise run dev                  # Go :8080 + Vite :5173; serves testdata/markdown and .dev/git-fixture
mise run dev:server           # Go only
mise run dev:web              # Vite only, against the Go server
mise run dev:mock             # Vite with testdata/api fixtures, no Go; /__ui is the primitive gallery
mise run dev:walkthroughs     # republish sample walkthroughs
./bin/srcz browser ~/code/foo   # local binary against any folder
```

Ports: copy `.env.example` to `.env`, set `SRCZ_DEV_SERVER_PORT` and `SRCZ_DEV_WEB_PORT`.

## Desktop app

User docs: [desktop app](../apps/site/src/pages/docs/desktop.mdx); decisions 0094, 0095. The window is the daemon's web app in the OS web view. Linux needs WebKitGTK 4.1 (Debian: `libwebkit2gtk-4.1-0`). `apps/desktop` needs Go 1.27 (`GOTOOLCHAIN` downloads it).

```sh
mise run dev:desktop                                 # build, then run against a private daemon, roots, and page state under .dev/desktop
.dev/desktop/srcz daemon stop --socket $PWD/.dev/desktop/daemon.sock   # stop its daemon
mise run test:desktop                                # unit tests
mise run test:desktop:e2e                            # built app (-tags e2e) under Xvfb in Dagger
mise run test:desktop:extended                       # desktop resource measurements
```

## Remote

```sh
ssh host 'srcz ~/code/foo'                    # no-UI host: prints web: and desktop: links
srcz browser --host 0.0.0.0 --no-open .       # simulate a remote server locally
```

## Docs site

```sh
mise run dev:site                               # Vocs dev server
pnpm build:site                                 # static build to apps/site/dist/public
pnpm --filter @srcz/site preview              # preview the build
```

## Test

```sh
mise run test:go              # Go
mise run test:race            # Go with race detector
mise run test:web             # frontend
mise run test:browser -- tests/browser/<spec>.ts   # browser (omit spec for all)
mise run test:browser:webkit -- tests/browser/<spec>.ts   # WebKit (opt-in)
mise run test:browser:extended # large-diff stress (opt-in)
mise run test:desktop         # desktop app unit tests
mise run test:desktop:e2e     # built desktop app under Xvfb
mise run test:release         # install scripts and release tooling
mise run typecheck            # TypeScript
mise run lint                 # oxlint and stylelint
mise run fmt:check && mise run tidy:check
mise run contracts:generate && mise run contracts:check   # after pkg/contract edits
```

## Before a PR

```sh
mise run check:pr             # correctness of committed HEAD (--force reruns)
mise run check:status         # right before push; HEAD must match
mise run check:extended       # performance/resources/compatibility when relevant
mise run check:full           # explicitly run both suites
mise run check:prune          # prune old artifacts
```

Details: [validation](validation.md). Adding file types: `AGENTS.md`, "Adding Languages and File Types".

## Package

`apps/desktop/scripts/package.sh` packages the desktop app from CLI archives, as `build.yml` does.

Releases and docs deploy: [release](release.md).
