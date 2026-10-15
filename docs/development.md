# Development

## Requirements

- Go 1.24 or later
- Node.js 26
- pnpm 11

## Install from main

Install the latest commit directly from `main`:

```sh
go install github.com/flexdinesh/servef@main
```

The Go install includes the embedded frontend; Node.js and pnpm are not required.

`@main` follows the branch head without waiting for CI; `@latest` selects the
latest stable version tag. Go module proxies may briefly cache an older branch
revision. To bypass the proxy for this module:

```sh
GONOPROXY=github.com/flexdinesh/servef go install github.com/flexdinesh/servef@main
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

Browser tests start API servers on `PORT` (default `18080`) and the next two
ports, plus the Vite server on `15173`.

`pnpm link:local` rebuilds the embedded frontend and installs the current
checkout with `go install`.

Initial setup:

```sh
go mod download
pnpm install
```

## Web development

`pnpm dev` starts Go on port `8080` and Vite on port `5173`. Stop it before
starting the comparison setup below. `pnpm dev:mock` runs Vite without Go and
uses the default API scenario under `testdata/api`.

Frontend source and dependencies live in the `web` workspace. Production builds
still write to `internal/web/dist` because those committed files are embedded in
the Go binary.

## Compare feature flags

Flags are fixed when the server starts. Build once, then run one server per flag set on a unique port.

Run this block in one terminal, with `pnpm dev` stopped:

```sh
# Build the self-contained server.
pnpm build:web
go build -o ./bin/servef .

# Stop both background servers when this block exits.
(
  trap 'kill $(jobs -p) 2>/dev/null' EXIT INT TERM

  # Default Excalidraw: http://localhost:8080
  ./bin/servef testdata/markdown --port 8080 --no-open &

  # tldraw: http://localhost:8081
  ./bin/servef testdata/markdown --port 8081 --no-open \
    --feature mermaid-tldraw &

  wait
)
```

Compare `http://localhost:8080` and `http://localhost:8081`. Press Ctrl-C to stop both. Repeat the server command with a new port for other flag sets.

`--feature` is repeatable. Unknown names fail startup.

## Release

See the [release guide](release.md).
