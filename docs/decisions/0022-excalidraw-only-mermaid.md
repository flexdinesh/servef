# 0022: Excalidraw-only Mermaid; no feature flags

- Status: Accepted
- Date: 2026-10-18

## Context

Mermaid diagrams rendered with Excalidraw by default; `--feature
mermaid-tldraw` switched them to tldraw. tldraw's license requires a
watermark and license key for production use, and its default assets load
from `cdn.tldraw.com`, against the bundled-assets rule. `mermaid-tldraw` was
the only feature flag.

## Decision

- Excalidraw is the only Mermaid renderer. tldraw and `@tldraw/mermaid` are
  removed.
- The feature-flag mechanism is removed: `--feature`, `/api/features`, the
  `feature-data` shell bootstrap, `internal/features`, and the frontend
  feature provider.
- The CSP no longer allows `cdn.tldraw.com`.

## Consequences

- `--feature` is an unknown flag; scripts that pass it fail at startup.
- Browser tests start two API servers instead of three.
- A future experiment reintroduces flags with its first real feature.
