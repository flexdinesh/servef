---
version: 1
slug: "web-src-app-tsx"
primary_target: "web/src/App.tsx"
related_targets: []
---

# Markdown workspace

Mode: Operate for navigation; Read for Markdown preview.
Scope: existing workspace, explorer, preview, status bar, content search.
Constraints: local assets, one Go binary, strict TypeScript, existing diagrams/source/navigation.

## Direction contract

THESIS: A compact VS Code-style Markdown workspace. App identity and global actions move into the status bar.

OWN-WORLD: Neutral editor surfaces on a shared canvas; rounded navigation and document islands, flat rectangular internal rows, subtle borders, compact sans UI, proportional prose, monospace code. All named themes share semantic tokens.

STORY: Browse paths with Pierre, read a document, or search its workspace's content separately.

FIRST VIEWPORT: Activity bar and explorer share one 10px-radius island; the document toolbar and content share another. Canvas insets and resize gutter are 6px, with 4px insets on mobile. Workspace name remains above the left explorer; 32px tabs remain on the right. Footer stays full-width on the canvas, aligned with the document island. Mobile uses a rounded files drawer. Content typography and control placement remain as documented in DESIGN.md.

FORM: User-pinned VS Code conventions and explicitly approved proposal/wireframe; code-first implementation. No unchosen visual alternatives.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Unresolved decisions: none. Ctrl/Cmd-click adopts editor selection, as approved.
