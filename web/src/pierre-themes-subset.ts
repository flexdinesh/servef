import { createThemeCollection } from "@pierre/theming"

// pierre-themes-subset stands in for `@pierre/theming/themes` when production
// builds bundle @pierre/diffs (vite.config.ts, decision 0024). Diffs use only
// servef's CSS-variable theme, so no Pierre or Shiki theme ships.
export { createTheme } from "@pierre/theming/themes"

const none = createThemeCollection({ themes: [] })

export const pierreThemes = none
export const shikiThemes = none
export const themes = none
