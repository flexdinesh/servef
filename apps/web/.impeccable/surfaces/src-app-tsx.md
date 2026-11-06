---
version: 1
slug: "src-app-tsx"
primary_target: "src/App.tsx"
related_targets: []
---

# Markdown workspace

Mode: Operate for navigation; Read for Markdown preview.
Scope: existing workspace, explorer, preview, status bar, content search.
Constraints: local assets, one Go binary, strict TypeScript, existing diagrams/source/navigation.

## Direction contract

THESIS: Sharp slate panes: a compact workspace with VS Code's conventions and Zed's visual language. App identity and global actions stay in the status bar.

OWN-WORLD: Cool slate surfaces, with a near-white document in Source Zero Light; square panes meeting on 1px hairlines, with no canvas gaps; flat rows, compact sans UI, mono chrome labels, proportional prose, monospace code. Every corner is square. All named themes share semantic tokens.

STORY: Browse paths with Pierre, read a document, or search its workspace's content separately. Review a compare as one continuous buffer.

FIRST VIEWPORT: Activity bar and explorer share one pane; the document toolbar and content share another, split by the sidebar's hairline. Workspace name remains above the left explorer; 32px mono tabs remain on the right. Footer stays full-width on chrome under a top hairline, aligned with the document pane. Mobile uses a full-height files drawer. Content typography and control placement remain as documented in DESIGN.md.

FORM: User-pinned VS Code conventions and the approved direction C, Slate panes (decision 0102); seed none, user-pinned; code-first implementation. No unchosen visual alternatives.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Unresolved decisions: none. Ctrl/Cmd-click adopts editor selection, as approved.
