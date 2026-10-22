import type { Theme } from "@pierre/highlights"

// highlights-themes-subset stands in for Highlights' bundled theme catalog when
// production builds bundle @pierre/diffs (vite.config.ts, decision 0029).
// servef registers its own theme (syntax-theme.ts), so no bundled one ships.
export const themes: Record<string, () => Promise<{ default: Theme }>> = {}
