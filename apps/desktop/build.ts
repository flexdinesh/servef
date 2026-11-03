// Bundles the main process (ESM) and the sandboxed preload (CJS) into build/.
import { build } from "esbuild"
import { fileURLToPath } from "node:url"

const root = fileURLToPath(new URL(".", import.meta.url))

const shared = {
  absWorkingDir: root,
  bundle: true,
  platform: "node",
  target: "node24",
  external: ["electron"],
  sourcemap: "linked",
  logLevel: "warning",
} satisfies Parameters<typeof build>[0]

await Promise.all([
  build({ ...shared, entryPoints: ["src/main/main.ts"], outfile: "build/main.mjs", format: "esm" }),
  // Sandboxed preloads must be CommonJS.
  build({ ...shared, entryPoints: ["src/preload/preload.ts"], outfile: "build/preload.cjs", format: "cjs" }),
])
