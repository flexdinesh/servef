import { registerCustomTheme } from "@pierre/diffs"

import { SYNTAX_THEME } from "./syntax-theme.ts"

// servef's diff theme is its Highlights theme, so diffs color tokens as
// Source does and follow the active theme. The variables cross Pierre's
// shadow root by inheritance.
export const DIFF_THEME = SYNTAX_THEME.name

registerCustomTheme(DIFF_THEME, () => Promise.resolve(SYNTAX_THEME), "zed")
