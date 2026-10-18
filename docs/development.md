# Development

## Requirements

- Go 1.24 or later
- Node.js 26
- pnpm 11

## Install from edge

Install the latest `main` that passed CI:

```sh
go install github.com/flexdinesh/servef@edge
```

The Go install includes the embedded frontend; Node.js and pnpm are not required.

`main` does not track `internal/web/dist/`, so `go install ...@main` fails.
After CI passes on `main`, CI force-pushes `edge`: that `main` commit plus the
built frontend. `@latest` selects the latest stable version tag. Go module
proxies may briefly cache an older branch revision. To bypass the proxy for
this module:

```sh
GONOPROXY=github.com/flexdinesh/servef go install github.com/flexdinesh/servef@edge
```

## Commands

| Purpose | Command |
| --- | --- |
| Install Go dependencies | `go mod download` |
| Install JavaScript dependencies | `pnpm install` |
| Run Go and Vite together | `pnpm dev` |
| Run only the Go server | `pnpm dev:server` |
| Run only Vite against the Go server | `pnpm dev:web` |
| Run Vite with API fixtures | `pnpm dev:mock` |
| Build embedded frontend assets | `pnpm build:web` |
| Build a local binary | `pnpm build:web && go build -o ./bin/servef .` |
| Install the local CLI | `pnpm link:local` |
| Run Go tests | `go test ./...` |
| Run frontend tests | `pnpm test:web` |
| Test release tooling | `pnpm test:release` |
| Type-check TypeScript | `pnpm typecheck` |
| Install browser-test dependencies | `pnpm exec playwright install --with-deps chromium` |
| Run browser tests | `pnpm test:browser` |
| Run browser tests on other ports | `PORT=18090 pnpm test:browser` |

Browser tests start API servers on `PORT` (default `18080`) and the next
port, plus the Vite server on `15173`.

On Linux with unprivileged user namespaces, `pnpm test:browser` runs Playwright,
its servers, and Chromium in a private loopback-only network namespace. Host
interface changes, such as Docker creating container links, otherwise make
Chromium abort in-flight requests with `ERR_NETWORK_CHANGED` and leave pages
blank. The namespace also keeps test ports from colliding with host servers.
Elsewhere it runs Playwright directly; `pnpm exec playwright test` always does.

`pnpm link:local` rebuilds the embedded frontend and installs the current
checkout with `go install`.

Initial setup:

```sh
go mod download
pnpm install
pnpm build:web
```

Go builds, tests, and vet need `internal/web/dist/`; run `pnpm build:web` in
each fresh clone or worktree first.

## Web development

`pnpm dev` starts Go on port `8080` and Vite on port `5173`. `pnpm dev:mock` runs Vite without Go and
uses the default API scenario under `testdata/api`.

Frontend source and dependencies live in the `web` workspace. Production builds
write to `internal/web/dist`, which is embedded in the Go binary and ignored by
Git.

## Release

See the [release guide](release.md).
