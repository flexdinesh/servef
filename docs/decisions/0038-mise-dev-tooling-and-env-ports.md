# 0038: mise owns dev tooling, tasks, and env-configured dev ports

- Status: Accepted
- Date: 2026-10-24
- Supersedes: [0032](0032-browser-tests-before-push.md) check commands

## Context

`pnpm dev` hardcoded Go on `8080` and Vite on `5173`, so two checkouts, or
another local app on those ports, could not run side by side without editing
tracked scripts. Contributors installed Go, Node.js, and pnpm by hand; CI
installed them with separate setup actions from `go.mod` and `.node-version`,
which left Go on the `go.mod` minimum patch. `pnpm check` ran nine steps in
sequence and never checked `gofmt` or `go.mod` tidiness.

## Decision

- mise is required for development and CI. `mise.toml` declares tools,
  environment, and every composite task; `package.json` keeps single JS steps
  (`build:web`, `test:web`, `lint`, `typecheck`, Vite dev servers) and
  delegates `dev`, `check`, `test:browser`, and `link:local` to mise.
- Versions: Go from `mise.toml` (`1.26`; `go.mod` keeps the language
  minimum), Node.js from `.node-version`, pnpm from `packageManager`.
  Committed `mise.lock` pins exact versions and checksums for `linux-x64` and
  `macos-arm64`; `mise upgrade` moves it. `minimum_release_age` is `3d`,
  matching pnpm. `min_version` and the CI `jdx/mise-action` `version` move
  together.
- Checks: `mise run check` runs `build:web` once, then `gofmt`, `go mod tidy
  -diff`, mise config validation, vet, Go tests and build, web and release
  tests, typecheck, lint, and the docs build in parallel. CI and release run
  it. `mise run check:push`, from the `.githooks/pre-push` hook, swaps Go
  tests for race tests, then runs browser tests alone.
- `build:web` declares its sources and `internal/web/dist` output, so mise
  skips it when inputs are unchanged.
- Dev ports come from the environment: `SERVEF_DEV_SERVER_PORT` (Go, default
  `8080`) and `SERVEF_DEV_WEB_PORT` (Vite, default `5173`). Vite proxies `/api`
  to the Go port unless `SERVEF_API_TARGET` is set. Unset or empty uses the
  default; an invalid port fails startup. mise loads them from a gitignored
  `.env`; `.env.example` lists them.

## Consequences

- Without mise, `pnpm check`, `pnpm dev`, and the pre-push hook fail; single
  JS scripts still run.
- CI and local machines install the same locked tool versions.
- A `build:web` input missing from `sources` can leave a stale build; add it
  and rerun with `mise run build:web --force`.
- Race tests need CGO and a C compiler locally.
- Browser tests keep their own fixed ports and are unaffected by `.env`.
