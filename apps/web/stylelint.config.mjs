// Design-token enforcement (decision 0033). Raw colors, type sizes, radii,
// depth, and motion values belong in styles/tokens.css and styles/themes.css;
// everything else references them through var().
const tokenOnly = [
  "/color$/",
  "background",
  "fill",
  "stroke",
  "font-size",
  "border-radius",
  "z-index",
]

export default {
  defaultSeverity: "error",
  plugins: ["stylelint-declaration-strict-value"],
  rules: {
    "color-no-hex": true,
    "color-named": "never",
    // Shadows and fills mix token colors; raw color functions stay in tokens.
    "function-disallowed-list": ["rgb", "rgba", "hsl", "hsla", "oklch", "lab", "lch"],
    "scale-unlimited/declaration-strict-value": [tokenOnly, {
      ignoreValues: ["transparent", "currentColor", "currentcolor", "inherit", "initial", "unset", "none", "0", "0px", "1px", "2px", "/^calc\\(/"],
      ignoreFunctions: true,
      disableFix: true,
      message: "Use a token from styles/tokens.css for ${property} (got ${value}).",
    }],
    "declaration-property-value-disallowed-list": [{
      "/^(transition|animation)/": ["/\\b\\d+(\\.\\d+)?m?s\\b/"],
      "/.*/": ["/(^|[^\\w.-])([3-9]|\\d{2,})(\\.\\d+)?px\\b/"],
    }, { message: "Use a motion or size token from styles/tokens.css (px only for 1-2px borders and offsets)." }],
  },
  overrides: [
    {
      files: ["src/styles/tokens.css", "src/styles/themes.css"],
      rules: {
        "color-no-hex": null,
        "color-named": null,
        "function-disallowed-list": null,
        "scale-unlimited/declaration-strict-value": null,
        "declaration-property-value-disallowed-list": null,
      },
    },
  ],
}
