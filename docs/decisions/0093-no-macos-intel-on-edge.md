# 0093: No macOS Intel on edge

- Status: Accepted
- Date: 2026-11-05
- Revises: [0079](0079-releases-edge-prerelease-and-install-scripts.md) ("the
  same asset set" on edge)

## Context

Every push to `main` builds the edge asset set. macOS Intel adds a CLI archive,
an Electron dmg and zip, and a native zip per push. CI never runs them:
`macos-latest` is Apple silicon, so the macOS smoke tests cover arm64 only.

## Decision

- Edge builds no `darwin_amd64` assets. Stable releases keep them.
- `build.yml` takes `macos-arches` (default `amd64 arm64`); `ci.yml` passes
  `arm64`. The CLI, Electron, native, and expected-asset steps all read it, so
  the exact-set check still holds per caller.
- `apps/native/scripts/package.sh` takes an optional architecture list.
- The edge install scripts stop on macOS Intel before downloading and point at
  the stable scripts (`shared:edge`).
- `ci.yml` needs no publish change: it already deletes assets no longer built.

## Consequences

- Intel Macs cannot follow `main`; they install stable, or build from source.
- A Mac shell under Rosetta reports `x86_64`, so it hits the same stop.
- Stable keeps shipping macOS Intel assets that only a release build produces,
  so a packaging break there surfaces at release time, not on a `main` push.
