import babelParser from "@babel/eslint-parser"
import reactHooks from "eslint-plugin-react-hooks"
import { defineConfig } from "eslint/config"

// React Hooks lint only. TypeScript 7 has no JavaScript API for
// typescript-eslint, so Babel parses the TypeScript syntax.
function babel(plugins: string[]) {
  return {
    parser: babelParser,
    parserOptions: { requireConfigFile: false, babelOptions: { babelrc: false, configFile: false, parserOpts: { plugins } } },
  }
}

export default defineConfig([
  { files: ["src/**/*.ts"], languageOptions: babel(["typescript"]) },
  { files: ["src/**/*.tsx"], languageOptions: babel(["typescript", "jsx"]) },
  {
    files: ["src/**/*.{ts,tsx}"],
    plugins: { "react-hooks": { rules: reactHooks.rules } },
    rules: { "react-hooks/rules-of-hooks": "error", "react-hooks/exhaustive-deps": "error" },
  },
])
