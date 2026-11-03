# 0078: apps/ and packages/ layout

- Status: Accepted; revised by [0080](0080-application-layer-contracts-and-transports.md) (creates `packages/contracts`)
- Date: 2026-11-03
- Revises: [0021](0021-built-frontend-on-release-tags-and-edge.md)
  (`go install` from the repository root),
  [0073](0073-install-scripts.md) (paths the install scripts read)

## Context

The Go module, its `main.go`, `cmd/`, and `internal/` sat at the repository
root beside three pnpm workspaces (`web/`, `desktop/`, `site/`) and the
repository tooling. A native shell is planned as another application on a
newer Go release (1.27) than the server (1.26). Applications, shared
packages, and repository tooling need separate homes, and the server must
not inherit another application's Go version.

## Decision

Move files only; no behavior changes.

| Path | Contents |
| --- | --- |
| `apps/servef/` | `go.mod`, `go.sum`, `main.go`, `cmd/`, `internal/`; the embedded UI builds into `apps/servef/internal/web/dist/` |
| `apps/web/`, `apps/desktop/`, `apps/site/` | the former `web/`, `desktop/`, `site/`; package names stay `@servef/web`, `@servef/desktop`, `@servef/site` |
| `packages/` | created when the first shared package exists, not before |
| root | `testdata/` (shared by Go, frontend, browser, and desktop tests), `tools/`, `tests/`, `scripts/`, `.dagger/`, `docs/`, `skills/`, `patches/`, workspace manifests, and repository docs |

The module path stays `github.com/flexdinesh/servef`, so imports and
`-ldflags -X github.com/flexdinesh/servef/internal/version...` stamps are
unchanged.

### Go workspace

A root `go.work` (Go 1.26) uses `./apps/servef` and `./tools`. `tools/`
becomes its own module (`github.com/flexdinesh/servef/tools`, standard
library only), so `go run ./tools/validate` and `go run ./tools/release-version`
keep working from the root. Go commands at the root name packages
explicitly (`./apps/servef/... ./tools/...`): `./...` matches nothing at a
workspace root that is not itself a module. `tidy:check` runs
`go mod tidy -diff` in each module.

`.dagger/` keeps its own module outside `go.work`; Dagger builds it from
`dagger.json`, and validation runs it unchanged with the workspace present.
A future native app on Go 1.27 stays out of `go.work` too: a workspace
selects one toolchain for every member, and the server stays on the Go that
`mise.lock` pins.

`go mod download` at the workspace root records sums for the whole module
graph in `go.work.sum`; Git ignores it, and each module's `go.sum` stays
authoritative.

### packages/

No `packages/contracts` now. An empty pnpm package fails typecheck and lint
tooling and has no consumer. The first shared package creates `packages/`
and joins `pnpm-workspace.yaml`.

### Tooling and paths

`pnpm-workspace.yaml`, the lockfile importers, mise tasks, Dagger inputs and
working paths, validation selectors, workflows, and install scripts follow
the move. Validation groups keep their scope: `apps/servef/` replaces
`cmd/`, `internal/`, and root Go files in each group's inputs.

`install-desktop.sh` fetches the icon from a release tag. Tags from before
this layout keep it at `desktop/resources/icon.png`; the script tries
`apps/desktop/resources/icon.png` first, then falls back.

## Consequences

- `go install github.com/flexdinesh/servef@...` works only for tags from
  before this layout: there is no `go.mod` at the root of later commits, so
  `@edge` stops building with the first edge publish after this change. The
  maintainer is dropping `go install` and Homebrew; the next phase reworks
  distribution. Until then, release and edge workflows only follow the new
  paths.
- Go tasks and docs name `./apps/servef/...` instead of `./...`, and
  `go build ./apps/servef` instead of `go build .`.
- The Cloudflare Pages build output directory (a dashboard setting,
  decision 0074) becomes `apps/site/dist/public`.
- Historical decision records keep their original paths.
