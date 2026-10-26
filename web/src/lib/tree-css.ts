// treeCSS is the Pierre tree styling every tree shares: 12px chevrons, and
// the selection mark beside the selection tint, which alone is under 3:1
// (WCAG 1.4.11). Tokens inherit into the tree's shadow root.
export const treeCSS = `[data-icon-name="file-tree-icon-chevron"] { width: 12px; height: 12px; }
[data-item-selected="true"] { box-shadow: inset 0 0 0 1px var(--color-selected-border); }`
