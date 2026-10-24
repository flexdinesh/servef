# 0036: Omarchy themes and the Color Theme quick pick

- Status: Accepted
- Date: 2026-10-24
- Extends: [0013](0013-ui-settings-and-base-font-size.md) theme setting,
  [0033](0033-design-tokens-and-ui-primitives.md) named themes

## Context

servef offered five named palettes beside Light, Dark, and System. Omarchy
ships 23 built-in themes, each a `colors.toml` palette (mode, accent,
background and foreground tiers, ANSI hues). Copied as-is they fail servef's
roles: muted text is below 4.5:1 in 18 of them, light-theme yellows sit near
2:1, and several palettes collapse red, yellow, and green into one hue
(Hackerman, Lumon, Vantablack, White), which erases diff additions and
deletions. Choosing among ~28 themes from a select also gives no way to
compare them quickly.

## Decision

- Named themes: Dracula, Solarized Light, and Omarchy's 23 built-ins
  (`basecamp/omarchy themes/*`): Catppuccin, Catppuccin Latte, Ethereal,
  Everforest, Flexoki Light, Gruvbox, Hackerman, Kanagawa, Last Horizon,
  Lumon, Lupine, Matte Black, Miasma, Nord, Osaka Jade, Retro 82, Ristretto,
  Rose Pine, Solitude, Starship, Tokyo Night, Vantablack, White. IDs are
  Omarchy's folder names; labels are Omarchy's names. Light, Dark, and System
  are unchanged.
- Omarchy's palettes replace servef's Catppuccin Mocha, Gruvbox Dark, and
  Nord. Stored `catppuccin-mocha` and `gruvbox-dark` read as `catppuccin` and
  `gruvbox`.
- `theme.ts` holds one table of label and polarity per theme; validation,
  resolution, and option order derive from it. Options list System, Light,
  Dark, then named themes by label.
- Each palette maps to servef tokens once by a fixed rule (recorded in
  `themes.css`), then stays hand-owned CSS. No runtime parsing or generator.
- Readable first: text, links, status, and syntax tokens reach 4.5:1 and
  comments 3:1 on `surface` and `surface-secondary`; only OKLCH lightness
  moves. A status role whose palette hue leaves its family takes the nearest
  in-family palette color, else a quiet hue capped at the palette's own
  chroma. Success and destructive stay at least 60° apart. Light, Dark,
  Dracula, and Solarized Light follow the same floors. `themes.test.ts` enforces block
  coverage, the full token set, and every floor.
- “Preferences › Color Theme” in the command palette opens a quick pick on
  the content-palette frame: System, then “light themes” and “dark themes”
  groups. Arrows and filtering preview the active theme without storing it;
  Enter or click stores it; Escape or the backdrop restores the stored theme.
  Hover does not preview. No global shortcut: browsers reserve Ctrl/Cmd+T.

## Consequences

- Some palettes look different from Omarchy: brighter muted text and
  comments in most dark themes, darker ink in Solarized Light and the light
  Omarchy themes, and tinted status colors in monochrome themes.
- Users of Catppuccin Mocha and Gruvbox Dark see Omarchy's colors.
- Pierre file icons keep their own hues in every theme.
- Light's muted text darkens slightly (`#666f7e`); Dark's primary buttons
  and badges use dark text on the accent.
- A preview resolves polarity like a stored theme, so diffs and diagrams
  follow it.
